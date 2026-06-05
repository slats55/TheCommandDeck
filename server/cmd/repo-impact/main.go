// Command repo-impact is a read-only developer tool that classifies what a
// branch changed: it runs `git diff --name-only <base>...<head>`, maps each
// changed file to a subsystem, raises risk flags, and prints a human summary
// or JSON. The JSON is designed to feed a future verification gate runner.
//
// It executes only git with fixed arguments (no shell, no arbitrary commands)
// and reads file contents from the working tree to power content-aware flags.
// It exits non-zero only on a real failure (e.g. a bad git ref) — the presence
// of risk flags is a finding, not a tool error.
//
// Usage:
//
//	go run ./cmd/repo-impact                      # origin/main...HEAD, human output
//	go run ./cmd/repo-impact --json               # machine-readable
//	go run ./cmd/repo-impact --base main --head HEAD
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/multica-ai/multica/server/internal/repoimpact"
)

// maxScanBytes caps how much of a file we read for content-aware flags. Large
// or binary files are skipped (Content left empty) — path rules still apply.
const maxScanBytes = 1 << 20 // 1 MiB

func main() {
	base := flag.String("base", "origin/main", "base ref to diff from")
	head := flag.String("head", "HEAD", "head ref to diff to")
	asJSON := flag.Bool("json", false, "emit JSON instead of a human summary")
	flag.Parse()

	root, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "repo-impact: not a git repository: %v\n", err)
		os.Exit(2)
	}

	paths, err := changedFiles(root, *base, *head)
	if err != nil {
		fmt.Fprintf(os.Stderr, "repo-impact: git diff failed for %s...%s: %v\n", *base, *head, err)
		os.Exit(2)
	}

	files := make([]repoimpact.ChangedFile, 0, len(paths))
	for _, p := range paths {
		files = append(files, repoimpact.ChangedFile{Path: p, Content: readScan(root, p)})
	}

	report := repoimpact.BuildReport(*base, *head, files)

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "repo-impact: encode failed: %v\n", err)
			os.Exit(2)
		}
		return
	}

	printHuman(report)
}

func repoRoot() (string, error) {
	out, err := runGit("", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func changedFiles(root, base, head string) ([]string, error) {
	// Three-dot diff: files changed on head since the merge-base with base.
	out, err := runGit(root, "diff", "--name-only", base+"..."+head)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			paths = append(paths, s)
		}
	}
	return paths, nil
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return stdout.String(), nil
}

// readScan returns up to maxScanBytes of a working-tree file's text. It returns
// "" for missing (deleted), oversized, or binary files — path-based flags still
// apply, only content-aware flags are skipped for those.
func readScan(root, rel string) string {
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil || info.IsDir() || info.Size() > maxScanBytes {
		return ""
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return ""
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "" // binary
	}
	return string(data)
}

func printHuman(r repoimpact.Report) {
	fmt.Printf("Repo impact: %s...%s\n", r.BaseRef, r.HeadRef)
	fmt.Printf("%s\n\n", r.Summary)

	if len(r.ChangedFiles) == 0 {
		fmt.Println("No changed files.")
		return
	}

	fmt.Println("Subsystems:")
	for _, name := range sortedKeys(r.Subsystems) {
		fmt.Printf("  %-20s %d file(s)\n", name, len(r.Subsystems[name]))
	}

	if len(r.RiskFlags) == 0 {
		fmt.Println("\nRisk flags: none")
		return
	}
	fmt.Println("\nRisk flags:")
	for _, f := range r.RiskFlags {
		fmt.Printf("  [%-6s] %s\n", f.Severity, f.Name)
		for _, e := range f.Evidence {
			fmt.Printf("            - %s\n", e)
		}
	}
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// small n; insertion sort keeps output deterministic without importing sort
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
