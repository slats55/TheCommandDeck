package cmdexec

import (
	"context"
	"os"
	execCmd "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		wantTokens []string
		wantErr    bool
		errMsg     string // substring that must appear in error
	}{
		// ── Approved commands (2 tokens: binary subcommand) ──────────────────────
		{
			name:       "git status is approved",
			cmd:        "git status",
			wantTokens: []string{"git", "status"},
			wantErr:    false,
		},

		// ── Approved commands (3 tokens: binary subcommand arg) ───────────────────
		{
			name:       "git branch --show-current is approved",
			cmd:        "git branch --show-current",
			wantTokens: []string{"git", "branch", "--show-current"},
			wantErr:    false,
		},
		{
			name:       "git rev-parse HEAD is approved",
			cmd:        "git rev-parse HEAD",
			wantTokens: []string{"git", "rev-parse", "HEAD"},
			wantErr:    false,
		},
		{
			name:       "git diff --stat is approved",
			cmd:        "git diff --stat",
			wantTokens: []string{"git", "diff", "--stat"},
			wantErr:    false,
		},

		// ── Rejection: empty / whitespace-only ───────────────────────────────────
		{
			name:    "empty string is rejected",
			cmd:     "",
			wantErr: true,
			errMsg:  "empty",
		},
		{
			name:    "whitespace-only is rejected",
			cmd:     "   \t  ",
			wantErr: true,
			errMsg:  "empty",
		},

		// ── Rejection: too many tokens (>3) ──────────────────────────────────────
		{
			name:    "4 tokens rejected",
			cmd:     "git status --short --verbose",
			wantErr: true,
			errMsg:  "too many tokens",
		},
		{
			name:    "5 tokens rejected",
			cmd:     "git branch --show-current -v",
			wantErr: true,
			errMsg:  "too many tokens",
		},

		// ── Rejection: shell metacharacters ─────────────────────────────────────
		{
			name:    "pipe rejected",
			cmd:     "git status | cat",
			wantErr: true,
			errMsg:  "disallowed characters",
		},
		{
			name:    "background & rejected",
			cmd:     "git status &",
			wantErr: true,
			errMsg:  "disallowed characters",
		},
		{
			name:    "redirect stdout rejected",
			cmd:     "git status > /tmp/out",
			wantErr: true,
			errMsg:  "disallowed characters",
		},
		{
			name:    "redirect stdin rejected",
			cmd:     "cat < /tmp/in",
			wantErr: true,
			errMsg:  "disallowed characters",
		},
		{
			name:    "variable expansion rejected",
			cmd:     "echo $HOME",
			wantErr: true,
			errMsg:  "disallowed characters",
		},
		{
			name:    "backtick substitution rejected",
			cmd:     "echo `ls`",
			wantErr: true,
			errMsg:  "disallowed characters",
		},
		{
			name:    "subshell parentheses rejected",
			cmd:     "(git status)",
			wantErr: true,
			errMsg:  "disallowed characters",
		},
		{
			name:    "brace expansion rejected",
			cmd:     "echo {1..3}",
			wantErr: true,
			errMsg:  "disallowed characters",
		},
		{
			name:    "semicolon command chaining rejected",
			cmd:     "git status; ls",
			wantErr: true,
			errMsg:  "disallowed characters",
		},
		{
			name:    "here-doc rejected",
			cmd:     "cat <<EOF",
			wantErr: true,
			errMsg:  "disallowed characters",
		},

		// ── Note: single-token "git" parses successfully (passes len check ≤ 3)
		// but is rejected later by isAllowed because argv[1] (subcommand) is missing.
		{
			name:       "single token parsed but rejected by isAllowed",
			cmd:        "git",
			wantTokens: []string{"git"},
			wantErr:    false,
		},
		// ── bash -c ls (3 tokens: binary subcommand arg) parses OK but isRejected
		// by isAllowed because bash is not in the allowlist.
		{
			name:       "bash -c ls parsed but rejected by isAllowed",
			cmd:        "bash -c ls",
			wantTokens: []string{"bash", "-c", "ls"},
			wantErr:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokens, err := parseCommand(tc.cmd)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseCommand(%q): expected error, got nil", tc.cmd)
				}
				if tc.errMsg != "" && !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("parseCommand(%q): error %q does not contain %q", tc.cmd, err.Error(), tc.errMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("parseCommand(%q): unexpected error: %v", tc.cmd, err)
				}
				if !equalSlice(tokens, tc.wantTokens) {
					t.Fatalf("parseCommand(%q): got %v, want %v", tc.cmd, tokens, tc.wantTokens)
				}
			}
		})
	}
}

