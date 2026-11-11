package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/testrepo"
	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

// recordCommands replaces startCommand for the test and returns what was started.
func recordCommands(t *testing.T) *[]*exec.Cmd {
	t.Helper()
	var started []*exec.Cmd
	original := startCommand
	startCommand = func(cmd *exec.Cmd) error { started = append(started, cmd); return nil }
	t.Cleanup(func() { startCommand = original })
	return &started
}

func asOS(t *testing.T, name string, found map[string]bool) {
	t.Helper()
	originalOS, originalLook := goos, lookPath
	goos = name
	lookPath = func(file string) (string, error) {
		if found[file] {
			return "/bin/" + file, nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { goos, lookPath = originalOS, originalLook })
}

func TestSystemCommandsPerPlatform(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "gone")
	cases := []struct {
		os    string
		found map[string]bool
		act   func() error
		want  string // program name the command must run
		inDir bool   // path passed as working directory, not as an argument
	}{
		{"windows", nil, func() error { return OpenURL("http://x") }, "rundll32", false},
		{"darwin", nil, func() error { return OpenURL("http://x") }, "open", false},
		{"linux", nil, func() error { return OpenURL("http://x") }, "xdg-open", false},
		{"windows", nil, func() error { return Reveal(dir) }, "explorer", false},
		{"darwin", nil, func() error { return Reveal(missing) }, "open", false},
		{"linux", nil, func() error { return Reveal(dir) }, "xdg-open", false},
		{"windows", map[string]bool{"wt.exe": true}, func() error { return OpenTerminal(dir) }, "wt.exe", false},
		{"windows", nil, func() error { return OpenTerminal(dir) }, "cmd.exe", true},
		{"darwin", nil, func() error { return OpenTerminal(dir) }, "open", false},
		{"linux", map[string]bool{"xterm": true}, func() error { return OpenTerminal(dir) }, "xterm", true},
	}
	for _, tc := range cases {
		started := recordCommands(t)
		asOS(t, tc.os, tc.found)
		if err := tc.act(); err != nil {
			t.Fatalf("%s %s: %v", tc.os, tc.want, err)
		}
		cmd := (*started)[0]
		if !strings.HasSuffix(filepath.Base(cmd.Path), tc.want) && !strings.HasSuffix(cmd.Args[0], tc.want) {
			t.Errorf("%s: ran %v, want %s", tc.os, cmd.Args, tc.want)
		}
		if tc.inDir && cmd.Dir != dir {
			t.Errorf("%s %s: Dir = %q, want the worktree", tc.os, tc.want, cmd.Dir)
		}
	}
}

func TestOpenTerminalNeverPassesPathToAShell(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x&calc;y")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Skipf("can't create folder with metacharacters: %v", err)
	}
	started := recordCommands(t)
	asOS(t, "windows", map[string]bool{"wt.exe": true}) // ';' must skip Windows Terminal too
	if err := OpenTerminal(dir); err != nil {
		t.Fatal(err)
	}
	cmd := (*started)[0]
	if len(cmd.Args) != 1 || cmd.Dir != dir {
		t.Fatalf("path must only be the working directory, got args %v dir %q", cmd.Args, cmd.Dir)
	}
}

func TestOpenTerminalErrors(t *testing.T) {
	recordCommands(t)
	if err := OpenTerminal(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing folder should fail")
	}
	asOS(t, "linux", nil)
	if err := OpenTerminal(t.TempDir()); err == nil {
		t.Error("no terminal emulator should fail")
	}
}

func TestStartDeleteRejectsWhileBusy(t *testing.T) {
	s := NewState(MemorySettings())
	s.worktrees = []wtkit.Worktree{{Path: "/w"}}
	s.progress.Phase = wtkit.PhaseScanning
	if _, err := s.StartDelete([]string{"/w"}, false); !errors.Is(err, errBusy) {
		t.Fatalf("delete during scan: %v", err)
	}
	s.progress.Phase = wtkit.PhaseIdle
	s.job = &DeleteJob{ID: 4, Running: true}
	if err := s.StartScan(Target{Kind: "home"}); !errors.Is(err, errBusy) {
		t.Fatalf("scan during delete: %v", err)
	}
	if _, err := s.StartDelete([]string{"/w"}, false); !errors.Is(err, errBusy) {
		t.Fatalf("two deletes: %v", err)
	}
}

