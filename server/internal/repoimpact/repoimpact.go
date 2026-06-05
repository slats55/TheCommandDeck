// Package repoimpact provides a deterministic, read-only classifier for the
// files a branch changed. Given a list of changed files it assigns each to a
// coarse repo subsystem and raises risk flags (migrations, auth, command
// execution, runtime/daemon, docker/compose, generated code, dependencies,
// large diff, docs-only, missing tests).
//
// It is intentionally a foundation: it does not read the database, mutate
// anything, or run a knowledge graph. The JSON it emits is designed to feed a
// future verification gate runner. All classification logic here is pure
// (path + optional file content in, structured report out) so it is fully
// table-testable; the git plumbing lives in the cmd/repo-impact CLI.
package repoimpact

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Subsystem is a coarse functional area of the repository.
type Subsystem string

const (
	SubsystemGithubCI        Subsystem = "github_ci"
	SubsystemDocs            Subsystem = "docs"
	SubsystemInfraDocker     Subsystem = "infra_docker"
	SubsystemDatabase        Subsystem = "database_migrations"
	SubsystemSecurityAuth    Subsystem = "security_auth"
	SubsystemTests           Subsystem = "tests"
	SubsystemDaemonRuntime   Subsystem = "daemon_runtime"
	SubsystemCommandExec     Subsystem = "command_exec"
	SubsystemScriptsDevtools Subsystem = "scripts_devtools"
	SubsystemBackend         Subsystem = "backend"
	SubsystemFrontend        Subsystem = "frontend"
	SubsystemUnknown         Subsystem = "unknown"
)

// largeDiffThreshold is the changed-file count at/above which a diff is flagged
// "large" for reviewer attention. File-count based (we only consume name-only).
const largeDiffThreshold = 25

// ChangedFile is one file in a diff. Content is optional and only used for
// content-aware risk flags (compose usage, generated-code markers); it may be
// empty when the file is binary, deleted, or too large to scan.
type ChangedFile struct {
	Path    string
	Content string
}

// RiskFlag is a single raised concern with the files that triggered it.
type RiskFlag struct {
	Name     string   `json:"name"`
	Severity string   `json:"severity"`
	Evidence []string `json:"evidence"`
}

// Report is the full structured impact classification for a diff.
type Report struct {
	BaseRef      string              `json:"baseRef"`
	HeadRef      string              `json:"headRef"`
	ChangedFiles []string            `json:"changedFiles"`
	Subsystems   map[string][]string `json:"subsystems"`
	RiskFlags    []RiskFlag          `json:"riskFlags"`
	Summary      string              `json:"summary"`
}

var (
	// Match a token only at a path-component / separator boundary so "auth"
	// matches middleware/auth.go and auth-initializer.tsx but not author.go.
	reSecurityAuth  = regexp.MustCompile(`(?:^|[/_\-.])(auth|authn|authz|jwt|session|csrf|oauth|login|logout|credential|credentials|password|passwd|secret)(?:[/_\-.]|$)`)
	reDaemonRuntime = regexp.MustCompile(`(?:^|[/_\-.])(daemon|daemonws|runtime|liveness|heartbeat)(?:[/_\-.]|$)`)
	reCommandExec   = regexp.MustCompile(`command[_\-]?(runner|workflow|run|template|ledger|exec|dispatch)`)
	reGeneratedPath = regexp.MustCompile(`(/generated/|\.gen\.|\.sql\.go$|\.pb\.go$|_templ\.go$)`)
	// Canonical machine-generated header (Go's convention, also matched for
	// other comment styles). Anchored to a line start and only scanned in the
	// file head, so a source file that merely *mentions* the marker mid-body
	// (like this classifier) is not misflagged as generated.
	reGeneratedHeader = regexp.MustCompile(`(?m)^\s*(?://|#|/\*|\*)\s*Code generated .* DO NOT EDIT\.`)
	dependencyFiles   = map[string]bool{
		"package.json": true, "package-lock.json": true, "pnpm-lock.yaml": true,
		"pnpm-workspace.yaml": true, "yarn.lock": true, "go.mod": true, "go.sum": true,
		"cargo.toml": true, "cargo.lock": true, "requirements.txt": true, "poetry.lock": true,
	}
)

