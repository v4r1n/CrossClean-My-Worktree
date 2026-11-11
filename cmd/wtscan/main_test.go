package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/testrepo"
)

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestHelpPrintsUsage(t *testing.T) {
	code, out, _ := runCLI(t, "--help")
	if code != 0 || !strings.Contains(out, "usage: wtscan") {
		t.Fatalf("code %d, out %q", code, out)
	}
}

func TestUnknownOptionExitsTwo(t *testing.T) {
	code, _, errOut := runCLI(t, "--bogus")
	if code != 2 || !strings.Contains(errOut, "unknown option --bogus") {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
}

func TestReportsFixtureWithFlagAfterFolder(t *testing.T) {
	testrepo.RequireGit(t)
	root := t.TempDir()
	testrepo.NewFixture(t, root)

	code, out, errOut := runCLI(t, root, "--no-size")

	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	assertFixtureReport(t, out, false)
	if !strings.Contains(errOut, "1 repos with worktrees") {
		t.Errorf("stderr: %q", errOut)
	}
}

func TestDefaultsToHomeAndMeasuresSizes(t *testing.T) {
	testrepo.RequireGit(t)
	home := testrepo.Isolate(t)
	testrepo.NewFixture(t, home)

	code, out, errOut := runCLI(t)

	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	assertFixtureReport(t, out, true)
	if !strings.Contains(out, "~") {
		t.Errorf("paths inside home should be abbreviated:\n%s", out)
	}
}

// TestBinaryEndToEnd builds the real wtscan binary and runs it as a user would.
func TestBinaryEndToEnd(t *testing.T) {
	testrepo.RequireGit(t)
	if testing.Short() {
		t.Skip("builds a binary")
	}
	bin := filepath.Join(t.TempDir(), "wtscan")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	root := t.TempDir()
	testrepo.NewFixture(t, root)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, root)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("wtscan: %v\n%s", err, stderr.String())
	}
	assertFixtureReport(t, stdout.String(), true)

	bad := exec.Command(bin, "--nope")
	if err := bad.Run(); bad.ProcessState.ExitCode() != 2 {
		t.Fatalf("unknown option: exit %d (%v)", bad.ProcessState.ExitCode(), err)
	}
}

func assertFixtureReport(t *testing.T, out string, sized bool) {
	t.Helper()
	for _, want := range []string{"feat/merged", "feat/dirty", "feat/gone", "missing dir", "1 uncommitted",
		"3 worktrees", "2 safe to delete"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	lines := strings.Split(out, "\n")
	verdicts := map[string]string{"feat/merged": "SAFE", "feat/dirty": "RISKY", "feat/gone": "SAFE"}
	for branch, verdict := range verdicts {
		found := false
		for _, line := range lines {
			if strings.Contains(line, branch) {
				found = strings.HasPrefix(line, verdict)
			}
		}
		if !found {
			t.Errorf("%s should be %s:\n%s", branch, verdict, out)
		}
	}
	if sized && strings.Contains(out, "0 bytes on disk") {
		t.Errorf("sizes were not measured:\n%s", out)
	}
}
