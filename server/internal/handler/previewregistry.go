package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type previewRegistryResponse struct {
	Previews      []previewRegistryEntry `json:"previews"`
	LastCheckedAt string                 `json:"last_checked_at"`
}

type previewRegistryEntry struct {
	ID               string  `json:"id"`
	WorkspaceID      string  `json:"workspace_id"`
	WorkspaceName    string  `json:"workspace_name"`
	WorkspaceSlug    string  `json:"workspace_slug"`
	ProjectID        *string `json:"project_id"`
	ProjectName      *string `json:"project_name"`
	RuntimeID        *string `json:"runtime_id"`
	RuntimeName      *string `json:"runtime_name"`
	RuntimeStatus    *string `json:"runtime_status"`
	MachineIdentity  *string `json:"machine_identity"`
	PreviewURL       string  `json:"preview_url"`
	Port             int     `json:"port"`
	HealthStatus     string  `json:"health_status"`
	HealthStatusCode *int    `json:"health_status_code"`
	HealthMessage    *string `json:"health_message,omitempty"`
	HealthError      *string `json:"health_error,omitempty"`
	LastCheckedAt    string  `json:"last_checked_at"`
	CommandRunID     *string `json:"command_run_id"`
	Command          *string `json:"command"`
	Source           string  `json:"source"`
}

type previewHealthProbe struct {
	Status        string
	StatusCode    *int
	PublicMessage *string
}

type previewTarget struct {
	PublicURL string
	CheckURLs []string
	Port      int
}

const (
	previewHealthStatusHealthy     = "healthy"
	previewHealthStatusUnhealthy   = "unhealthy"
	previewHealthStatusUnavailable = "unavailable"
	previewHealthStatusUnknown     = "unknown"

	safePreviewUnavailableMessage = "Preview is currently unavailable."
	safePreviewUnhealthyMessage   = "Preview health check did not return a successful response."
	previewHealthTimeout          = 2 * time.Second
)

// HandleCommandDeckPreviews returns the real previews known to this running
// CommandDeck deployment. The first slice exposes the self-hosted web preview
// and probes it live instead of storing synthetic preview records.
func (h *Handler) HandleCommandDeckPreviews(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceID")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}

	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}

	ctx := r.Context()
	workspace, err := h.Queries.GetWorkspace(ctx, wsUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}

	runtimes, err := h.Queries.ListAgentRuntimes(ctx, wsUUID)
	if err != nil {
		slog.Error("list preview runtimes failed", "workspace_id", workspaceID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list preview registry")
		return
	}

	checkedAt := time.Now().UTC().Format(time.RFC3339)
	target, err := configuredPreviewTarget()
	if err != nil {
		slog.Warn("preview registry target rejected", "workspace_id", workspaceID, "error", err)
		writeJSON(w, http.StatusOK, previewRegistryResponse{
			Previews:      []previewRegistryEntry{},
			LastCheckedAt: checkedAt,
		})
		return
	}

	health := probePreviewHealth(ctx, target)
	runtime := selectPreviewRuntime(runtimes)

	entry := previewRegistryEntry{
		ID:               "self-hosted-web",
		WorkspaceID:      workspaceID,
		WorkspaceName:    workspace.Name,
		WorkspaceSlug:    workspace.Slug,
		ProjectID:        nil,
		ProjectName:      nil,
		RuntimeID:        runtimeID(runtime),
		RuntimeName:      runtimeName(runtime),
		RuntimeStatus:    runtimeStatus(runtime),
		MachineIdentity:  runtimeMachineIdentity(runtime),
		PreviewURL:       target.PublicURL,
		Port:             target.Port,
		HealthStatus:     health.Status,
		HealthStatusCode: health.StatusCode,
		HealthMessage:    health.PublicMessage,
		LastCheckedAt:    checkedAt,
		CommandRunID:     nil,
		Command:          nil,
		Source:           "self_hosted_stack",
	}

	writeJSON(w, http.StatusOK, previewRegistryResponse{
		Previews:      []previewRegistryEntry{entry},
		LastCheckedAt: checkedAt,
	})
}

func configuredPreviewURL() string {
	for _, key := range []string{"FRONTEND_ORIGIN", "MULTICA_APP_URL"} {
		if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
			return strings.TrimRight(raw, "/")
		}
	}
	return "http://localhost:3000"
}

func configuredPreviewTarget() (previewTarget, error) {
	return validatePreviewTarget(configuredPreviewURL())
}

