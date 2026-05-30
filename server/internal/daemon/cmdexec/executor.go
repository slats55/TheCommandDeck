// Package cmdexec provides safe, bounded command execution for CommandDeck.
// Only pre-approved commands (currently: git status, git branch, git rev-parse,
// git diff) are executed, using argv-style execution to prevent shell injection.
// Working directory is validated to ensure it stays within the runtime's workspace
// boundary.
package cmdexec

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// MaxDuration is the maximum time a command is allowed to run.
	MaxDuration = 30 * time.Second
	// MaxStdoutBytes is the maximum stdout bytes kept per command run.
	MaxStdoutBytes = 64 * 1024
	// MaxStderrBytes is the maximum stderr bytes kept per command run.
	MaxStderrBytes = 64 * 1024
	// OutputTruncatedMarker is appended when captured output exceeds safety limits.
	OutputTruncatedMarker = "\n[output truncated by CommandDeck safety limit]"
)

// Executor runs approved commands with bounded working directories.
type Executor struct {
	// allowedCommands contains exact argv forms. Arguments are part of the key so
	// approved subcommands cannot accept unapproved flags.
	allowedCommands map[string]bool

	// workspacesRoot is the base directory for all workspace worktrees.
	// Used to validate working_directory boundaries.
	workspacesRoot string
	maxDuration    time.Duration
	maxStdoutBytes int
	maxStderrBytes int
	runFn          func(ctx context.Context, cmd *exec.Cmd, stdoutLimit, stderrLimit int) (stdout, stderr string, exitCode int, stdoutTruncated, stderrTruncated bool, runErr error)
}

// NewExecutor creates a new command executor.
// workspacesRoot is the daemon's workspaces root (e.g. ~/multica_workspaces).
func NewExecutor(workspacesRoot string) *Executor {
	// Slice 1: only the four seeded built-in templates are allowed.
	allowed := map[string]bool{
		commandKey([]string{"git", "status"}):                   true,
		commandKey([]string{"git", "branch", "--show-current"}): true,
		commandKey([]string{"git", "rev-parse", "HEAD"}):        true,
		commandKey([]string{"git", "diff", "--stat"}):           true,
	}
	return &Executor{
		allowedCommands: allowed,
		workspacesRoot:  workspacesRoot,
		maxDuration:     MaxDuration,
		maxStdoutBytes:  MaxStdoutBytes,
		maxStderrBytes:  MaxStderrBytes,
		runFn:           runCommand,
	}
}

// Result holds the outcome of a command execution.
type Result struct {
	Status          string // "completed", "failed", "timeout"
	ExitCode        int
	Stdout          string
	Stderr          string
	StdoutTruncated bool
	StderrTruncated bool
	DurationMs      int
	WorkingDir      string
}

// Execute runs the given command in the specified working directory,
// after validating that workingDir is within the workspace boundary
// and that the command is in the allowlist.
//
// Uses exec.LookPath to find the binary and argv-style execution
// (no shell, no string splitting) to prevent injection.
func (e *Executor) Execute(ctx context.Context, command string, workingDir string) Result {
	start := time.Now()

	// Step 1: validate working directory boundary.
	if !e.isWithinBoundary(workingDir) {
		return Result{
			Status:     "failed",
			ExitCode:   1,
			Stderr:     "working directory is outside allowed workspace boundary",
			DurationMs: int(time.Since(start).Milliseconds()),
			WorkingDir: workingDir,
		}
	}

	// Step 2: parse the command into argv. Only approved commands are accepted.
	argv, err := parseCommand(command)
	if err != nil {
		return Result{
			Status:     "failed",
			ExitCode:   1,
			Stderr:     "invalid command: " + err.Error(),
			DurationMs: int(time.Since(start).Milliseconds()),
			WorkingDir: workingDir,
		}
	}

	// Step 3: validate against allowlist.
	if !e.isAllowed(argv) {
		return Result{
			Status:     "failed",
			ExitCode:   1,
			Stderr:     "command not in allowlist",
			DurationMs: int(time.Since(start).Milliseconds()),
			WorkingDir: workingDir,
		}
	}

	// Step 4: validate working directory exists and is accessible.
	if workingDir != "" {
		info, err := os.Stat(workingDir)
		if err != nil || !info.IsDir() {
			return Result{
				Status:     "failed",
				ExitCode:   1,
				Stderr:     "working directory does not exist or is not accessible",
				DurationMs: int(time.Since(start).Milliseconds()),
				WorkingDir: workingDir,
			}
		}
	}

	// Step 5: resolve the binary path.
	binary, err := exec.LookPath(argv[0])
	if err != nil {
		return Result{
			Status:     "failed",
			ExitCode:   1,
			Stderr:     "command not found: " + argv[0],
			DurationMs: int(time.Since(start).Milliseconds()),
			WorkingDir: workingDir,
		}
	}

	// Step 6: execute with timeout using argv (no shell).
	ctx, cancel := context.WithTimeout(ctx, e.maxDuration)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, argv[1:]...)
	cmd.Dir = workingDir
	cmd.Env = os.Environ() // inherit daemon environment, no extra secrets

	stdout, stderr, exitCode, stdoutTruncated, stderrTruncated, runErr := e.runFn(ctx, cmd, e.maxStdoutBytes, e.maxStderrBytes)

	status := "completed"
	if ctx.Err() == context.DeadlineExceeded {
		status = "timeout"
		if strings.TrimSpace(stderr) == "" {
			stderr = "command timed out before completion"
		}
	} else if errors.Is(runErr, context.DeadlineExceeded) {
		status = "timeout"
		if strings.TrimSpace(stderr) == "" {
			stderr = "command timed out before completion"
		}
	} else if exitCode != 0 {
		status = "failed"
	}
	if stdoutTruncated && !strings.Contains(stdout, OutputTruncatedMarker) {
		stdout += OutputTruncatedMarker
	}
	if stderrTruncated && !strings.Contains(stderr, OutputTruncatedMarker) {
		stderr += OutputTruncatedMarker
	}

	return Result{
		Status:          status,
		ExitCode:        exitCode,
		Stdout:          stdout,
		Stderr:          stderr,
		StdoutTruncated: stdoutTruncated,
		StderrTruncated: stderrTruncated,
		DurationMs:      int(time.Since(start).Milliseconds()),
		WorkingDir:      workingDir,
	}
}

