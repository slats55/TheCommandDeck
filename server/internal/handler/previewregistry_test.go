package handler

import (
	"context"
	"encoding/json"
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
