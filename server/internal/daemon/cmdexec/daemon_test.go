package cmdexec

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os/exec"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

func TestWebSocketHandlerCancelBeforeExecuteYieldsCancelled(t *testing.T) {
	send := make(chan []byte, 1)
	h := NewWebSocketHandler(t.TempDir(), send, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cancelPayload, _ := json.Marshal(protocol.CommandRunCancelPayload{
		CommandRunID: "run-1",
		RuntimeID:    "runtime-1",
	})
	h.HandleCancel(cancelPayload)

	execPayload, _ := json.Marshal(CommandRunExecutePayload{
		CommandRunID: "run-1",
		Command:      "git status",
	})
	h.Handle(execPayload)

	result := recvResult(t, send)
	if result.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %q", result.Status)
	}
}

func TestWebSocketHandlerCancelActiveRunYieldsCancelled(t *testing.T) {
	send := make(chan []byte, 2)
	h := NewWebSocketHandler(t.TempDir(), send, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.executor.runFn = func(ctx context.Context, _ *exec.Cmd, _, _ int) (string, string, int, bool, bool, error) {
		<-ctx.Done()
		return "", "", 1, false, false, ctx.Err()
	}
	h.executor.allowedCommands[commandKey([]string{"fakecmd", "status"})] = true
	execPayload, _ := json.Marshal(CommandRunExecutePayload{
		CommandRunID: "run-2",
		Command:      "fakecmd status",
	})
	h.Handle(execPayload)
	cancelPayload, _ := json.Marshal(protocol.CommandRunCancelPayload{
		CommandRunID: "run-2",
		RuntimeID:    "runtime-1",
	})
	h.HandleCancel(cancelPayload)

	result := recvResult(t, send)
	if result.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %q", result.Status)
	}
}

func TestWebSocketHandlerConsumeCanceledRemovesRunID(t *testing.T) {
	send := make(chan []byte, 1)
	h := NewWebSocketHandler(t.TempDir(), send, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.cancelRun("run-3")
	if !h.consumeCanceled("run-3") {
		t.Fatal("expected canceled run ID to be consumed")
	}
	if h.consumeCanceled("run-3") {
		t.Fatal("expected canceled run ID to be removed after consumption")
	}
}

func TestWebSocketHandlerPruneCanceledDropsExpiredRunIDs(t *testing.T) {
	send := make(chan []byte, 1)
	h := NewWebSocketHandler(t.TempDir(), send, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Now()

	h.mu.Lock()
	h.canceled["expired-run"] = now.Add(-(canceledRunRetention + time.Second))
	h.canceled["fresh-run"] = now
	h.pruneCanceledLocked(now)
	_, hasExpired := h.canceled["expired-run"]
	_, hasFresh := h.canceled["fresh-run"]
	h.mu.Unlock()

	if hasExpired {
		t.Fatal("expected expired canceled run ID to be pruned")
	}
	if !hasFresh {
		t.Fatal("expected fresh canceled run ID to remain")
	}
}

func recvResult(t *testing.T, ch <-chan []byte) CommandRunResultPayload {
	t.Helper()
	select {
	case frame := <-ch:
		var msg protocol.Message
		if err := json.Unmarshal(frame, &msg); err != nil {
			t.Fatalf("unmarshal frame: %v", err)
		}
		var result CommandRunResultPayload
		if err := json.Unmarshal(msg.Payload, &result); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for command result")
		return CommandRunResultPayload{}
	}
}