func probePreviewHealth(ctx context.Context, target previewTarget) previewHealthProbe {
	client := newPreviewHealthClient()
	for _, checkURL := range target.CheckURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
		if err != nil {
			slog.Warn("preview health request creation failed", "error", err)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			slog.Warn("preview health request failed", "error", err)
			continue
		}
		defer resp.Body.Close()

		code := resp.StatusCode
		if code >= http.StatusMultipleChoices && code < http.StatusBadRequest {
			message := safePreviewUnavailableMessage
			slog.Warn("preview health redirect not followed", "status", code, "location", resp.Header.Get("Location"))
			return previewHealthProbe{
				Status:        previewHealthStatusUnavailable,
				StatusCode:    &code,
				PublicMessage: &message,
			}
		}

		status := previewHealthStatusHealthy
		var message *string
		if code >= http.StatusBadRequest {
			status = previewHealthStatusUnhealthy
			msg := safePreviewUnhealthyMessage
			message = &msg
		}
		return previewHealthProbe{Status: status, StatusCode: &code, PublicMessage: message}
	}

	message := safePreviewUnavailableMessage
	return previewHealthProbe{Status: previewHealthStatusUnavailable, PublicMessage: &message}
}

func newPreviewHealthClient() *http.Client {
	return &http.Client{
		Timeout: previewHealthTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func validatePreviewTarget(raw string) (previewTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return previewTarget{}, errors.New("preview URL is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return previewTarget{}, fmt.Errorf("parse preview URL: %w", err)
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return previewTarget{}, fmt.Errorf("unsupported preview URL scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return previewTarget{}, errors.New("preview URL host is required")
	}
	if parsed.User != nil {
		return previewTarget{}, errors.New("preview URL userinfo is not allowed")
	}
	if !isTrustedPreviewHost(parsed.Scheme, parsed.Hostname()) {
		return previewTarget{}, fmt.Errorf("preview URL host %q is not trusted", parsed.Hostname())
	}

	port := previewPortFromURL(parsed)
	checkURLs := []string{parsed.String()}
	host := parsed.Hostname()
	if isLocalPreviewHost(host) && port == 3000 {
		internal := *parsed
		internal.Host = net.JoinHostPort("commanddeck-web", "3000")
		checkURLs = []string{internal.String(), parsed.String()}
	}

	return previewTarget{
		PublicURL: strings.TrimRight(parsed.String(), "/"),
		CheckURLs: checkURLs,
		Port:      port,
	}, nil
}

func isTrustedPreviewHost(scheme, host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if isLocalPreviewHost(host) || host == "commanddeck-web" {
		return true
	}
	if scheme != "https" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !isUnsafePreviewIP(ip)
	}
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return false
	}
	return true
}

func isLocalPreviewHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func isUnsafePreviewIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

func previewPortFromURL(parsed *url.URL) int {
	if port := parsed.Port(); port != "" {
		if parsedPort, err := net.LookupPort("tcp", port); err == nil {
			return parsedPort
		}
	}
	switch parsed.Scheme {
	case "https":
		return 443
	case "http":
		return 80
	default:
		return 0
	}
}

func selectPreviewRuntime(runtimes []db.AgentRuntime) *db.AgentRuntime {
	if len(runtimes) == 0 {
		return nil
	}
	for i := range runtimes {
		if runtimes[i].Status == "online" {
			return &runtimes[i]
		}
	}
	return &runtimes[0]
}

func runtimeID(runtime *db.AgentRuntime) *string {
	if runtime == nil {
		return nil
	}
	value := uuidToString(runtime.ID)
	return &value
}

func runtimeName(runtime *db.AgentRuntime) *string {
	if runtime == nil || runtime.Name == "" {
		return nil
	}
	return &runtime.Name
}

func runtimeStatus(runtime *db.AgentRuntime) *string {
	if runtime == nil || runtime.Status == "" {
		return nil
	}
	return &runtime.Status
}

func runtimeMachineIdentity(runtime *db.AgentRuntime) *string {
	if runtime == nil {
		return nil
	}
	if runtime.DaemonID.Valid && strings.TrimSpace(runtime.DaemonID.String) != "" {
		value := runtime.DaemonID.String
		return &value
	}
	if strings.TrimSpace(runtime.DeviceInfo) != "" {
		value := runtime.DeviceInfo
		return &value
	}
	return runtimeName(runtime)
}
