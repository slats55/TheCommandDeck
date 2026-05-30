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
