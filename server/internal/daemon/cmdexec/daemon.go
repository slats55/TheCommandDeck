// Package cmdexec provides safe, bounded command execution for CommandDeck.
// This file contains the daemon-side wiring that connects WS messages to execution.
package cmdexec

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

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
	CommandRunID    string `json:"command_run_id"`
	Status          string `json:"status"` // "completed", "failed", "timeout", "cancelled"
	ExitCode        int    `json:"exit_code,omitempty"`
	Stdout          string `json:"stdout,omitempty"`
	Stderr          string `json:"stderr,omitempty"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	DurationMs      int    `json:"duration_ms,omitempty"`
}

// WebSocketHandler bridges between the daemon's WS connection and the Executor.
// It receives command_run:execute messages, runs the approved command,
// and sends command_run:result messages back on the same connection.
type WebSocketHandler struct {
	executor *Executor
	send     chan []byte // same channel used by the daemon's WS writePump
	logger   *slog.Logger
	mu       sync.Mutex
	active   map[string]context.CancelFunc
	canceled map[string]time.Time
}

const canceledRunRetention = 5 * time.Minute

// NewWebSocketHandler creates a handler that sends result frames on the provided
// channel and executes commands using the given workspaces root.
func NewWebSocketHandler(workspacesRoot string, sendChan chan []byte, logger *slog.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		executor: NewExecutor(workspacesRoot),
		send:     sendChan,
		logger:   logger,
		active:   make(map[string]context.CancelFunc),
		canceled: make(map[string]time.Time),
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

	if h.consumeCanceled(execPayload.CommandRunID) {
		h.sendResult(CommandRunResultPayload{
			CommandRunID: execPayload.CommandRunID,
			Status:       "cancelled",
			ExitCode:     1,
			Stderr:       "command cancelled before execution",
		})
		return
	}

	runCtx, cancel := context.WithCancel(context.Background())
	h.setActive(execPayload.CommandRunID, cancel)
	go h.executeRun(runCtx, execPayload, workingDir)
}

// HandleCancel processes an inbound command_run:cancel frame.
func (h *WebSocketHandler) HandleCancel(rawPayload json.RawMessage) {
	var cancelPayload protocol.CommandRunCancelPayload
	if err := json.Unmarshal(rawPayload, &cancelPayload); err != nil {
		h.logger.Debug("command_run:cancel: failed to unmarshal payload", "error", err)
		return
	}
	if cancelPayload.CommandRunID == "" {
		h.logger.Debug("command_run:cancel: missing command_run_id")
		return
	}
	h.cancelRun(cancelPayload.CommandRunID)
}

func (h *WebSocketHandler) executeRun(runCtx context.Context, execPayload CommandRunExecutePayload, workingDir string) {
	result := h.executor.Execute(runCtx, execPayload.Command, workingDir)
	if runCtx.Err() == context.Canceled && result.Status != "timeout" {
		result.Status = "cancelled"
		if result.ExitCode == 0 {
			result.ExitCode = 1
		}
		if result.Stderr == "" {
			result.Stderr = "command cancelled by request"
		}
	}
	h.clearActive(execPayload.CommandRunID)
	h.sendResult(CommandRunResultPayload{
		CommandRunID:    execPayload.CommandRunID,
		Status:          result.Status,
		ExitCode:        result.ExitCode,
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		StdoutTruncated: result.StdoutTruncated,
		StderrTruncated: result.StderrTruncated,
		DurationMs:      result.DurationMs,
	})
}

func (h *WebSocketHandler) sendResult(resultPayload CommandRunResultPayload) {
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
		h.logger.Debug("command_run:result: send buffer full, dropping result", "command_run_id", resultPayload.CommandRunID)
	}
}

func (h *WebSocketHandler) setActive(runID string, cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.active[runID] = cancel
}

func (h *WebSocketHandler) clearActive(runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.active, runID)
	delete(h.canceled, runID)
}

func (h *WebSocketHandler) cancelRun(runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneCanceledLocked(time.Now())
	if cancel, ok := h.active[runID]; ok {
		h.canceled[runID] = time.Now()
		cancel()
		return
	}
	h.canceled[runID] = time.Now()
}

func (h *WebSocketHandler) consumeCanceled(runID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneCanceledLocked(time.Now())
	if _, ok := h.canceled[runID]; !ok {
		return false
	}
	delete(h.canceled, runID)
	return true
}

func (h *WebSocketHandler) pruneCanceledLocked(now time.Time) {
	for runID, cancelledAt := range h.canceled {
		if now.Sub(cancelledAt) > canceledRunRetention {
			delete(h.canceled, runID)
		}
	}
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}
