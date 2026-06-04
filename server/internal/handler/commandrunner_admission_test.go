package handler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

// createAdmissionTestRuntime inserts a temporary runtime in the handler test
// workspace with the given coarse status and last_seen_at, returning its UUID.
func createAdmissionTestRuntime(t *testing.T, status string, lastSeen time.Time) string {
	t.Helper()
	if testHandler == nil {
		t.Skip("no database available")
	}
	var id string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status,
			device_info, metadata, last_seen_at
		)
		VALUES ($1, NULL, $2, 'local', $3, $4, '', '{}'::jsonb, $5)
		RETURNING id
	`, testWorkspaceID, "Admission Test RT", "admission_test", status, lastSeen).Scan(&id); err != nil {
		t.Fatalf("insert admission test runtime: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_runtime WHERE id = $1`, id)
	})
	return id
}

// withFakeLiveness swaps in a fake liveness store for the duration of a test and
// restores the original afterward. Tests in this package run sequentially, so
// this is race-free as long as callers don't use t.Parallel().
func withFakeLiveness(t *testing.T, fake LivenessStore) {
	t.Helper()
	orig := testHandler.LivenessStore
	testHandler.LivenessStore = fake
	t.Cleanup(func() { testHandler.LivenessStore = orig })
}

// TestRequireCommandDeckRuntimeAdmission verifies that command dispatch admission
// consults the same liveness truth the Runtime Health panel uses: a stored
// "online"/"busy" status is not sufficient — the runtime must be live.
func TestRequireCommandDeckRuntimeAdmission(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	now := time.Now()

	tests := []struct {
		name     string
		status   string
		lastSeen time.Time
		liveness *fakeLivenessStore
		wantOK   bool
		wantCode int
	}{
		{
			name:     "live online runtime admitted",
			status:   "online",
			lastSeen: now.Add(-90 * time.Second), // old DB age — the flap window
			liveness: &fakeLivenessStore{available: true, aliveOK: true, aliveResult: map[string]bool{}},
			wantOK:   true,
		},
		{
			name:     "online-but-stale runtime rejected",
			status:   "online",
			lastSeen: now.Add(-5 * time.Second), // fresh DB age, but Redis says dead
			liveness: &fakeLivenessStore{available: true, aliveOK: true, aliveResult: map[string]bool{}},
			wantOK:   false,
			wantCode: 409,
		},
		{
			name:     "offline runtime rejected before liveness check",
			status:   "offline",
			lastSeen: now.Add(-5 * time.Second),
			liveness: &fakeLivenessStore{available: true, aliveOK: true, aliveResult: map[string]bool{}},
			wantOK:   false,
			wantCode: 400,
		},
		{
			name:     "db fallback admits recently-seen runtime when liveness unavailable",
			status:   "online",
			lastSeen: now.Add(-100 * time.Second), // within 150s envelope
			liveness: &fakeLivenessStore{available: false},
			wantOK:   true,
		},
		{
			name:     "db fallback rejects long-gone runtime when liveness unavailable",
			status:   "online",
			lastSeen: now.Add(-200 * time.Second), // beyond 150s envelope
			liveness: &fakeLivenessStore{available: false},
			wantOK:   false,
			wantCode: 409,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtimeID := createAdmissionTestRuntime(t, tt.status, tt.lastSeen)
			// For the admitted-while-online case, mark this runtime alive in Redis
			// despite its old DB last_seen_at — that is the flap the fix targets.
			if tt.liveness.available && tt.name == "live online runtime admitted" {
				tt.liveness.aliveResult[runtimeID] = true
			}
			withFakeLiveness(t, tt.liveness)

			req := newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/command-runner/run", nil)
			rec := httptest.NewRecorder()
			_, ok := testHandler.RequireCommandDeckRuntime(rec, req, testWorkspaceID, runtimeID)
			if ok != tt.wantOK {
				t.Fatalf("admission ok = %v, want %v (code %d)", ok, tt.wantOK, rec.Code)
			}
			if !tt.wantOK && rec.Code != tt.wantCode {
				t.Fatalf("rejection code = %d, want %d", rec.Code, tt.wantCode)
			}
		})
	}
}

// TestRequireCommandDeckRuntimeCrossWorkspace confirms workspace scoping is
// preserved: a runtime in another workspace is a 404 regardless of liveness.
func TestRequireCommandDeckRuntimeCrossWorkspace(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	runtimeID := createAdmissionTestRuntime(t, "online", time.Now())
	withFakeLiveness(t, &fakeLivenessStore{available: true, aliveOK: true, aliveResult: map[string]bool{runtimeID: true}})

	otherWorkspace := "99999999-9999-9999-9999-999999999999"
	req := newRequest("POST", "/api/workspaces/"+otherWorkspace+"/command-runner/run", nil)
	rec := httptest.NewRecorder()
	if _, ok := testHandler.RequireCommandDeckRuntime(rec, req, otherWorkspace, runtimeID); ok {
		t.Fatal("runtime from a different workspace must not be admitted")
	}
	if rec.Code != 404 {
		t.Fatalf("cross-workspace rejection code = %d, want 404", rec.Code)
	}
}

// TestHandleCommandRunnerRunRejectsStaleWithoutCreatingRow proves the admission
// gate fires before any command_run row is created for a stale target.
func TestHandleCommandRunnerRunRejectsStaleWithoutCreatingRow(t *testing.T) {
	if testHandler == nil {
		t.Skip("no database available")
	}
	runtimeID := createAdmissionTestRuntime(t, "online", time.Now())
	// available + aliveOK but runtime not in the alive map → resolved dead.
	withFakeLiveness(t, &fakeLivenessStore{available: true, aliveOK: true, aliveResult: map[string]bool{}})

	req := newRequest("POST", "/api/workspaces/"+testWorkspaceID+"/command-runner/run", map[string]string{
		"runtime_id": runtimeID,
	})
	req = withURLParam(req, "workspaceID", testWorkspaceID)
	rec := httptest.NewRecorder()
	testHandler.HandleCommandRunnerRun(rec, req)

	if rec.Code != 409 {
		t.Fatalf("stale dispatch code = %d, want 409", rec.Code)
	}

	var count int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM command_run WHERE runtime_id = $1`, runtimeID,
	).Scan(&count); err != nil {
		t.Fatalf("count command_run rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no command_run rows for rejected dispatch, found %d", count)
	}
}