// isAllowed checks if the argv exactly matches an entry in the allowlist.
// Example: ["git", "status"] is allowed; ["git", "diff", "--name-only"] is not.
func (e *Executor) isAllowed(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	return e.allowedCommands[commandKey(argv)]
}

func commandKey(argv []string) string {
	return strings.Join(argv, "\x00")
}

// isWithinBoundary returns true if workingDir is within the workspacesRoot
// or is empty (daemon will use its own default).
func (e *Executor) isWithinBoundary(workingDir string) bool {
	if workingDir == "" {
		return true // empty means "use default" — allow it
	}
	if e.workspacesRoot == "" {
		return true // no boundary configured
	}
	absWorkDir, err := filepath.Abs(workingDir)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(e.workspacesRoot)
	if err != nil {
		return false
	}
	// Ensure the working directory is under workspacesRoot.
	return strings.HasPrefix(absWorkDir, absRoot+string(filepath.Separator)) ||
		absWorkDir == absRoot
}

// parseCommand parses a command string into argv for argv-style execution.
// Only handles simple "binary subcommand" forms. Returns an error for
// anything that looks like shell features (pipes, redirects, variable
// expansion, etc.).
//
// Currently allows: git status, git branch, git rev-parse, git diff
func parseCommand(command string) ([]string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, &parseError{command, "empty command"}
	}

	// Reject obvious shell features.
	lower := strings.ToLower(command)
	shellChars := []string{"|", "&", ">", "<", "$", "`", "(", ")", "{", "}", ";", "<<", ">>"}
	for _, c := range shellChars {
		if strings.Contains(lower, c) {
			return nil, &parseError{command, "command contains disallowed characters"}
		}
	}

	// Simple space split — sufficient for "git status" and similar.
	// Split into at most 3 parts: binary, subcommand, and one optional arg.
	// Args are not supported in Slice 1 beyond one flag, so we validate at most 3 tokens.
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, &parseError{command, "empty after trim"}
	}
	if len(parts) > 3 {
		// "binary subcommand arg1 arg2 ..." — reject args beyond the one allowed flag.
		return nil, &parseError{command, "too many tokens: max 3 (binary subcommand [arg])"}
	}

	return parts, nil
}

type parseError struct {
	command string
	msg     string
}

func (e *parseError) Error() string {
	return e.msg + ": " + e.command
}

type cappedBuffer struct {
	builder   strings.Builder
	limit     int
	written   int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		b.truncated = true
		b.written += len(p)
		return len(p), nil
	}
	remaining := b.limit - b.builder.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = b.builder.Write(p)
		} else {
			_, _ = b.builder.Write(p[:remaining])
			b.truncated = true
		}
	} else {
		b.truncated = true
	}
	b.written += len(p)
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	return b.builder.String()
}

// runCommand executes cmd and captures bounded stdout/stderr. Always waits for completion.
func runCommand(ctx context.Context, cmd *exec.Cmd, stdoutLimit, stderrLimit int) (stdout, stderr string, exitCode int, stdoutTruncated, stderrTruncated bool, runErr error) {
	outBuf := &cappedBuffer{limit: stdoutLimit}
	errBuf := &cappedBuffer{limit: stderrLimit}
	cmd.Stdout = io.Writer(outBuf)
	cmd.Stderr = io.Writer(errBuf)

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	return outBuf.String(), errBuf.String(), exitCode, outBuf.truncated, errBuf.truncated, err
}