func TestExecutorIsAllowed(t *testing.T) {
	exec := NewExecutor("/base/workspaces")

	tests := []struct {
		name     string
		argv     []string
		expected bool
	}{
		// ── Approved commands ────────────────────────────────────────────────────
		{argv: []string{"git", "status"}, expected: true},
		{argv: []string{"git", "branch", "--show-current"}, expected: true},
		{argv: []string{"git", "rev-parse", "HEAD"}, expected: true},
		{argv: []string{"git", "diff", "--stat"}, expected: true},

		// ── Rejection: non-approved git subcommands ───────────────────────────────
		{argv: []string{"git", "branch"}, expected: false},
		{argv: []string{"git", "branch", "-a"}, expected: false},
		{argv: []string{"git", "rev-parse"}, expected: false},
		{argv: []string{"git", "rev-parse", "--show-toplevel"}, expected: false},
		{argv: []string{"git", "diff"}, expected: false},
		{argv: []string{"git", "diff", "--name-only"}, expected: false},
		{argv: []string{"git", "push"}, expected: false},
		{argv: []string{"git", "commit"}, expected: false},
		{argv: []string{"git", "stash"}, expected: false},
		{argv: []string{"git", "fetch"}, expected: false},
		{argv: []string{"git", "pull"}, expected: false},
		{argv: []string{"git", "clone"}, expected: false},
		{argv: []string{"git", "reset"}, expected: false},

		// ── Rejection: non-git binaries ──────────────────────────────────────────
		{argv: []string{"ls"}, expected: false},
		{argv: []string{"bash"}, expected: false},
		{argv: []string{"python"}, expected: false},
		{argv: []string{"curl"}, expected: false},
		{argv: []string{"wget"}, expected: false},

		// ── Rejection: edge cases ─────────────────────────────────────────────────
		{argv: []string{}, expected: false},
		{argv: []string{"git"}, expected: false},              // missing subcommand
		{argv: []string{"bash", "-c", "ls"}, expected: false}, // bash not in allowlist
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := exec.isAllowed(tc.argv)
			if got != tc.expected {
				t.Fatalf("isAllowed(%v): got %v, want %v", tc.argv, got, tc.expected)
			}
		})
	}
}

func TestExecutorIsWithinBoundary(t *testing.T) {
	tests := []struct {
		name           string
		workspacesRoot string
		workingDir     string
		expected       bool
	}{
		{
			name:           "empty workingDir is allowed",
			workspacesRoot: "/base/workspaces",
			workingDir:     "",
			expected:       true,
		},
		{
			name:           "no boundary configured allows everything",
			workspacesRoot: "",
			workingDir:     "/any/path",
			expected:       true,
		},
		{
			name:           "subdirectory is within boundary",
			workspacesRoot: "/base/workspaces",
			workingDir:     "/base/workspaces/ws1/repo",
			expected:       true,
		},
		{
			name:           "exact root match is within boundary",
			workspacesRoot: "/base/workspaces",
			workingDir:     "/base/workspaces",
			expected:       true,
		},
		{
			name:           "sibling directory is outside boundary",
			workspacesRoot: "/base/workspaces",
			workingDir:     "/base/other",
			expected:       false,
		},
		{
			name:           "parent directory is outside boundary",
			workspacesRoot: "/base/workspaces",
			workingDir:     "/base",
			expected:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exec := NewExecutor(tc.workspacesRoot)
			got := exec.isWithinBoundary(tc.workingDir)
			if got != tc.expected {
				t.Fatalf("isWithinBoundary(%q): got %v, want %v", tc.workingDir, got, tc.expected)
			}
		})
	}
}

