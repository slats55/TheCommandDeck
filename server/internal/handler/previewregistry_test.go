package handler

import (
	"context"
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
