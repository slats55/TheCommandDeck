package cmdexec

import (
	"context"
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
		{argv: []string{"git", "branch"}, expected: true},
		{argv: []string{"git", "rev-parse"}, expected: true},
		{argv: []string{"git", "diff"}, expected: true},

		// ── Rejection: non-approved git subcommands ───────────────────────────────
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
		{argv: []string{"git"}, expected: false},                  // missing subcommand
		{argv: []string{"bash", "-c", "ls"}, expected: false},    // bash not in allowlist
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
			command:      "git status | cat",
			workingDir:   baseDir,
			wantStatus:   "failed",
			wantStderrSub: "disallowed characters",
		},
		{
			name:          "too many tokens rejected",
			command:      "git status --short --verbose",
			workingDir:   baseDir,
			wantStatus:   "failed",
			wantStderrSub: "too many tokens",
		},
		{
			name:          "non-git binary rejected at allowlist",
			command:      "ls -la",
			workingDir:   baseDir,
			wantStatus:   "failed",
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