package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/daemonws"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// CommandRunnerRunRequest is the API request for running a command.
type CommandRunnerRunRequest struct {
	RuntimeID  string `json:"runtime_id"`
	TemplateID string `json:"template_id,omitempty"`
	IssueID    string `json:"issue_id,omitempty"`
}

// CommandRunnerRunResponse is the API response after creating a command run.
type CommandRunnerRunResponse struct {
	ID               string  `json:"id"`
	Status           string  `json:"status"`
	Command          string  `json:"command"`
	WorkingDirectory string  `json:"working_directory"`
	ExitCode         *int    `json:"exit_code,omitempty"`
	Stdout           *string `json:"stdout,omitempty"`
	Stderr           *string `json:"stderr,omitempty"`
	DurationMs       *int    `json:"duration_ms,omitempty"`
	StartedAt        *string `json:"started_at,omitempty"`
	FinishedAt       *string `json:"finished_at,omitempty"`
	CreatedAt        string  `json:"created_at"`
}

// CommandRunnerRunListResponse is the API response for listing command runs.
type CommandRunnerRunListResponse struct {
	CommandRuns []CommandRunnerRunResponse `json:"command_runs"`
	Total       int                        `json:"total"`
}

// commandRunToResponse converts a db.CommandRun to CommandRunnerRunResponse.
func commandRunToResponse(run db.CommandRun) CommandRunnerRunResponse {
	resp := CommandRunnerRunResponse{
		ID:               uuidToString(run.ID),
		Status:           run.Status,
		Command:          run.Command,
		WorkingDirectory: run.WorkingDirectory,
		CreatedAt:        timestampToString(run.CreatedAt),
	}
	if run.ExitCode.Valid {
		resp.ExitCode = (*int32)(&run.ExitCode.Int)
	}
	if run.Stdout.Valid {
		resp.Stdout = &run.Stdout.String
	}
	if run.Stderr.Valid {
		resp.Stderr = &run.Stderr.String
	}
	if run.DurationMs.Valid {
		resp.DurationMs = (*int32)(&run.DurationMs.Int)
	}
	if run.StartedAt.Valid {
		resp.StartedAt = timestampToPtr(run.StartedAt)
	}
	if run.FinishedAt.Valid {
		resp.FinishedAt = timestampToPtr(run.FinishedAt)
	}
	return resp
}

// RequireCommandDeckRuntime verifies the runtime belongs to the workspace and is active.
func (h *Handler) RequireCommandDeckRuntime(w http.ResponseWriter, r *http.Request, workspaceID, runtimeID string) (db.AgentRuntime, bool) {
	if runtimeID == "" {
		writeError(w, http.StatusBadRequest, "runtime_id is required")
		return db.AgentRuntime{}, false
	}

	runtimeUUID, ok := parseUUIDOrBadRequest(w, runtimeID, "runtime_id")
	if !ok {
		return db.AgentRuntime{}, false
	}

	ctx := r.Context()
	rt, err := h.Queries.GetAgentRuntime(ctx, runtimeUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "runtime not found")
		return db.AgentRuntime{}, false
	}

	if uuidToString(rt.WorkspaceID) != workspaceID {
		writeError(w, http.StatusNotFound, "runtime not found")
		return db.AgentRuntime{}, false
	}

	// Runtime must be in an active state to receive commands.
	if rt.Status != "online" && rt.Status != "busy" {
		writeError(w, http.StatusBadRequest, "runtime is not online")
		return db.AgentRuntime{}, false
	}

	return rt, true
}

