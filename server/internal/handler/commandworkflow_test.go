package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func insertCommandRunForWorkflowTest(t *testing.T, workspaceID, runtimeID, initiatorID string) string {
	t.Helper()

	var runID string
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO command_run (
			workspace_id, template_id, runtime_id, issue_id,
			command, arguments, working_directory, status,
			initiator_type, initiator_id
		) VALUES (
			$1, NULL, $2, NULL,
			$3, '{}'::text[], $4, 'completed',
			'member', $5
		)
		RETURNING id
	`, workspaceID, runtimeID, "git status", "/tmp/workflow-test", initiatorID).Scan(&runID)
	if err != nil {
		t.Fatalf("insert command run: %v", err)
	}

	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM command_run WHERE id = $1`, runID)
	})

	return runID
}

func createSecondaryWorkflowTestWorkspace(t *testing.T) (workspaceID string, runtimeID string) {
	t.Helper()

	ctx := context.Background()
	slug := fmt.Sprintf("handler-tests-secondary-%d", time.Now().UnixNano())
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description, issue_prefix)
		VALUES ('Handler Tests Secondary', $1, 'secondary', 'HS2')
		RETURNING id
	`, slug).Scan(&workspaceID); err != nil {
		t.Fatalf("create secondary workspace: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, 'owner')
	`, workspaceID, testUserID); err != nil {
		t.Fatalf("add secondary workspace member: %v", err)
	}
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_runtime (
			workspace_id, daemon_id, name, runtime_mode, provider, status, device_info, metadata, last_seen_at
		)
		VALUES ($1, NULL, 'Secondary Runtime', 'cloud', 'handler_test_runtime', 'online', 'secondary runtime', '{}'::jsonb, now())
		RETURNING id
	`, workspaceID).Scan(&runtimeID); err != nil {
		t.Fatalf("create secondary runtime: %v", err)
	}

	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, workspaceID)
	})

	return workspaceID, runtimeID
}

func TestCommandWorkflowExecutionCreateAndList(t *testing.T) {
	runtimeID := handlerTestRuntimeID(t)
	commandRunID := insertCommandRunForWorkflowTest(t, testWorkspaceID, runtimeID, testUserID)

	createReq := newRequest(http.MethodPost, "/api/commandrunner/workflows", map[string]any{
		"title":          "Preview launch verification",
		"objective":      "Track BUILD->VERIFY handoff for preview registration",
		"command_run_id": commandRunID,
		"status":         "planned",
	})

	createW := httptest.NewRecorder()
	testHandler.HandleCommandWorkflowExecutionCreate(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create workflow status = %d: %s", createW.Code, createW.Body.String())
	}

	var created commandWorkflowExecutionResponse
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("decode create workflow response: %v", err)
	}
	if created.Title != "Preview launch verification" {
		t.Fatalf("created title = %q", created.Title)
	}
	if created.CommandRunID == nil || *created.CommandRunID != commandRunID {
		t.Fatalf("created command_run_id = %v, want %s", created.CommandRunID, commandRunID)
	}
	if created.Status != commandWorkflowStatusPlanned {
		t.Fatalf("created status = %q, want %q", created.Status, commandWorkflowStatusPlanned)
	}

	listReq := newRequest(http.MethodGet, "/api/commandrunner/workflows", nil)
	listW := httptest.NewRecorder()
	testHandler.HandleCommandWorkflowExecutionList(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list workflow status = %d: %s", listW.Code, listW.Body.String())
	}

	var listed commandWorkflowExecutionListResponse
	if err := json.NewDecoder(listW.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list workflow response: %v", err)
	}
	if listed.Total < 1 {
		t.Fatalf("listed total = %d, want >= 1", listed.Total)
	}
	if len(listed.WorkflowExecutions) < 1 {
		t.Fatalf("listed records length = %d, want >= 1", len(listed.WorkflowExecutions))
	}

	var found bool
	for _, item := range listed.WorkflowExecutions {
		if item.ID == created.ID {
			found = true
			if item.CommandRunStatus == nil || *item.CommandRunStatus != "completed" {
				t.Fatalf("command_run_status = %v, want completed", item.CommandRunStatus)
			}
			if item.CommandRun == nil || *item.CommandRun != "git status" {
				t.Fatalf("command_run = %v, want git status", item.CommandRun)
			}
		}
	}
	if !found {
		t.Fatalf("created workflow %s not found in list response", created.ID)
	}
}

func TestCommandWorkflowExecutionCreateRejectsCrossWorkspaceCommandRun(t *testing.T) {
	otherWorkspaceID, otherRuntimeID := createSecondaryWorkflowTestWorkspace(t)
	otherRunID := insertCommandRunForWorkflowTest(t, otherWorkspaceID, otherRuntimeID, testUserID)

	req := newRequest(http.MethodPost, "/api/commandrunner/workflows", map[string]any{
		"title":          "Cross workspace record attempt",
		"objective":      "Should not be allowed",
		"command_run_id": otherRunID,
	})

	w := httptest.NewRecorder()
	testHandler.HandleCommandWorkflowExecutionCreate(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("create workflow with cross-workspace command run status = %d, want 404: %s", w.Code, w.Body.String())
	}
}

func TestCommandWorkflowExecutionStatusUpdateIsWorkspaceScoped(t *testing.T) {
	runtimeID := handlerTestRuntimeID(t)
	commandRunID := insertCommandRunForWorkflowTest(t, testWorkspaceID, runtimeID, testUserID)

	createReq := newRequest(http.MethodPost, "/api/commandrunner/workflows", map[string]any{
		"title":          "Lifecycle status transition",
		"objective":      "Validate status changes",
		"command_run_id": commandRunID,
	})
	createW := httptest.NewRecorder()
	testHandler.HandleCommandWorkflowExecutionCreate(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create workflow status = %d: %s", createW.Code, createW.Body.String())
	}

	var created commandWorkflowExecutionResponse
	if err := json.NewDecoder(createW.Body).Decode(&created); err != nil {
		t.Fatalf("decode create workflow response: %v", err)
	}

	updateReq := newRequest(http.MethodPatch, "/api/commandrunner/workflows/"+created.ID+"/status", map[string]any{
		"status": commandWorkflowStatusRunning,
	})
	updateReq = withURLParam(updateReq, "workflowId", created.ID)
	updateW := httptest.NewRecorder()
	testHandler.HandleCommandWorkflowExecutionStatusUpdate(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("status update code = %d: %s", updateW.Code, updateW.Body.String())
	}

	var updated commandWorkflowExecutionResponse
	if err := json.NewDecoder(updateW.Body).Decode(&updated); err != nil {
		t.Fatalf("decode status update response: %v", err)
	}
	if updated.Status != commandWorkflowStatusRunning {
		t.Fatalf("updated status = %q, want %q", updated.Status, commandWorkflowStatusRunning)
	}

	crossWorkspaceReq := newRequest(http.MethodPatch, "/api/commandrunner/workflows/"+created.ID+"/status", map[string]any{
		"status": commandWorkflowStatusCompleted,
	})
	crossWorkspaceReq.Header.Set("X-Workspace-ID", "00000000-0000-0000-0000-000000000000")
	crossWorkspaceReq = withURLParam(crossWorkspaceReq, "workflowId", created.ID)
	crossWorkspaceW := httptest.NewRecorder()
	testHandler.HandleCommandWorkflowExecutionStatusUpdate(crossWorkspaceW, crossWorkspaceReq)
	if crossWorkspaceW.Code != http.StatusNotFound {
		t.Fatalf("cross workspace status update code = %d, want 404: %s", crossWorkspaceW.Code, crossWorkspaceW.Body.String())
	}
}
