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

	"github.com/jackc/pgx/v5/pgtype"
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
	LastSuccessAt    *string `json:"last_success_at,omitempty"`
	RegisteredAt     string  `json:"registered_at"`
	UpdatedAt        string  `json:"updated_at"`
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

// HandleCommandDeckPreviews returns persisted trusted preview records and does
// not mutate durable registry state.
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

	entries, lastCheckedAt, err := h.listPreviewRegistryEntries(ctx, wsUUID, workspaceID, workspace.Name, workspace.Slug)
	if err != nil {
		slog.Error("list preview registry records failed", "workspace_id", workspaceID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list preview registry")
		return
	}

	writeJSON(w, http.StatusOK, previewRegistryResponse{
		Previews:      entries,
		LastCheckedAt: lastCheckedAt,
	})
}

// HandleCommandDeckPreviewSelfHostedSync explicitly refreshes the trusted
// self-hosted preview registry record. No client URL input is accepted.
func (h *Handler) HandleCommandDeckPreviewSelfHostedSync(w http.ResponseWriter, r *http.Request) {
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

	target, err := configuredPreviewTarget()
	if err != nil {
		slog.Warn("preview registry target rejected", "workspace_id", workspaceID, "error", err)
		writeError(w, http.StatusBadRequest, "trusted preview target is unavailable")
		return
	}

	healthCheckedAt, health, err := h.upsertSelfHostedPreviewRecord(ctx, wsUUID, target)
	if err != nil {
		slog.Error("upsert self-hosted preview registry record failed", "workspace_id", workspaceID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to refresh preview registry")
		return
	}

	entries, _, err := h.listPreviewRegistryEntries(ctx, wsUUID, workspaceID, workspace.Name, workspace.Slug)
	if err != nil {
		slog.Error("list preview registry records failed", "workspace_id", workspaceID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list preview registry")
		return
	}
	checkedAt := healthCheckedAt.Format(time.RFC3339)
	for i := range entries {
		if entries[i].PreviewURL == target.PublicURL && entries[i].Source == "self_hosted_stack" {
			entries[i].HealthStatusCode = health.StatusCode
			entries[i].HealthMessage = health.PublicMessage
			entries[i].LastCheckedAt = checkedAt
			if health.Status == previewHealthStatusHealthy {
				entries[i].LastSuccessAt = &checkedAt
			}
		}
	}

	writeJSON(w, http.StatusOK, previewRegistryResponse{
		Previews:      entries,
		LastCheckedAt: checkedAt,
	})
}

func (h *Handler) listPreviewRegistryEntries(ctx context.Context, workspaceUUID pgtype.UUID, workspaceID, workspaceName, workspaceSlug string) ([]previewRegistryEntry, string, error) {
	records, err := h.Queries.ListPreviewRegistryRecords(ctx, workspaceUUID)
	if err != nil {
		return nil, "", err
	}
	entries := make([]previewRegistryEntry, 0, len(records))
	var latest time.Time
	for _, record := range records {
		if record.LastCheckedAt.Valid && record.LastCheckedAt.Time.After(latest) {
			latest = record.LastCheckedAt.Time
		}
		entries = append(entries, previewRegistryEntryFromRecord(record, workspaceID, workspaceName, workspaceSlug))
	}
	lastCheckedAt := ""
	if !latest.IsZero() {
		lastCheckedAt = latest.UTC().Format(time.RFC3339)
	}
	return entries, lastCheckedAt, nil
}

func (h *Handler) upsertSelfHostedPreviewRecord(ctx context.Context, wsUUID pgtype.UUID, target previewTarget) (time.Time, previewHealthProbe, error) {
	checkedAtTime := time.Now().UTC()
	health := probePreviewHealth(ctx, target)
	var lastSuccessAt pgtype.Timestamptz
	if health.Status == previewHealthStatusHealthy {
		lastSuccessAt = pgtype.Timestamptz{Time: checkedAtTime, Valid: true}
	}
	_, err := h.Queries.UpsertPreviewRegistryRecord(ctx, db.UpsertPreviewRegistryRecordParams{
		WorkspaceID:   wsUUID,
		RuntimeID:     pgtype.UUID{},
		CommandRunID:  pgtype.UUID{},
		Name:          "Self-hosted web preview",
		PreviewUrl:    target.PublicURL,
		Port:          int32(target.Port),
		Source:        "self_hosted_stack",
		Status:        health.Status,
		LastCheckedAt: pgtype.Timestamptz{Time: checkedAtTime, Valid: true},
		LastSuccessAt: lastSuccessAt,
	})
	if err != nil {
		return time.Time{}, previewHealthProbe{}, err
	}
	return checkedAtTime, health, nil
}

func previewRegistryEntryFromRecord(row db.ListPreviewRegistryRecordsRow, workspaceID, workspaceName, workspaceSlug string) previewRegistryEntry {
	return previewRegistryEntry{
		ID:               uuidToString(row.ID),
		WorkspaceID:      workspaceID,
		WorkspaceName:    workspaceName,
		WorkspaceSlug:    workspaceSlug,
		ProjectID:        nil,
		ProjectName:      nil,
		RuntimeID:        uuidToPtr(row.RuntimeID),
		RuntimeName:      textToPtr(row.RuntimeName),
		RuntimeStatus:    textToPtr(row.RuntimeStatus),
		MachineIdentity:  textToPtr(row.RuntimeDaemonID),
		PreviewURL:       row.PreviewUrl,
		Port:             int(row.Port),
		HealthStatus:     row.Status,
		HealthStatusCode: nil,
		HealthMessage:    nil,
		LastCheckedAt:    timestampToString(row.LastCheckedAt),
		LastSuccessAt:    timestampToPtr(row.LastSuccessAt),
		RegisteredAt:     timestampToString(row.CreatedAt),
		UpdatedAt:        timestampToString(row.UpdatedAt),
		CommandRunID:     uuidToPtr(row.CommandRunID),
		Command:          nil,
		Source:           row.Source,
	}
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