func TestExecuteApprovedBuiltins(t *testing.T) {
	requireGit(t)
	baseDir := t.TempDir()
	repoDir := filepath.Join(baseDir, "repo")
	if err := os.Mkdir(repoDir, 0755); err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	initGitRepo(t, repoDir)
	if err := os.WriteFile(filepath.Join(repoDir, "changed.txt"), []byte("changed\n"), 0644); err != nil {
		t.Fatalf("failed to create changed file: %v", err)
	}

	exec := NewExecutor(baseDir)
	tests := []struct {
		name          string
		command       string
		wantStdoutSub string
	}{
		{name: "git status", command: "git status", wantStdoutSub: "changed.txt"},
		{name: "git branch", command: "git branch --show-current", wantStdoutSub: "main"},
		{name: "git rev-parse", command: "git rev-parse HEAD"},
		{name: "git diff stat", command: "git diff --stat"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			result := exec.Execute(ctx, tc.command, repoDir)
			if result.Status != "completed" {
				t.Fatalf("Execute(%q): status=%q stderr=%q", tc.command, result.Status, result.Stderr)
			}
			if result.ExitCode != 0 {
				t.Fatalf("Execute(%q): exit code=%d, want 0", tc.command, result.ExitCode)
			}
			if result.WorkingDir != repoDir {
				t.Fatalf("Execute(%q): working dir=%q, want %q", tc.command, result.WorkingDir, repoDir)
			}
			if tc.wantStdoutSub != "" && !strings.Contains(result.Stdout, tc.wantStdoutSub) {
				t.Fatalf("Execute(%q): stdout=%q, want substring %q", tc.command, result.Stdout, tc.wantStdoutSub)
			}
		})
	}
}

func TestExecuteRejectedCases(t *testing.T) {
	baseDir := t.TempDir()
	exec := NewExecutor(baseDir)

	tests := []struct {
		name          string
		command       string
		workingDir    string
		wantStatus    string
		wantStderrSub string
	}{
		{
			name:          "shell metacharacters rejected at parse stage",
			command:       "git status | cat",
			workingDir:    baseDir,
			wantStatus:    "failed",
			wantStderrSub: "disallowed characters",
		},
		{
			name:          "too many tokens rejected",
			command:       "git status --short --verbose",
			workingDir:    baseDir,
			wantStatus:    "failed",
			wantStderrSub: "too many tokens",
		},
		{
			name:          "non-git binary rejected at allowlist",
			command:       "ls -la",
			workingDir:    baseDir,
			wantStatus:    "failed",
			wantStderrSub: "command not in allowlist",
		},
		{
			name:          "unapproved git diff arg rejected",
			command:       "git diff --name-only",
			workingDir:    baseDir,
			wantStatus:    "failed",
			wantStderrSub: "command not in allowlist",
		},
		{
			name:          "unapproved git branch arg rejected",
			command:       "git branch -a",
			workingDir:    baseDir,
			wantStatus:    "failed",
			wantStderrSub: "command not in allowlist",
		},
		{
			name:          "unapproved git rev-parse arg rejected",
			command:       "git rev-parse --show-toplevel",
			workingDir:    baseDir,
			wantStatus:    "failed",
			wantStderrSub: "command not in allowlist",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			result := exec.Execute(ctx, tc.command, tc.workingDir)
			if result.Status != tc.wantStatus {
				t.Fatalf("Execute(%q): status=%q, want %q", tc.command, result.Status, tc.wantStatus)
			}
			if !strings.Contains(result.Stderr, tc.wantStderrSub) {
				t.Fatalf("Execute(%q): stderr=%q, want substring %q", tc.command, result.Stderr, tc.wantStderrSub)
			}
		})
	}
}

func TestExecuteWorkingDirBoundary(t *testing.T) {
	baseDir := t.TempDir()
	exec := NewExecutor(baseDir)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	result := exec.Execute(ctx, "git status", "/outside/the/boundary")
	if result.Status != "failed" {
		t.Fatalf("Execute with out-of-boundary dir: status=%q, want failed", result.Status)
	}
	if !strings.Contains(result.Stderr, "outside allowed workspace boundary") {
		t.Fatalf("Execute with out-of-boundary dir: stderr=%q, want 'outside allowed workspace boundary'", result.Stderr)
	}
}

