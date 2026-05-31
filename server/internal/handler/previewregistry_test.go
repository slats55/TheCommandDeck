package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidatePreviewTargetAllowsLocalSelfHostedPreview(t *testing.T) {
	target, err := validatePreviewTarget("http://localhost:3000")
	if err != nil {
		t.Fatalf("validatePreviewTarget returned error: %v", err)
	}
	if target.PublicURL != "http://localhost:3000" {
		t.Fatalf("PublicURL = %q", target.PublicURL)
	}
	if target.Port != 3000 {
		t.Fatalf("Port = %d, want 3000", target.Port)
	}
	if len(target.CheckURLs) != 2 {
		t.Fatalf("CheckURLs length = %d, want 2", len(target.CheckURLs))
	}
	if !strings.Contains(target.CheckURLs[0], "commanddeck-web:3000") {
		t.Fatalf("first check URL should use internal compose host, got %q", target.CheckURLs[0])
	}
}

func TestValidatePreviewTargetRejectsUnsafeTargets(t *testing.T) {
	tests := []string{
		"file:///etc/passwd",
		"ftp://localhost:3000",
		"http://example.com",
		"http://169.254.169.254/latest/meta-data",
		"https://10.0.0.2",
		"https://service.local",
		"https://user:pass@example.com",
		"://bad",
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if _, err := validatePreviewTarget(raw); err == nil {
				t.Fatalf("validatePreviewTarget(%q) succeeded, want error", raw)
			}
		})
	}
}

func TestProbePreviewHealthReportsHealthyForSuccessfulTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	target, err := validatePreviewTarget(server.URL)
	if err != nil {
		t.Fatalf("validatePreviewTarget returned error: %v", err)
	}
	got := probePreviewHealth(context.Background(), target)

	if got.Status != previewHealthStatusHealthy {
		t.Fatalf("Status = %q, want healthy", got.Status)
	}
	if got.StatusCode == nil || *got.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %v, want 200", got.StatusCode)
	}
	if got.PublicMessage != nil {
		t.Fatalf("PublicMessage = %q, want nil", *got.PublicMessage)
	}
}

func TestProbePreviewHealthSanitizesTransportErrors(t *testing.T) {
	target := previewTarget{
		PublicURL: "http://localhost:3000",
		CheckURLs: []string{
			"http://127.0.0.1:1",
		},
		Port: 3000,
	}

	got := probePreviewHealth(context.Background(), target)
	if got.Status != previewHealthStatusUnavailable {
		t.Fatalf("Status = %q, want unavailable", got.Status)
	}
	if got.PublicMessage == nil || *got.PublicMessage != safePreviewUnavailableMessage {
		t.Fatalf("PublicMessage = %v, want safe unavailable message", got.PublicMessage)
	}
	if got.PublicMessage != nil && strings.Contains(*got.PublicMessage, "127.0.0.1") {
		t.Fatalf("PublicMessage exposes internal target: %q", *got.PublicMessage)
	}
}

func TestProbePreviewHealthDoesNotFollowRedirects(t *testing.T) {
	redirectFollowed := false
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectFollowed = true
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	target, err := validatePreviewTarget(redirectServer.URL)
	if err != nil {
		t.Fatalf("validatePreviewTarget returned error: %v", err)
	}
	got := probePreviewHealth(context.Background(), target)

	if redirectFollowed {
		t.Fatalf("health probe followed redirect to %s", targetServer.URL)
	}
	if got.Status != previewHealthStatusUnavailable {
		t.Fatalf("Status = %q, want unavailable", got.Status)
	}
	if got.StatusCode == nil || *got.StatusCode != http.StatusFound {
		t.Fatalf("StatusCode = %v, want 302", got.StatusCode)
	}
}

func TestPreviewHealthClientHasBoundedTimeout(t *testing.T) {
	client := newPreviewHealthClient()
	if client.Timeout != previewHealthTimeout {
		t.Fatalf("Timeout = %s, want %s", client.Timeout, previewHealthTimeout)
	}
	if client.Timeout > 3*time.Second {
		t.Fatalf("Timeout = %s, want bounded timeout", client.Timeout)
	}
}