func normalize(p string) string {
	return strings.ToLower(strings.ReplaceAll(p, "\\", "/"))
}

// ClassifyPath assigns a file to exactly one primary subsystem. The order is
// most-specific-first so a file that could match several lands in the lens a
// reviewer cares about most (an auth file is "security_auth", not "backend").
func ClassifyPath(p string) Subsystem {
	q := normalize(p)
	base := path.Base(q)
	ext := path.Ext(q)

	switch {
	case strings.HasPrefix(q, ".github/"):
		return SubsystemGithubCI
	case isDocs(q, ext):
		return SubsystemDocs
	case isInfraDocker(q, base):
		return SubsystemInfraDocker
	case isMigration(q, ext):
		return SubsystemDatabase
	case isTest(q):
		// Test files land in "tests" even when they touch auth/runtime code;
		// the corresponding risk flag still fires independently below.
		return SubsystemTests
	case reSecurityAuth.MatchString(q):
		return SubsystemSecurityAuth
	case reDaemonRuntime.MatchString(q):
		return SubsystemDaemonRuntime
	case reCommandExec.MatchString(q):
		return SubsystemCommandExec
	case isScriptsDevtools(q, base):
		return SubsystemScriptsDevtools
	case strings.HasPrefix(q, "server/"):
		return SubsystemBackend
	case strings.HasPrefix(q, "apps/") || strings.HasPrefix(q, "packages/"):
		return SubsystemFrontend
	default:
		return SubsystemUnknown
	}
}

func isDocs(q, ext string) bool {
	switch ext {
	case ".md", ".mdx", ".txt", ".rst":
		return true
	}
	return strings.HasPrefix(q, "docs/") || strings.HasPrefix(q, "apps/docs/")
}

func isInfraDocker(q, base string) bool {
	if base == ".dockerignore" || strings.HasPrefix(base, "dockerfile") {
		return true
	}
	if strings.Contains(base, "compose") && (strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")) {
		return true
	}
	return false
}

func isMigration(q, ext string) bool {
	if ext == ".sql" {
		return true
	}
	return strings.Contains(q, "/migrations/") || strings.HasPrefix(q, "server/migrations/")
}

func isTest(q string) bool {
	return strings.HasSuffix(q, "_test.go") ||
		strings.Contains(q, ".test.") ||
		strings.Contains(q, ".spec.") ||
		strings.HasPrefix(q, "e2e/") ||
		strings.Contains(q, "/e2e/")
}

func isScriptsDevtools(q, base string) bool {
	if strings.HasPrefix(q, "scripts/") {
		return true
	}
	switch base {
	case "makefile", "turbo.json", "biome.json", ".npmrc":
		return true
	}
	return strings.HasSuffix(base, ".mk")
}

func contentMentionsCompose(content string) bool {
	return strings.Contains(content, "docker compose") || strings.Contains(content, "docker-compose")
}

func isGenerated(q, content string) bool {
	if reGeneratedPath.MatchString(q) {
		return true
	}
	head := content
	if len(head) > 512 {
		head = head[:512]
	}
	return reGeneratedHeader.MatchString(head)
}

// riskFlagOrder fixes the output order and severity of every flag, so the same
// diff always produces byte-identical output (deterministic for the gate).
var riskFlagOrder = []struct {
	name     string
	severity string
}{
	{"migrations_touched", "high"},
	{"auth_security_touched", "high"},
	{"command_execution_touched", "high"},
	{"runtime_daemon_touched", "medium"},
	{"docker_compose_touched", "medium"},
	{"generated_code_touched", "medium"},
	{"package_dependency_touched", "medium"},
	{"large_diff", "low"},
	{"docs_only", "low"},
	{"tests_missing_for_code_change", "low"},
}