// handleCommandRunnerRun dispatches a command execution request to the daemon
// and creates a command_run record.
func (h *Handler) handleCommandRunnerRun(w http.ResponseWriter, r *http.Request, workspaceID string) {
	var req CommandRunnerRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RuntimeID == "" {
		writeError(w, http.StatusBadRequest, "runtime_id is required")
		return
	}

	rt, ok := h.RequireCommandDeckRuntime(w, r, workspaceID, req.RuntimeID)
	if !ok {
		return
	}

	ctx := r.Context()
	userID := requestUserID(r)

	// Determine initiator — if X-Agent-ID + X-Task-ID headers are both present,
	// the caller is an agent; otherwise it's a human member.
	initiatorType := "member"
	initiatorID := userID
	if agentID := r.Header.Get("X-Agent-ID"); agentID != "" && r.Header.Get("X-Task-ID") != "" {
		initiatorType = "agent"
		initiatorID = agentID
	}

	// Look up template: if template_id is provided, validate it belongs to the workspace;
	// otherwise fall back to the built-in "Git Status" template.
	var templateID pgtype.UUID
	var command string
	var workingDirBound string

	if req.TemplateID != "" {
		templateID, ok := parseUUIDOrBadRequest(w, req.TemplateID, "template_id")
		if !ok {
			return
		}
		tpl, err := h.Queries.GetCommandTemplate(ctx, templateID)
		if err != nil {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		if uuidToString(tpl.WorkspaceID) != workspaceID {
			writeError(w, http.StatusNotFound, "template not found")
			return
		}
		command = tpl.Command
		if tpl.WorkingDirBound.Valid {
			workingDirBound = tpl.WorkingDirBound.String
		}
	} else {
		// Default to the built-in Git Status template.
		wsUUID := util.MustParseUUID(workspaceID)
		tpl, err := h.Queries.GetCommandTemplateByName(ctx, db.GetCommandTemplateByNameParams{
			WorkspaceID: wsUUID,
			Name:        "Git Status",
		})
		if err != nil {
			slog.Warn("git status template not found for workspace", "workspace_id", workspaceID)
			writeError(w, http.StatusBadRequest, "git status template not found in this workspace")
			return
		}
		templateID = tpl.ID
		command = tpl.Command
		if tpl.WorkingDirBound.Valid {
			workingDirBound = tpl.WorkingDirBound.String
		}
	}

	// Determine working directory for the command — use the runtime's worktree root
	// from runtime metadata (key: "worktree_root"). If not set, leave empty (daemon
	// will use a default). Validate it against working_dir_bound if set.
	worktreeRoot := worktreeRootFromMetadata(rt.Metadata)
	if workingDirBound != "" && worktreeRoot != "" && !isPrefixedBy(worktreeRoot, workingDirBound) {
		writeError(w, http.StatusBadRequest, "runtime working directory is not within allowed bound")
		return
	}

	// Parse optional issue_id.
	var issueID pgtype.UUID
	if req.IssueID != "" {
		var ok bool
		issueID, ok = parseUUIDOrBadRequest(w, req.IssueID, "issue_id")
		if !ok {
			return
		}
	}

	runtimeUUID := util.MustParseUUID(req.RuntimeID)
	wsUUID := util.MustParseUUID(workspaceID)
	initiatorUUID := util.MustParseUUID(initiatorID)

	// Create command_run record in pending state.
	run, err := h.Queries.CreateCommandRun(ctx, db.CreateCommandRunParams{
		WorkspaceID:     wsUUID,
		TemplateID:      templateID,
		RuntimeID:       runtimeUUID,
		IssueID:         issueID,
		Command:         command,
		Arguments:       []string{},
		WorkingDirectory: worktreeRoot,
		Status:          "pending",
		InitiatorType:   initiatorType,
		InitiatorID:     initiatorUUID,
	})
	if err != nil {
		slog.Error("create command run failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create command run")
		return
	}

	// Send execution request to daemon via daemon WebSocket.
	// DaemonHub.DeliverDaemonRuntime sends the frame to all clients watching this runtime.
	if h.DaemonHub != nil {
		payload := protocol.CommandRunExecutePayload{
			CommandRunID:  uuidToString(run.ID),
			RuntimeID:     req.RuntimeID,
			Command:       command,
			WorkingDir:    worktreeRoot,
			WorkspaceID:   workspaceID,
			InitiatorType: initiatorType,
			InitiatorID:   initiatorID,
			IssueID:       req.IssueID,
		}
		payloadBytes, _ := json.Marshal(payload)
		frame := protocol.Message{
			Type:    protocol.CommandRunExecute,
			Payload: payloadBytes,
		}
		frameBytes, _ := json.Marshal(frame)
		h.DaemonHub.DeliverDaemonRuntime(req.RuntimeID, frameBytes, "")
	}

	writeJSON(w, http.StatusCreated, commandRunToResponse(run))
}

// handleCommandRunnerGet returns a single command run by ID.
func (h *Handler) handleCommandRunnerGet(w http.ResponseWriter, r *http.Request, workspaceID string) {
	runID := chi.URLParam(r, "runId")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run_id is required")
		return
	}

	runUUID, ok := parseUUIDOrBadRequest(w, runID, "run_id")
	if !ok {
		return
	}

	ctx := r.Context()
	run, err := h.Queries.GetCommandRun(ctx, runUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "command run not found")
		return
	}

	if uuidToString(run.WorkspaceID) != workspaceID {
		writeError(w, http.StatusNotFound, "command run not found")
		return
	}

	writeJSON(w, http.StatusOK, commandRunToResponse(run))
}