func TestHandleCommandDeckPreviewsIsReadOnly(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `DELETE FROM preview_registry WHERE workspace_id = $1`, testWorkspaceID); err != nil {
		t.Fatalf("cleanup preview registry: %v", err)
	}

	got := requestPreviewRegistry(t)
	if len(got.Previews) != 0 {
		t.Fatalf("GET previews length = %d, want 0 for read-only listing", len(got.Previews))
	}

	var count int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*) FROM preview_registry WHERE workspace_id = $1
	`, testWorkspaceID).Scan(&count); err != nil {
		t.Fatalf("count preview records: %v", err)
	}
	if count != 0 {
		t.Fatalf("preview_registry count = %d, want 0", count)
	}
}

func TestHandleCommandDeckPreviewSelfHostedSyncPersistsRecord(t *testing.T) {
	previewServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer previewServer.Close()

	t.Setenv("FRONTEND_ORIGIN", previewServer.URL)

	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `DELETE FROM preview_registry WHERE workspace_id = $1`, testWorkspaceID); err != nil {
		t.Fatalf("cleanup preview registry: %v", err)
	}

	first := requestPreviewRegistrySync(t)
	if len(first.Previews) != 1 {
		t.Fatalf("first response previews length = %d, want 1", len(first.Previews))
	}

	entry := first.Previews[0]
	if entry.ID == "self-hosted-web" || entry.ID == "" {
		t.Fatalf("preview ID = %q, want persisted UUID", entry.ID)
	}
	if entry.PreviewURL != previewServer.URL {
		t.Fatalf("PreviewURL = %q, want %q", entry.PreviewURL, previewServer.URL)
	}
	if entry.HealthStatus != previewHealthStatusHealthy {
		t.Fatalf("HealthStatus = %q, want healthy", entry.HealthStatus)
	}
	if entry.LastSuccessAt == nil || *entry.LastSuccessAt == "" {
		t.Fatal("LastSuccessAt was not recorded for healthy preview")
	}
	if entry.RegisteredAt == "" || entry.UpdatedAt == "" {
		t.Fatalf("registered/updated timestamps were not returned: %#v", entry)
	}

	second := requestPreviewRegistrySync(t)
	if len(second.Previews) != 1 {
		t.Fatalf("second response previews length = %d, want 1", len(second.Previews))
	}
	if second.Previews[0].ID != entry.ID {
		t.Fatalf("second preview ID = %q, want same persisted record %q", second.Previews[0].ID, entry.ID)
	}

	var count int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*)
		FROM preview_registry
		WHERE workspace_id = $1 AND source = 'self_hosted_stack' AND preview_url = $2
	`, testWorkspaceID, previewServer.URL).Scan(&count); err != nil {
		t.Fatalf("count persisted preview records: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted preview record count = %d, want 1", count)
	}
}

func TestHandleCommandDeckPreviewSelfHostedSyncDoesNotAssignUnprovenRuntime(t *testing.T) {
	previewServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer previewServer.Close()

	t.Setenv("FRONTEND_ORIGIN", previewServer.URL)
	resp := requestPreviewRegistrySync(t)
	if len(resp.Previews) != 1 {
		t.Fatalf("previews length = %d, want 1", len(resp.Previews))
	}
	entry := resp.Previews[0]
	if entry.RuntimeID != nil {
		t.Fatalf("RuntimeID = %v, want nil for unproven runtime provenance", *entry.RuntimeID)
	}
	if entry.MachineIdentity != nil {
		t.Fatalf("MachineIdentity = %v, want nil for unproven runtime provenance", *entry.MachineIdentity)
	}
}

