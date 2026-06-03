package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// withWorkflowTestWorkspaceCtx injects the workspace + member context that the
// real chi workspace middleware chain populates in production. The command
// workflow handlers read the workspace ID via workspaceIDFromURL, which prefers
// middleware.WorkspaceIDFromContext and only falls back to the chi URL param.
// These tests call the handlers directly (no middleware), so without this the
// handler sees an empty workspace and returns 400 "workspace_id is required"
// before reaching the authorization/scoping logic under test. This mirrors
// withChatTestWorkspaceCtx but is parameterized by workspace so the same helper
// can scope a request to a secondary workspace for cross-workspace assertions.
func withWorkflowTestWorkspaceCtx(t *testing.T, req *http.Request, workspaceID string) *http.Request {
	t.Helper()
	memberRow, err := testHandler.Queries.GetMemberByUserAndWorkspace(context.Background(), db.GetMemberByUserAndWorkspaceParams{
		UserID:      util.MustParseUUID(testUserID),
		WorkspaceID: util.MustParseUUID(workspaceID),
	})
	if err != nil {
		t.Fatalf("load member row for workspace %s: %v", workspaceID, err)
	}
	return req.WithContext(middleware.SetMemberContext(req.Context(), workspaceID, memberRow))
}

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
	createReq = withWorkflowTestWorkspaceCtx(t, createReq, testWorkspaceID)

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
	listReq = withWorkflowTestWorkspaceCtx(t, listReq, testWorkspaceID)
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
	// Scope the request to the primary workspace (where the caller is a member).
	// The rejection must come from the command_run belonging to a *different*
	// workspace, not from a missing workspace context.
	req = withWorkflowTestWorkspaceCtx(t, req, testWorkspaceID)

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
	createReq = withWorkflowTestWorkspaceCtx(t, createReq, testWorkspaceID)
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
	updateReq = withWorkflowTestWorkspaceCtx(t, updateReq, testWorkspaceID)
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

	// Scope the same workflow ID to a *different* workspace the caller also
	// belongs to. The workspace-scoped UPDATE must match no row (the workflow
	// lives in the primary workspace), proving status updates are workspace
	// scoped at the query level rather than only at the membership gate.
	otherWorkspaceID, _ := createSecondaryWorkflowTestWorkspace(t)
	crossWorkspaceReq := newRequest(http.MethodPatch, "/api/commandrunner/workflows/"+created.ID+"/status", map[string]any{
		"status": commandWorkflowStatusCompleted,
	})
	crossWorkspaceReq = withURLParam(crossWorkspaceReq, "workflowId", created.ID)
	crossWorkspaceReq = withWorkflowTestWorkspaceCtx(t, crossWorkspaceReq, otherWorkspaceID)
	crossWorkspaceW := httptest.NewRecorder()
	testHandler.HandleCommandWorkflowExecutionStatusUpdate(crossWorkspaceW, crossWorkspaceReq)
	if crossWorkspaceW.Code != http.StatusNotFound {
		t.Fatalf("cross workspace status update code = %d, want 404: %s", crossWorkspaceW.Code, crossWorkspaceW.Body.String())
	}
}