// handleCommandRunnerList returns command runs for the workspace.
func (h *Handler) handleCommandRunnerList(w http.ResponseWriter, r *http.Request, workspaceID string) {
	ctx := r.Context()
	wsUUID := util.MustParseUUID(workspaceID)

	runs, err := h.Queries.ListCommandRuns(ctx, wsUUID)
	if err != nil {
		slog.Error("list command runs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list command runs")
		return
	}

	resp := CommandRunnerRunListResponse{
		CommandRuns: make([]CommandRunnerRunResponse, 0, len(runs)),
		Total:       len(runs),
	}
	for _, run := range runs {
		resp.CommandRuns = append(resp.CommandRuns, commandRunToResponse(run))
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleCommandRunnerTemplates returns available command templates for the workspace.
func (h *Handler) handleCommandRunnerTemplates(w http.ResponseWriter, r *http.Request, workspaceID string) {
	ctx := r.Context()
	wsUUID := util.MustParseUUID(workspaceID)

	templates, err := h.Queries.ListCommandTemplates(ctx, wsUUID)
	if err != nil {
		slog.Error("list command templates failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list templates")
		return
	}

	type TemplateResponse struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Command     string  `json:"command"`
		Description *string `json:"description,omitempty"`
		Category    string  `json:"category"`
		RiskLevel   string  `json:"risk_level"`
		IsBuiltin bool    `json:"is_builtin"`
		CreatedAt   string  `json:"created_at"`
	}

	resp := make([]TemplateResponse, 0, len(templates))
	for _, t := range templates {
		r := TemplateResponse{
			ID:        uuidToString(t.ID),
			Name:      t.Name,
			Command:   t.Command,
			Category:  t.Category,
			RiskLevel: t.RiskLevel,
			IsBuiltin: t.IsBuiltin,
			CreatedAt: timestampToString(t.CreatedAt),
		}
		if t.Description.Valid {
			r.Description = &t.Description.String
		}
		resp = append(resp, r)
	}

	writeJSON(w, http.StatusOK, map[string]any{"templates": resp})
}

// HandleDaemonCommandRunWS processes an inbound command_run:result frame from a
// daemon connected via the daemon WebSocket. It updates the command_run record.
func (h *Handler) HandleDaemonCommandRunWS(ctx context.Context, identity daemonws.ClientIdentity, runtimeID string, payload json.RawMessage) error {
	var result protocol.CommandRunResultPayload
	if err := json.Unmarshal(payload, &result); err != nil {
		slog.Debug("HandleDaemonCommandRunWS: failed to unmarshal payload", "error", err)
		return err
	}

	runID, err := util.ParseUUID(result.CommandRunID)
	if err != nil {
		slog.Debug("HandleDaemonCommandRunWS: invalid run_id", "run_id", result.CommandRunID)
		return err
	}

	var exitCode pgtype.Int4
	if result.ExitCode >= 0 {
		exitCode = pgtype.Int4{Int: int32(result.ExitCode), Valid: true}
	}

	now := time.Now()
	var finishedAt pgtype.Timestamptz
	finishedAt.Time = now
	finishedAt.Valid = true

	var startedAt pgtype.Timestamptz
	if result.DurationMs > 0 {
		startedAt.Time = now.Add(-time.Duration(result.DurationMs) * time.Millisecond)
		startedAt.Valid = true
	}

	var durationMs pgtype.Int4
	if result.DurationMs >= 0 {
		durationMs = pgtype.Int4{Int: int32(result.DurationMs), Valid: true}
	}

	_, err = h.Queries.UpdateCommandRunResult(ctx, db.UpdateCommandRunResultParams{
		ID:         runID,
		Status:     result.Status,
		ExitCode:   exitCode,
		Stdout:     toText(result.Stdout),
		Stderr:     toText(result.Stderr),
		FinishedAt: finishedAt,
		DurationMs: durationMs,
		StartedAt:  startedAt,
	})
	if err != nil {
		slog.Error("UpdateCommandRunResult failed", "run_id", result.CommandRunID, "error", err)
		return err
	}

	slog.Info("command run completed", "run_id", result.CommandRunID, "status", result.Status)
	return nil
}

// toText converts a string to a nullable pgtype.Text.
func toText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// isPrefixedBy returns true if s starts with prefix.
func isPrefixedBy(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// worktreeRootFromMetadata extracts worktree_root from runtime metadata JSON.
func worktreeRootFromMetadata(meta json.RawMessage) string {
	if len(meta) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(meta, &m) != nil {
		return ""
	}
	if root, ok := m["worktree_root"]; ok {
		if s, ok := root.(string); ok && s != "" {
			return s
		}
	}
	return ""
}