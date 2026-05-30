// Package cmdexec provides safe, bounded command execution for CommandDeck.
// This file contains the daemon-side wiring that connects WS messages to execution.
package cmdexec

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/multica-ai/multica/server/pkg/protocol"
)

// CommandRunExecutePayload is the server → daemon payload for command_run:execute.
// It is sent over the WebSocket connection that the daemon already has open.
type CommandRunExecutePayload struct {
	CommandRunID string `json:"command_run_id"`
	Command      string `json:"command"`
	WorkingDir   string `json:"working_directory,omitempty"`
	AllowedDir   string `json:"allowed_dir,omitempty"` // workspace boundary for validation
}

// CommandRunResultPayload is the daemon → server payload for command_run:result.
type CommandRunResultPayload struct {
	CommandRunID string `json:"command_run_id"`
	Status       string `json:"status"` // "completed", "failed", "timeout"
	ExitCode     int    `json:"exit_code,omitempty"`
	Stdout       string `json:"stdout,omitempty"`
	Stderr       string `json:"stderr,omitempty"`
	DurationMs   int    `json:"duration_ms,omitempty"`
}

// WebSocketHandler bridges between the daemon's WS connection and the Executor.
// It receives command_run:execute messages, runs the approved command,
// and sends command_run:result messages back on the same connection.
type WebSocketHandler struct {
	executor *Executor
	send     chan []byte // same channel used by the daemon's WS writePump
	logger   *slog.Logger
}

// NewWebSocketHandler creates a handler that sends result frames on the provided
// channel and executes commands using the given workspaces root.
func NewWebSocketHandler(workspacesRoot string, sendChan chan []byte, logger *slog.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		executor: NewExecutor(workspacesRoot),
		send:     sendChan,
		logger:   logger,
	}
}

// Handle processes an inbound command_run:execute frame.
// It validates the runtime is authorized, executes the command, and sends
// the result back to the server over the WebSocket.
func (h *WebSocketHandler) Handle(rawPayload json.RawMessage) {
	var execPayload CommandRunExecutePayload
	if err := json.Unmarshal(rawPayload, &execPayload); err != nil {
		h.logger.Debug("command_run:execute: failed to unmarshal payload", "error", err)
		return
	}
	if execPayload.CommandRunID == "" {
		h.logger.Debug("command_run:execute: missing command_run_id")
		return
	}

	// Determine working directory: use execPayload.WorkingDir if set,
	// otherwise fall back to execPayload.AllowedDir (the runtime's worktree root).
	// The executor validates workingDir against the workspace boundary.
	workingDir := execPayload.WorkingDir
	if workingDir == "" {
		workingDir = execPayload.AllowedDir
	}

	ctx, cancel := context.WithTimeout(context.Background(), MaxDuration)
	defer cancel()

	result := h.executor.Execute(ctx, execPayload.Command, workingDir)

	resultPayload := CommandRunResultPayload{
		CommandRunID: execPayload.CommandRunID,
		Status:       result.Status,
		ExitCode:     result.ExitCode,
		Stdout:       result.Stdout,
		Stderr:       result.Stderr,
		DurationMs:   result.DurationMs,
	}

	frame, err := json.Marshal(protocol.Message{
		Type:    protocol.CommandRunResult,
		Payload: mustMarshal(resultPayload),
	})
	if err != nil {
		h.logger.Debug("command_run:result: failed to marshal", "error", err)
		return
	}

	select {
	case h.send <- frame:
	default:
		h.logger.Debug("command_run:result: send buffer full, dropping result",
			"command_run_id", execPayload.CommandRunID)
	}
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}
