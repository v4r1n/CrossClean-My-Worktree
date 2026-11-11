// Package testrepo builds throwaway git repositories and an isolated home
// folder for tests. It is only imported from _test.go files.
package testrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Git runs git in dir, failing the test on error, and returns its output.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// Write creates a file and its parent folders.
func Write(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// New creates a repository at dir with one commit on main.
func New(t testing.TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	Git(t, dir, "init", "-q", "-b", "main")
	Write(t, filepath.Join(dir, "README"), "hello\n")
	Git(t, dir, "add", ".")
	Git(t, dir, "commit", "-q", "-m", "init")
}

// RequireGit skips the test when git isn't installed.
func RequireGit(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// Isolate points the home and config folders at a fresh temp folder, so a test
// never scans or writes the real user's files. It returns the new home.
func Isolate(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	if real, err := filepath.EvalSymlinks(home); err == nil {
		home = real
	}
	config := filepath.Join(home, "config")
	for key, value := range map[string]string{
		"HOME": home, "USERPROFILE": home, "APPDATA": config, "XDG_CONFIG_HOME": config,
	} {
		t.Setenv(key, value)
	}
	return home
}

// Fixture is a repository with one worktree of each interesting kind.
type Fixture struct {
	Repo   string // main checkout
	Merged string // clean, merged: Safe
	Dirty  string // untracked file: Risky
	Gone   string // folder deleted: Safe (prunable)
}

// NewFixture builds the fixture under root (root/code/...).
func NewFixture(t testing.TB, root string) Fixture {
	t.Helper()
	code := filepath.Join(root, "code")
	f := Fixture{
		Repo:   filepath.Join(code, "app"),
		Merged: filepath.Join(code, "app", ".claude", "worktrees", "merged"),
		Dirty:  filepath.Join(code, "dirty work"),
		Gone:   filepath.Join(code, "gone"),
	}
	New(t, f.Repo)
	Git(t, f.Repo, "worktree", "add", "-q", "-b", "feat/merged", f.Merged)
	Git(t, f.Repo, "worktree", "add", "-q", "-b", "feat/dirty", f.Dirty)
	Write(t, filepath.Join(f.Dirty, "notes.txt"), "work in progress\n")
	Git(t, f.Repo, "worktree", "add", "-q", "-b", "feat/gone", f.Gone)
	if err := os.RemoveAll(f.Gone); err != nil {
		t.Fatal(err)
	}
	return f
}

// Worktrees lists the paths git still knows as linked worktrees of repo.
func Worktrees(t testing.TB, repo string) []string {
	t.Helper()
	var paths []string
	for i, line := range strings.Split(Git(t, repo, "worktree", "list", "--porcelain"), "\n") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "worktree "); ok && i > 0 {
			paths = append(paths, filepath.Clean(path))
		}
	}
	return paths
}
