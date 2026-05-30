package protocol

// Command-run message types.
const (
	// CommandRunExecute is sent server -> daemon to request execution.
	CommandRunExecute = "command_run:execute"
	// CommandRunStarted is sent daemon -> server when execution starts.
	CommandRunStarted = "command_run:started"
	// CommandRunResult is sent daemon -> server when execution finishes.
	CommandRunResult = "command_run:result"
	// CommandRunCancel is sent server -> daemon to request cancellation.
	CommandRunCancel = "command_run:cancel"
)

// CommandRunExecutePayload is the payload for command_run:execute (server -> daemon).
type CommandRunExecutePayload struct {
	CommandRunID  string `json:"command_run_id"`
	RuntimeID     string `json:"runtime_id"`                  // target runtime, for routing confirmation
	Command       string `json:"command"`                     // e.g. "git status"
	WorkingDir    string `json:"working_directory,omitempty"` // absolute path on the daemon machine
	AllowedDir    string `json:"allowed_dir,omitempty"`       // workspace boundary for validation
	WorkspaceID   string `json:"workspace_id"`
	InitiatorType string `json:"initiator_type,omitempty"` // "member" or "agent"
	InitiatorID   string `json:"initiator_id,omitempty"`
	IssueID       string `json:"issue_id,omitempty"`
}

// CommandRunStartedPayload is the payload for command_run:started (daemon -> server).
type CommandRunStartedPayload struct {
	CommandRunID string `json:"command_run_id"`
	Status       string `json:"status"` // "running"
}

// CommandRunResultPayload is the payload for command_run:result (daemon -> server).
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

// CommandRunCancelPayload is the payload for command_run:cancel (server -> daemon).
type CommandRunCancelPayload struct {
	CommandRunID string `json:"command_run_id"`
	RuntimeID    string `json:"runtime_id"`
}
