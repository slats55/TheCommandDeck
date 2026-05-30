package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/daemonws"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func insertCommandRunForLiveEventTest(t *testing.T, status string) string {
	t.Helper()

	var runID string
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO command_run (
			workspace_id, template_id, runtime_id, issue_id,
			command, arguments, working_directory, status,
			initiator_type, initiator_id
		) VALUES (
			$1, NULL, $2, NULL,
			$3, '{}'::text[], $4, $5,
			'member', $6
		)
		RETURNING id
	`, testWorkspaceID, testRuntimeID, "git status", "/tmp", status, testUserID).Scan(&runID)
	if err != nil {
		t.Fatalf("insert command run: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM command_run WHERE id = $1`, runID)
	})
	return runID
}

func TestHandleDaemonCommandRunStartedWSMarksRunningAndPublishes(t *testing.T) {
	runID := insertCommandRunForLiveEventTest(t, "pending")

	eventCh := make(chan events.Event, 1)
	testHandler.Bus.Subscribe("command_run:updated", func(e events.Event) {
		select {
		case eventCh <- e:
		default:
		}
	})

	payload, _ := json.Marshal(protocol.CommandRunStartedPayload{
		CommandRunID: runID,
		Status:       "running",
	})
	err := testHandler.HandleDaemonCommandRunStartedWS(context.Background(), daemonws.ClientIdentity{
		WorkspaceID: testWorkspaceID,
		RuntimeIDs:  []string{testRuntimeID},
	}, testRuntimeID, payload)
	if err != nil {
		t.Fatalf("HandleDaemonCommandRunStartedWS: %v", err)
	}

	run, err := testHandler.Queries.GetCommandRun(context.Background(), parseUUID(runID))
	if err != nil {
		t.Fatalf("GetCommandRun: %v", err)
	}
	if run.Status != "running" {
		t.Fatalf("status = %q, want running", run.Status)
	}
	if !run.StartedAt.Valid {
		t.Fatal("started_at should be set")
	}

	select {
	case e := <-eventCh:
		payloadMap, ok := e.Payload.(map[string]any)
		if !ok {
			t.Fatalf("event payload type = %T, want map", e.Payload)
		}
		runPayload, ok := payloadMap["run"].(CommandRunnerRunResponse)
		if !ok {
			t.Fatalf("event run payload type = %T, want CommandRunnerRunResponse", payloadMap["run"])
		}
		if runPayload.ID != runID || runPayload.Status != "running" {
			t.Fatalf("unexpected run payload: %+v", runPayload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected command_run:updated event")
	}
}

func TestHandleDaemonCommandRunWSPublishesFinalEvent(t *testing.T) {
	runID := insertCommandRunForLiveEventTest(t, "running")

	eventCh := make(chan events.Event, 1)
	testHandler.Bus.Subscribe("command_run:updated", func(e events.Event) {
		select {
		case eventCh <- e:
		default:
		}
	})

	payload, _ := json.Marshal(protocol.CommandRunResultPayload{
		CommandRunID:    runID,
		Status:          "completed",
		ExitCode:        0,
		Stdout:          "ok",
		Stderr:          "",
		StdoutTruncated: false,
		StderrTruncated: false,
		DurationMs:      20,
	})
	err := testHandler.HandleDaemonCommandRunWS(context.Background(), daemonws.ClientIdentity{
		WorkspaceID: testWorkspaceID,
		RuntimeIDs:  []string{testRuntimeID},
	}, testRuntimeID, payload)
	if err != nil {
		t.Fatalf("HandleDaemonCommandRunWS: %v", err)
	}

	run, err := testHandler.Queries.GetCommandRun(context.Background(), parseUUID(runID))
	if err != nil {
		t.Fatalf("GetCommandRun: %v", err)
	}
	if run.Status != "completed" {
		t.Fatalf("status = %q, want completed", run.Status)
	}

	select {
	case e := <-eventCh:
		payloadMap, ok := e.Payload.(map[string]any)
		if !ok {
			t.Fatalf("event payload type = %T, want map", e.Payload)
		}
		runPayload, ok := payloadMap["run"].(CommandRunnerRunResponse)
		if !ok {
			t.Fatalf("event run payload type = %T, want CommandRunnerRunResponse", payloadMap["run"])
		}
		if runPayload.ID != runID || runPayload.Status != "completed" {
			t.Fatalf("unexpected run payload: %+v", runPayload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected command_run:updated event")
	}
}
