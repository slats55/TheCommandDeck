package handler

import (
	"context"
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
	HealthError      *string `json:"health_error"`
	LastCheckedAt    string  `json:"last_checked_at"`
	CommandRunID     *string `json:"command_run_id"`
	Command          *string `json:"command"`
	Source           string  `json:"source"`
}

type previewHealthProbe struct {
	Status     string
	StatusCode *int
	Error      *string
}

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

	previewURL := configuredPreviewURL()
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	health := probePreviewHealth(ctx, previewURL)
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
		PreviewURL:       previewURL,
		Port:             previewPort(previewURL),
		HealthStatus:     health.Status,
		HealthStatusCode: health.StatusCode,
		HealthError:      health.Error,
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

func probePreviewHealth(ctx context.Context, previewURL string) previewHealthProbe {
	checkURLs := previewHealthCheckURLs(previewURL)
	if len(checkURLs) == 0 {
		msg := "invalid preview URL"
		return previewHealthProbe{Status: "unknown", Error: &msg}
	}

	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr string
	for _, checkURL := range checkURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
		if err != nil {
			lastErr = "invalid preview URL"
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		defer resp.Body.Close()

		code := resp.StatusCode
		status := "healthy"
		if code >= http.StatusBadRequest {
			status = "unhealthy"
		}
		return previewHealthProbe{Status: status, StatusCode: &code}
	}

	if lastErr == "" {
		lastErr = "preview health check failed"
	}
	return previewHealthProbe{Status: "unhealthy", Error: &lastErr}
}

func previewHealthCheckURLs(previewURL string) []string {
	parsed, err := url.Parse(previewURL)
	if err != nil {
		return nil
	}
	checkURLs := []string{parsed.String()}
	host := parsed.Hostname()
	if (host == "localhost" || host == "127.0.0.1") && previewPort(previewURL) == 3000 {
		internal := *parsed
		internal.Host = net.JoinHostPort("commanddeck-web", "3000")
		checkURLs = []string{internal.String(), parsed.String()}
	}
	return checkURLs
}

func previewPort(previewURL string) int {
	parsed, err := url.Parse(previewURL)
	if err != nil {
		return 0
	}
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