func TestReportRuntimePreview_AssignsTrustedRuntimeAndKeepsCommandRunUnlinked(t *testing.T) {
	previewServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer previewServer.Close()

	runtimeID, daemonID := createDaemonPreviewTestRuntime(t, "preview-report-daemon-1")

	req := newDaemonTokenRequest(http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/previews/report", map[string]any{
		"preview_url": previewServer.URL,
		"name":        "Runtime Reported Preview",
	}, testWorkspaceID, daemonID)
	req = withURLParam(req, "runtimeId", runtimeID)

	w := httptest.NewRecorder()
	testHandler.ReportRuntimePreview(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ReportRuntimePreview status = %d: %s", w.Code, w.Body.String())
	}

	var resp previewRegistryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Previews) != 1 {
		t.Fatalf("previews length = %d, want 1", len(resp.Previews))
	}
	entry := resp.Previews[0]
	if entry.RuntimeID == nil || *entry.RuntimeID != runtimeID {
		t.Fatalf("RuntimeID = %v, want %s", entry.RuntimeID, runtimeID)
	}
	if entry.CommandRunID != nil {
		t.Fatalf("CommandRunID = %v, want nil", *entry.CommandRunID)
	}
	if entry.MachineIdentity == nil || !strings.EqualFold(*entry.MachineIdentity, daemonID) {
		t.Fatalf("MachineIdentity = %v, want %s", entry.MachineIdentity, daemonID)
	}

	var persistedRuntimeID *string
	var persistedCommandRunID *string
	if err := testPool.QueryRow(context.Background(), `
		SELECT runtime_id::text, command_run_id::text
		FROM preview_registry
		WHERE workspace_id = $1 AND preview_url = $2
	`, testWorkspaceID, previewServer.URL).Scan(&persistedRuntimeID, &persistedCommandRunID); err != nil {
		t.Fatalf("load persisted preview: %v", err)
	}
	if persistedRuntimeID == nil || *persistedRuntimeID != runtimeID {
		t.Fatalf("persisted runtime_id = %v, want %s", persistedRuntimeID, runtimeID)
	}
	if persistedCommandRunID != nil {
		t.Fatalf("persisted command_run_id = %v, want nil", *persistedCommandRunID)
	}
}

func TestReportRuntimePreview_RejectsSpoofedRuntimeIDPayload(t *testing.T) {
	previewServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer previewServer.Close()

	runtimeID, daemonID := createDaemonPreviewTestRuntime(t, "preview-report-daemon-2")
	req := newDaemonTokenRequest(http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/previews/report", map[string]any{
		"preview_url": previewServer.URL,
		"runtime_id":  "00000000-0000-0000-0000-000000000999",
	}, testWorkspaceID, daemonID)
	req = withURLParam(req, "runtimeId", runtimeID)

	w := httptest.NewRecorder()
	testHandler.ReportRuntimePreview(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ReportRuntimePreview status = %d, want 400: %s", w.Code, w.Body.String())
	}
}