func TestDeleteReportsFailures(t *testing.T) {
	testrepo.RequireGit(t)
	repo := filepath.Join(t.TempDir(), "app")
	testrepo.New(t, repo)
	s := NewState(MemorySettings())
	// A main checkout dressed up as a worktree: Remove must refuse it.
	s.worktrees = []wtkit.Worktree{{Path: repo, RepoID: filepath.Join(repo, ".git"), Branch: "main"}}
	if _, err := s.StartDelete([]string{repo}, true); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		if snap := s.Snapshot("linux"); snap.Job != nil && !snap.Job.Running {
			r := snap.Job.Report
			if r.Removed != 0 || len(r.Failures) != 1 || !strings.Contains(r.Failures[0].Message, "refusing") {
				t.Fatalf("report: %+v", r)
			}
			if !wtkit.IsDirectory(repo) {
				t.Fatal("main checkout deleted")
			}
			return
		}
		sleepTick()
	}
	t.Fatal("delete job never finished")
}

func TestSharedVolume(t *testing.T) {
	dir := t.TempDir()
	if _, ok := sharedVolume(nil); ok {
		t.Error("no items: no reading")
	}
	got, ok := sharedVolume([]wtkit.Worktree{{Path: filepath.Join(dir, "a", "b")}})
	if !ok || wtkit.Key(got) != wtkit.Key(dir) {
		t.Errorf("want nearest existing ancestor %s, got %s %v", dir, got, ok)
	}
	if isWindowsOS() {
		if _, ok := sharedVolume([]wtkit.Worktree{{Path: `C:\a`}, {Path: `D:\b`}}); ok {
			t.Error("two drives: no reading")
		}
	}
}

func TestSummarizeSortsBySizeThenName(t *testing.T) {
	big := &wtkit.Usage{Allocated: 100}
	got := summarize([]wtkit.Worktree{
		{RepoID: "b", RepoName: "beta"}, {RepoID: "a", RepoName: "alpha"},
		{RepoID: "c", RepoName: "gamma", Usage: big}, {RepoID: "a", RepoName: "alpha"},
	})
	names := []string{got[0].Name, got[1].Name, got[2].Name}
	if strings.Join(names, ",") != "gamma,alpha,beta" || got[1].Count != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestTargetRootsAndOptions(t *testing.T) {
	if roots := (Target{Kind: "folder", Path: "/x"}).Roots(); len(roots) != 1 || roots[0] != "/x" {
		t.Errorf("folder roots %v", roots)
	}
	if roots := (Target{Kind: "home"}).Roots(); roots[0] != wtkit.Home() {
		t.Errorf("home roots %v", roots)
	}
	if roots := (Target{Kind: "all"}).Roots(); len(roots) == 0 {
		t.Error("all roots empty")
	}
	if opts := targetOptions(nil, "darwin"); opts[1].Title != "Entire Mac" {
		t.Errorf("darwin title %q", opts[1].Title)
	}
}

func TestSettingsPersistAcrossLoads(t *testing.T) {
	testrepo.Isolate(t)
	folder := t.TempDir()
	LoadSettings().SetTarget(Target{Kind: "folder", Path: folder})

	loaded := LoadSettings()
	if loaded.Target().Path != folder || len(loaded.Recent()) != 1 {
		t.Fatalf("reloaded %+v %v", loaded.Target(), loaded.Recent())
	}
}

func TestSettingsIgnoreBadFiles(t *testing.T) {
	testrepo.Isolate(t)
	s := LoadSettings()
	for _, content := range []string{"{not json", `{"target":{"kind":"folder","path":"relative"}}`} {
		testrepo.Write(t, s.path, content)
		if got := LoadSettings().Target(); got.Kind != "home" {
			t.Errorf("%q: target %+v, want home", content, got)
		}
	}
}

func TestSettingsSaveFailureIsNotFatal(t *testing.T) {
	testrepo.Isolate(t)
	s := LoadSettings()
	// Make the config folder a file so MkdirAll fails.
	testrepo.Write(t, filepath.Dir(filepath.Dir(s.path)), "blocker")
	s.path = filepath.Join(filepath.Dir(filepath.Dir(s.path)), "CleanMyWT", "settings.json")
	s.SetTarget(Target{Kind: "all"})
	if s.Target().Kind != "all" {
		t.Fatal("in-memory target should still change")
	}
}
