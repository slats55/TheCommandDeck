package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	commandWorkflowStatusPlanned     = "planned"
	commandWorkflowStatusRunning     = "running"
	commandWorkflowStatusNeedsReview = "needs_review"
	commandWorkflowStatusCompleted   = "completed"
	commandWorkflowStatusFailed      = "failed"
	commandWorkflowStatusCancelled   = "cancelled"
)

type commandWorkflowExecutionResponse struct {
	ID               string  `json:"id"`
	WorkspaceID      string  `json:"workspace_id"`
	ProjectID        *string `json:"project_id,omitempty"`
	ProjectTitle     *string `json:"project_title,omitempty"`
	CommandRunID     *string `json:"command_run_id,omitempty"`
	CommandRunStatus *string `json:"command_run_status,omitempty"`
	CommandRun       *string `json:"command_run,omitempty"`
	Title            string  `json:"title"`
	Objective        string  `json:"objective"`
	Status           string  `json:"status"`
	CreatedByType    string  `json:"created_by_type"`
	CreatedByID      string  `json:"created_by_id"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

type commandWorkflowExecutionListResponse struct {
	WorkflowExecutions []commandWorkflowExecutionResponse `json:"workflow_executions"`
	Total              int                                `json:"total"`
}

type createCommandWorkflowExecutionRequest struct {
	ProjectID    string `json:"project_id,omitempty"`
	CommandRunID string `json:"command_run_id,omitempty"`
	Title        string `json:"title"`
	Objective    string `json:"objective"`
	Status       string `json:"status,omitempty"`
}

type updateCommandWorkflowExecutionStatusRequest struct {
	Status string `json:"status"`
}

func isValidCommandWorkflowStatus(status string) bool {
	switch status {
	case commandWorkflowStatusPlanned,
		commandWorkflowStatusRunning,
		commandWorkflowStatusNeedsReview,
		commandWorkflowStatusCompleted,
		commandWorkflowStatusFailed,
		commandWorkflowStatusCancelled:
		return true
	default:
		return false
	}
}

func commandWorkflowExecutionRowToResponse(row db.GetCommandWorkflowExecutionRow) commandWorkflowExecutionResponse {
	return commandWorkflowExecutionResponse{
		ID:               uuidToString(row.ID),
		WorkspaceID:      uuidToString(row.WorkspaceID),
		ProjectID:        uuidToPtr(row.ProjectID),
		ProjectTitle:     textToPtr(row.ProjectTitle),
		CommandRunID:     uuidToPtr(row.CommandRunID),
		CommandRunStatus: textToPtr(row.CommandRunStatus),
		CommandRun:       textToPtr(row.CommandRunCommand),
		Title:            row.Title,
		Objective:        row.Objective,
		Status:           row.Status,
		CreatedByType:    row.CreatedByType,
		CreatedByID:      uuidToString(row.CreatedByID),
		CreatedAt:        timestampToString(row.CreatedAt),
		UpdatedAt:        timestampToString(row.UpdatedAt),
	}
}

func commandWorkflowExecutionListRowToResponse(row db.ListCommandWorkflowExecutionsRow) commandWorkflowExecutionResponse {
	return commandWorkflowExecutionResponse{
		ID:               uuidToString(row.ID),
		WorkspaceID:      uuidToString(row.WorkspaceID),
		ProjectID:        uuidToPtr(row.ProjectID),
		ProjectTitle:     textToPtr(row.ProjectTitle),
		CommandRunID:     uuidToPtr(row.CommandRunID),
		CommandRunStatus: textToPtr(row.CommandRunStatus),
		CommandRun:       textToPtr(row.CommandRunCommand),
		Title:            row.Title,
		Objective:        row.Objective,
		Status:           row.Status,
		CreatedByType:    row.CreatedByType,
		CreatedByID:      uuidToString(row.CreatedByID),
		CreatedAt:        timestampToString(row.CreatedAt),
		UpdatedAt:        timestampToString(row.UpdatedAt),
	}
}

func (h *Handler) HandleCommandWorkflowExecutionCreate(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceID")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}

	var req createCommandWorkflowExecutionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if len(req.Title) > 200 {
		writeError(w, http.StatusBadRequest, "title must be 200 characters or fewer")
		return
	}
	req.Objective = strings.TrimSpace(req.Objective)
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = commandWorkflowStatusPlanned
	}
	if !isValidCommandWorkflowStatus(status) {
		writeError(w, http.StatusBadRequest, "status is invalid")
		return
	}

	var projectUUID pgtype.UUID
	if strings.TrimSpace(req.ProjectID) != "" {
		projectID, ok := parseUUIDOrBadRequest(w, req.ProjectID, "project_id")
		if !ok {
			return
		}
		_, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{
			ID:          projectID,
			WorkspaceID: workspaceUUID,
		})
		if err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		projectUUID = projectID
	}

	var commandRunUUID pgtype.UUID
	if strings.TrimSpace(req.CommandRunID) != "" {
		runID, ok := parseUUIDOrBadRequest(w, req.CommandRunID, "command_run_id")
		if !ok {
			return
		}
		run, err := h.Queries.GetCommandRun(r.Context(), runID)
		if err != nil || uuidToString(run.WorkspaceID) != workspaceID {
			writeError(w, http.StatusNotFound, "command run not found")
			return
		}
		commandRunUUID = runID
	}

	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	createdByType, createdByID := h.resolveActor(r, userID, workspaceID)
	createdByUUID, err := util.ParseUUID(createdByID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "actor_id is invalid")
		return
	}

	created, err := h.Queries.CreateCommandWorkflowExecution(r.Context(), db.CreateCommandWorkflowExecutionParams{
		WorkspaceID:   workspaceUUID,
		ProjectID:     projectUUID,
		CommandRunID:  commandRunUUID,
		Title:         req.Title,
		Objective:     req.Objective,
		Status:        status,
		CreatedByType: createdByType,
		CreatedByID:   createdByUUID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workflow execution")
		return
	}

	row, err := h.Queries.GetCommandWorkflowExecution(r.Context(), db.GetCommandWorkflowExecutionParams{
		ID:          created.ID,
		WorkspaceID: workspaceUUID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load workflow execution")
		return
	}

	writeJSON(w, http.StatusCreated, commandWorkflowExecutionRowToResponse(row))
}

func (h *Handler) HandleCommandWorkflowExecutionList(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceID")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}

	rows, err := h.Queries.ListCommandWorkflowExecutions(r.Context(), workspaceUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workflow executions")
		return
	}

	resp := commandWorkflowExecutionListResponse{
		WorkflowExecutions: make([]commandWorkflowExecutionResponse, 0, len(rows)),
		Total:              len(rows),
	}
	for _, row := range rows {
		resp.WorkflowExecutions = append(resp.WorkflowExecutions, commandWorkflowExecutionListRowToResponse(row))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) HandleCommandWorkflowExecutionGet(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceID")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}

	workflowID := strings.TrimSpace(chi.URLParam(r, "workflowId"))
	if workflowID == "" {
		writeError(w, http.StatusBadRequest, "workflow_id is required")
		return
	}
	workflowUUID, ok := parseUUIDOrBadRequest(w, workflowID, "workflow_id")
	if !ok {
		return
	}

	row, err := h.Queries.GetCommandWorkflowExecution(r.Context(), db.GetCommandWorkflowExecutionParams{
		ID:          workflowUUID,
		WorkspaceID: workspaceUUID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow execution not found")
		return
	}
	writeJSON(w, http.StatusOK, commandWorkflowExecutionRowToResponse(row))
}

func (h *Handler) HandleCommandWorkflowExecutionStatusUpdate(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceID")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}

	workflowID := strings.TrimSpace(chi.URLParam(r, "workflowId"))
	if workflowID == "" {
		writeError(w, http.StatusBadRequest, "workflow_id is required")
		return
	}
	workflowUUID, ok := parseUUIDOrBadRequest(w, workflowID, "workflow_id")
	if !ok {
		return
	}

	var req updateCommandWorkflowExecutionStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status := strings.TrimSpace(req.Status)
	if !isValidCommandWorkflowStatus(status) {
		writeError(w, http.StatusBadRequest, "status is invalid")
		return
	}

	updated, err := h.Queries.UpdateCommandWorkflowExecutionStatus(r.Context(), db.UpdateCommandWorkflowExecutionStatusParams{
		Status:      status,
		ID:          workflowUUID,
		WorkspaceID: workspaceUUID,
	})
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "workflow execution not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update workflow execution")
		return
	}

	row, err := h.Queries.GetCommandWorkflowExecution(r.Context(), db.GetCommandWorkflowExecutionParams{
		ID:          updated.ID,
		WorkspaceID: workspaceUUID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load workflow execution")
		return
	}
	writeJSON(w, http.StatusOK, commandWorkflowExecutionRowToResponse(row))
}