func TestReportRuntimePreview_RejectsDaemonRuntimeMismatch(t *testing.T) {
	previewServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer previewServer.Close()

	runtimeID, _ := createDaemonPreviewTestRuntime(t, "preview-report-daemon-3")
	req := newDaemonTokenRequest(http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/previews/report", map[string]any{
		"preview_url": previewServer.URL,
	}, testWorkspaceID, "different-daemon-id")
	req = withURLParam(req, "runtimeId", runtimeID)

	w := httptest.NewRecorder()
	testHandler.ReportRuntimePreview(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("ReportRuntimePreview status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestReportRuntimePreview_RejectsCrossWorkspaceDaemon(t *testing.T) {
	previewServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer previewServer.Close()

	runtimeID, daemonID := createDaemonPreviewTestRuntime(t, "preview-report-daemon-4")
	req := newDaemonTokenRequest(http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/previews/report", map[string]any{
		"preview_url": previewServer.URL,
	}, "00000000-0000-0000-0000-000000000000", daemonID)
	req = withURLParam(req, "runtimeId", runtimeID)

	w := httptest.NewRecorder()
	testHandler.ReportRuntimePreview(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("ReportRuntimePreview status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestReportRuntimePreview_RequiresDaemonTokenAuthPath(t *testing.T) {
	previewServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer previewServer.Close()

	runtimeID, _ := createDaemonPreviewTestRuntime(t, "preview-report-daemon-5")
	req := newRequest(http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/previews/report", map[string]any{
		"preview_url": previewServer.URL,
	})
	req = withURLParam(req, "runtimeId", runtimeID)

	w := httptest.NewRecorder()
	testHandler.ReportRuntimePreview(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("ReportRuntimePreview status = %d, want 403: %s", w.Code, w.Body.String())
	}
}

func TestHandleCommandDeckPreviewRetire_HidesActiveRecordAndPreservesEvidence(t *testing.T) {
	previewServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer previewServer.Close()

	runtimeID, daemonID := createDaemonPreviewTestRuntime(t, "preview-retire-daemon-1")
	previewID := reportRuntimePreviewForTest(t, runtimeID, daemonID, previewServer.URL)

	req := newRequest(http.MethodPost, "/api/commandrunner/previews/"+previewID+"/retire", nil)
	req = withURLParam(req, "workspaceID", testWorkspaceID)
	req = withURLParam(req, "previewId", previewID)
	w := httptest.NewRecorder()
	testHandler.HandleCommandDeckPreviewRetire(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HandleCommandDeckPreviewRetire status = %d: %s", w.Code, w.Body.String())
	}

	var resp previewRegistryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Previews) != 0 {
		t.Fatalf("active previews length = %d, want 0 after retirement", len(resp.Previews))
	}

	var retiredAt *time.Time
	var retiredByType *string
	var retiredByID *string
	if err := testPool.QueryRow(context.Background(), `
		SELECT retired_at, retired_by_type, retired_by_id::text
		FROM preview_registry
		WHERE id = $1
	`, previewID).Scan(&retiredAt, &retiredByType, &retiredByID); err != nil {
		t.Fatalf("load retired preview: %v", err)
	}
	if retiredAt == nil || retiredAt.IsZero() {
		t.Fatalf("retired_at = %v, want non-null timestamp", retiredAt)
	}
	if retiredByType == nil || *retiredByType != "member" {
		t.Fatalf("retired_by_type = %v, want member", retiredByType)
	}
	if retiredByID == nil || *retiredByID != testUserID {
		t.Fatalf("retired_by_id = %v, want %s", retiredByID, testUserID)
	}
}

