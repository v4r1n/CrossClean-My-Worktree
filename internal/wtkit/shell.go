package wtkit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Result is the outcome of running a command.
type Result struct {
	Status int
	Stdout string
	Stderr string
}

// OK reports a zero exit status.
func (r Result) OK() bool { return r.Status == 0 }

// Output is stdout without surrounding whitespace.
func (r Result) Output() string { return strings.TrimSpace(r.Stdout) }

// ErrorOutput is stderr without surrounding whitespace.
func (r Result) ErrorOutput() string { return strings.TrimSpace(r.Stderr) }

// Lines splits stdout into lines, dropping carriage returns.
func (r Result) Lines() []string {
	lines := strings.Split(strings.ReplaceAll(r.Stdout, "\r\n", "\n"), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// Run runs a command synchronously and captures its output. Status is -1 when
// the command couldn't be started.
func Run(name string, args []string, dir string, env []string) Result {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	hideWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	status := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			status = exitErr.ExitCode()
		} else {
			status = -1
			stderr.WriteString(err.Error())
		}
	}
	return Result{Status: status, Stdout: stdout.String(), Stderr: stderr.String()}
}

// gitCandidates are fallback locations when git isn't on PATH (e.g. a GUI
// launch with a minimal environment).
func gitCandidates() []string {
	if isWindows {
		var out []string
		for _, env := range []string{"ProgramFiles", "ProgramW6432", "LOCALAPPDATA"} {
			if base := os.Getenv(env); base != "" {
				out = append(out,
					filepath.Join(base, "Git", "cmd", "git.exe"),
					filepath.Join(base, "Programs", "Git", "cmd", "git.exe"))
			}
		}
		return out
	}
	return []string{"/opt/homebrew/bin/git", "/usr/local/bin/git", "/usr/bin/git"}
}

// GitExecutable is the git binary used for every git call.
var GitExecutable = sync.OnceValue(func() string {
	if path, err := exec.LookPath("git"); err == nil {
		return path
	}
	for _, candidate := range gitCandidates() {
		if IsRegularFile(candidate) {
			return candidate
		}
	}
	return "git"
})

// gitEnv: GIT_OPTIONAL_LOCKS=0 keeps `git status` from taking index.lock, so
// scanning never interferes with a session working in a worktree.
var gitEnv = []string{"GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}

// Git runs git with the given arguments.
func Git(args ...string) Result {
	return Run(GitExecutable(), args, "", gitEnv)
}

// GitAvailable reports whether git can be run.
func GitAvailable() bool {
	return Git("--version").OK()
}

// RepoArgs are the arguments that point git at a repository, given its common dir.
func RepoArgs(commonDir string) []string {
	if filepath.Base(commonDir) == ".git" {
		return []string{"-C", filepath.Dir(commonDir)}
	}
	return []string{"--git-dir", commonDir}
}