// RiskFlags computes every triggered risk flag with its supporting evidence.
func RiskFlags(files []ChangedFile) []RiskFlag {
	ev := map[string][]string{}
	add := func(name, file string) { ev[name] = append(ev[name], file) }

	hasCode, hasTests := false, false
	allDocs := len(files) > 0

	for _, f := range files {
		q := normalize(f.Path)
		base := path.Base(q)
		sub := ClassifyPath(f.Path)

		if sub != SubsystemDocs {
			allDocs = false
		}
		switch sub {
		case SubsystemTests:
			hasTests = true
		case SubsystemBackend, SubsystemFrontend, SubsystemCommandExec,
			SubsystemDaemonRuntime, SubsystemSecurityAuth, SubsystemDatabase:
			hasCode = true
		}

		if sub == SubsystemDatabase {
			add("migrations_touched", f.Path)
		}
		if reSecurityAuth.MatchString(q) {
			add("auth_security_touched", f.Path)
		}
		if reCommandExec.MatchString(q) {
			add("command_execution_touched", f.Path)
		}
		if reDaemonRuntime.MatchString(q) {
			add("runtime_daemon_touched", f.Path)
		}
		if isInfraDocker(q, base) || contentMentionsCompose(f.Content) {
			add("docker_compose_touched", f.Path)
		}
		if isGenerated(q, f.Content) {
			add("generated_code_touched", f.Path)
		}
		if dependencyFiles[base] {
			add("package_dependency_touched", f.Path)
		}
	}

	if allDocs {
		for _, f := range files {
			add("docs_only", f.Path)
		}
	}
	if len(files) >= largeDiffThreshold {
		ev["large_diff"] = []string{fmt.Sprintf("%d files changed", len(files))}
	}
	if hasCode && !hasTests {
		ev["tests_missing_for_code_change"] = []string{"code changed but no test files in diff"}
	}

	var flags []RiskFlag
	for _, spec := range riskFlagOrder {
		if e, ok := ev[spec.name]; ok && len(e) > 0 {
			// Sort evidence so output is byte-stable regardless of the order
			// git emitted the changed files — the gate consumes this verbatim.
			sort.Strings(e)
			flags = append(flags, RiskFlag{Name: spec.name, Severity: spec.severity, Evidence: e})
		}
	}
	return flags
}

// BuildReport classifies a diff into the full structured report.
func BuildReport(base, head string, files []ChangedFile) Report {
	subs := map[Subsystem][]string{}
	changed := make([]string, 0, len(files))
	for _, f := range files {
		changed = append(changed, f.Path)
		s := ClassifyPath(f.Path)
		subs[s] = append(subs[s], f.Path)
	}
	sort.Strings(changed)

	subsOut := make(map[string][]string, len(subs))
	for k, v := range subs {
		sort.Strings(v)
		subsOut[string(k)] = v
	}

	flags := RiskFlags(files)
	return Report{
		BaseRef:      base,
		HeadRef:      head,
		ChangedFiles: changed,
		Subsystems:   subsOut,
		RiskFlags:    flags,
		Summary:      buildSummary(subsOut, flags, len(changed)),
	}
}

func buildSummary(subs map[string][]string, flags []RiskFlag, n int) string {
	names := make([]string, 0, len(subs))
	for k := range subs {
		names = append(names, k)
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprintf(&b, "%d changed file(s)", n)
	if len(names) > 0 {
		fmt.Fprintf(&b, " across %s", strings.Join(names, ", "))
	}
	if len(flags) == 0 {
		b.WriteString(". No risk flags.")
		return b.String()
	}
	parts := make([]string, 0, len(flags))
	for _, f := range flags {
		parts = append(parts, fmt.Sprintf("%s (%s)", f.Name, f.Severity))
	}
	fmt.Fprintf(&b, ". Risks: %s.", strings.Join(parts, ", "))
	return b.String()
}
