package protocol

// CommandRunExecute is the message type for server → daemon command execution requests.
const CommandRunExecute = "command_run:execute"

// CommandRunResult is the message type for daemon → server command execution results.
const CommandRunResult = "command_run:result"
const CommandRunCancel = "command_run:cancel"

// CommandRunExecutePayload is the payload for command_run:execute (server → daemon).
// It carries the command to run, working directory, and execution context.
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

// CommandRunResultPayload is the payload for command_run:result (daemon → server).
// It carries the execution outcome for recording in the DB.
type CommandRunResultPayload struct {
	CommandRunID string `json:"command_run_id"`
	Status       string `json:"status"` // "completed", "failed", "timeout", "cancelled"
	ExitCode     int    `json:"exit_code,omitempty"`
	Stdout       string `json:"stdout,omitempty"`
	Stderr       string `json:"stderr,omitempty"`
	DurationMs   int    `json:"duration_ms,omitempty"`
}

// CommandRunCancelPayload is the payload for command_run:cancel.
type CommandRunCancelPayload struct {
	CommandRunID string `json:"command_run_id"`
	RuntimeID    string `json:"runtime_id"`
}