func TestHandleCommandDeckPreviewRetire_IsWorkspaceScoped(t *testing.T) {
	previewServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer previewServer.Close()

	runtimeID, daemonID := createDaemonPreviewTestRuntime(t, "preview-retire-daemon-2")
	previewID := reportRuntimePreviewForTest(t, runtimeID, daemonID, previewServer.URL)

	otherWorkspaceID := createAdditionalWorkspaceForPreviewTest(t)
	req := newRequest(http.MethodPost, "/api/commandrunner/previews/"+previewID+"/retire", nil)
	req = withURLParam(req, "workspaceID", otherWorkspaceID)
	req = withURLParam(req, "previewId", previewID)
	req.Header.Set("X-Workspace-ID", otherWorkspaceID)

	w := httptest.NewRecorder()
	testHandler.HandleCommandDeckPreviewRetire(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("HandleCommandDeckPreviewRetire status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestReportRuntimePreview_ReactivatesRetiredRecordDeterministically(t *testing.T) {
	previewServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer previewServer.Close()

	runtimeID, daemonID := createDaemonPreviewTestRuntime(t, "preview-retire-daemon-3")
	previewID := reportRuntimePreviewForTest(t, runtimeID, daemonID, previewServer.URL)

	retireReq := newRequest(http.MethodPost, "/api/commandrunner/previews/"+previewID+"/retire", nil)
	retireReq = withURLParam(retireReq, "workspaceID", testWorkspaceID)
	retireReq = withURLParam(retireReq, "previewId", previewID)
	retireW := httptest.NewRecorder()
	testHandler.HandleCommandDeckPreviewRetire(retireW, retireReq)
	if retireW.Code != http.StatusOK {
		t.Fatalf("HandleCommandDeckPreviewRetire status = %d: %s", retireW.Code, retireW.Body.String())
	}

	reactivatedID := reportRuntimePreviewForTest(t, runtimeID, daemonID, previewServer.URL)
	if reactivatedID != previewID {
		t.Fatalf("reactivated preview ID = %q, want stable ID %q", reactivatedID, previewID)
	}

	var retiredAt *time.Time
	if err := testPool.QueryRow(context.Background(), `
		SELECT retired_at
		FROM preview_registry
		WHERE id = $1
	`, previewID).Scan(&retiredAt); err != nil {
		t.Fatalf("load preview record after reactivation: %v", err)
	}
	if retiredAt != nil {
		t.Fatalf("retired_at = %v, want NULL after trusted runtime reactivation", retiredAt)
	}
}

func createDaemonPreviewTestRuntime(t *testing.T, daemonID string) (runtimeID string, normalizedDaemonID string) {
	t.Helper()

	normalizedDaemonID = strings.TrimSpace(daemonID)
	if normalizedDaemonID == "" {
		normalizedDaemonID = "preview-runtime-daemon"
	}

	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_runtime (
			workspace_id,
			daemon_id,
			name,
			runtime_mode,
			provider,
			status,
			device_info,
			metadata,
			last_seen_at
		)
		VALUES ($1, $2, 'Preview Test Runtime', 'local', 'codex', 'online', 'Preview Test Machine', '{}'::jsonb, now())
		RETURNING id
	`, testWorkspaceID, normalizedDaemonID).Scan(&runtimeID); err != nil {
		t.Fatalf("create preview test runtime: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), `
			DELETE FROM preview_registry WHERE runtime_id = $1
		`, runtimeID); err != nil {
			t.Fatalf("cleanup preview registry runtime rows: %v", err)
		}
		if _, err := testPool.Exec(context.Background(), `
			DELETE FROM agent_runtime WHERE id = $1
		`, runtimeID); err != nil {
			t.Fatalf("cleanup preview test runtime: %v", err)
		}
	})

	return runtimeID, normalizedDaemonID
}

func reportRuntimePreviewForTest(t *testing.T, runtimeID, daemonID, previewURL string) string {
	t.Helper()

	req := newDaemonTokenRequest(http.MethodPost, "/api/daemon/runtimes/"+runtimeID+"/previews/report", map[string]any{
		"preview_url": previewURL,
		"name":        "Runtime Reported Preview",
	}, testWorkspaceID, daemonID)
	req = withURLParam(req, "runtimeId", runtimeID)

	w := httptest.NewRecorder()
	testHandler.ReportRuntimePreview(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ReportRuntimePreview status = %d: %s", w.Code, w.Body.String())
	}

	var resp previewRegistryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Previews) != 1 {
		t.Fatalf("previews length = %d, want 1", len(resp.Previews))
	}
	if resp.Previews[0].ID == "" {
		t.Fatal("preview ID is empty")
	}
	return resp.Previews[0].ID
}

func createAdditionalWorkspaceForPreviewTest(t *testing.T) string {
	t.Helper()

	slug := fmt.Sprintf("preview-retire-test-%d", time.Now().UnixNano())
	var workspaceID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO workspace (name, slug, description, issue_prefix)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, "Preview Retire Test Workspace", slug, "temporary workspace for preview retirement tests", "PRV").Scan(&workspaceID); err != nil {
		t.Fatalf("create additional workspace: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`, workspaceID, testUserID); err != nil {
		t.Fatalf("create additional workspace member: %v", err)
	}

	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, workspaceID); err != nil {
			t.Fatalf("cleanup additional workspace: %v", err)
		}
	})

	return workspaceID
}

func requestPreviewRegistry(t *testing.T) previewRegistryResponse {
	t.Helper()

	req := newRequest(http.MethodGet, "/api/commandrunner/previews", nil)
	req = withURLParam(req, "workspaceID", testWorkspaceID)
	w := httptest.NewRecorder()

	testHandler.HandleCommandDeckPreviews(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HandleCommandDeckPreviews status = %d: %s", w.Code, w.Body.String())
	}

	var resp previewRegistryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func requestPreviewRegistrySync(t *testing.T) previewRegistryResponse {
	t.Helper()

	req := newRequest(http.MethodPost, "/api/commandrunner/previews/self-hosted/sync", nil)
	req = withURLParam(req, "workspaceID", testWorkspaceID)
	w := httptest.NewRecorder()

	testHandler.HandleCommandDeckPreviewSelfHostedSync(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HandleCommandDeckPreviewSelfHostedSync status = %d: %s", w.Code, w.Body.String())
	}

	var resp previewRegistryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}