func TestExecuteNonexistentWorkingDir(t *testing.T) {
	baseDir := t.TempDir()
	exec := NewExecutor(baseDir)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	result := exec.Execute(ctx, "git status", filepath.Join(baseDir, "nonexistent", "path"))
	if result.Status != "failed" {
		t.Fatalf("Execute with nonexistent dir: status=%q, want failed", result.Status)
	}
	if !strings.Contains(result.Stderr, "working directory does not exist") {
		t.Fatalf("Execute with nonexistent dir: stderr=%q, want 'working directory does not exist'", result.Stderr)
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────────

func TestExecuteExitCodeAndStderrRecorded(t *testing.T) {
	requireGit(t)
	baseDir := t.TempDir()
	exec := NewExecutor(baseDir)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	result := exec.Execute(ctx, "git status", baseDir)
	if result.Status != "failed" {
		t.Fatalf("Execute in non-git dir: status=%q, want failed", result.Status)
	}
	if result.ExitCode == 0 {
		t.Fatalf("Execute in non-git dir: exit code=%d, want non-zero", result.ExitCode)
	}
	if result.Stderr == "" {
		t.Fatal("Execute in non-git dir: stderr is empty, want real git failure output")
	}
}

func TestExecuteMarksTimeoutWhenRunnerDeadlineExceeded(t *testing.T) {
	exec := NewExecutor(t.TempDir())
	command := installFakeAllowedCommand(t, exec)
	exec.maxDuration = 15 * time.Millisecond
	exec.runFn = func(ctx context.Context, _ *execCmd.Cmd, _, _ int) (string, string, int, bool, bool, error) {
		<-ctx.Done()
		return "", "", 1, false, false, ctx.Err()
	}

	result := exec.Execute(t.Context(), command, "")
	if result.Status != "timeout" {
		t.Fatalf("expected timeout status, got %q", result.Status)
	}
	if !strings.Contains(result.Stderr, "timed out") {
		t.Fatalf("expected timeout stderr marker, got %q", result.Stderr)
	}
}

func TestRunCommandBoundsOutputAndMarksTruncation(t *testing.T) {
	cmd := execCmd.Command(os.Args[0], "-test.run=TestHelperLargeOutputProcess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"LARGE_STDOUT="+strings.Repeat("o", 256),
		"LARGE_STDERR="+strings.Repeat("e", 256),
	)

	stdout, stderr, code, stdoutTruncated, stderrTruncated, err := runCommand(t.Context(), cmd, 32, 32)
	if err != nil || code != 0 {
		t.Fatalf("runCommand helper process failed: code=%d err=%v stderr=%q", code, err, stderr)
	}
	if len(stdout) != 32 {
		t.Fatalf("expected bounded stdout length 32, got %d", len(stdout))
	}
	if len(stderr) != 32 {
		t.Fatalf("expected bounded stderr length 32, got %d", len(stderr))
	}
	if !stdoutTruncated || !stderrTruncated {
		t.Fatalf("expected both stdout/stderr to be truncated, got stdout=%v stderr=%v", stdoutTruncated, stderrTruncated)
	}
}

func TestExecuteAppendsTruncationMarker(t *testing.T) {
	exec := NewExecutor(t.TempDir())
	command := installFakeAllowedCommand(t, exec)
	exec.runFn = func(_ context.Context, _ *execCmd.Cmd, _, _ int) (string, string, int, bool, bool, error) {
		return "ok", "warn", 0, true, true, nil
	}
	result := exec.Execute(t.Context(), command, "")
	if !strings.Contains(result.Stdout, OutputTruncatedMarker) {
		t.Fatalf("expected stdout truncation marker, got %q", result.Stdout)
	}
	if !strings.Contains(result.Stderr, OutputTruncatedMarker) {
		t.Fatalf("expected stderr truncation marker, got %q", result.Stderr)
	}
}

func TestHelperLargeOutputProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	_, _ = os.Stdout.WriteString(os.Getenv("LARGE_STDOUT"))
	_, _ = os.Stderr.WriteString(os.Getenv("LARGE_STDERR"))
	os.Exit(0)
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	runGit := func(args ...string) {
		t.Helper()
		cmd := execCmd.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = os.Environ()
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
		}
	}

	runGit("init", "-b", "main")
	runGit("config", "user.email", "commanddeck-test@example.invalid")
	runGit("config", "user.name", "CommandDeck Test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	runGit("add", "README.md")
	runGit("commit", "-m", "initial")
}

func installFakeAllowedCommand(t *testing.T, exec *Executor) string {
	t.Helper()
	binDir := t.TempDir()
	fakePath := filepath.Join(binDir, "fakecmd")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(fakePath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write fake command: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	command := "fakecmd status"
	exec.allowedCommands[commandKey([]string{"fakecmd", "status"})] = true
	return command
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := execCmd.LookPath("git"); err != nil {
		t.Skip("git not installed in test environment")
	}
}
