package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/testrepo"
	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

// client drives a real server over HTTP, the way the browser UI does.
type client struct {
	t     *testing.T
	base  string
	token string
}

func startServer(t *testing.T) *client {
	t.Helper()
	srv := NewServer(NewState(MemorySettings()))
	ln, err := srv.Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := &http.Server{Handler: srv.Handler()}
	go func() { _ = httpServer.Serve(ln) }()
	t.Cleanup(func() { _ = httpServer.Close() })
	base, _, _ := strings.Cut(srv.URL(), "/#")
	return &client{t: t, base: base, token: srv.Token()}
}

func (c *client) call(method, path string, body any, out any) int {
	c.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, c.base+path, reader)
	req.Header.Set(TokenHeader, c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			c.t.Fatal(err)
		}
	}
	return res.StatusCode
}

func (c *client) state() Snapshot {
	c.t.Helper()
	var snap Snapshot
	if code := c.call("GET", "/api/state", nil, &snap); code != http.StatusOK {
		c.t.Fatalf("state: %d", code)
	}
	return snap
}

// waitUntil polls the state until done reports true.
func (c *client) waitUntil(what string, done func(Snapshot) bool) Snapshot {
	c.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if snap := c.state(); done(snap) {
			return snap
		}
		time.Sleep(50 * time.Millisecond)
	}
	c.t.Fatalf("timed out waiting for %s", what)
	return Snapshot{}
}

func rowFor(t *testing.T, snap Snapshot, path string) Row {
	t.Helper()
	for _, row := range snap.Rows {
		if wtkit.Key(wtkit.Real(row.Path)) == wtkit.Key(wtkit.Real(path)) {
			return row
		}
	}
	t.Fatalf("no row for %s", path)
	return Row{}
}

// TestEndToEndScanAndDelete is the main user flow over real HTTP: scan a
// folder, check verdicts, delete the safe worktrees and their branches, and
// confirm git agrees.
func TestEndToEndScanAndDelete(t *testing.T) {
	testrepo.RequireGit(t)
	root := t.TempDir()
	fx := testrepo.NewFixture(t, root)
	c := startServer(t)

	if code := c.call("POST", "/api/scan", Target{Kind: "folder", Path: root}, nil); code != http.StatusOK {
		t.Fatalf("scan: %d", code)
	}
	snap := c.waitUntil("scan", func(s Snapshot) bool { return s.Progress.Phase == wtkit.PhaseIdle && s.LastScan != nil })

	if len(snap.Rows) != 3 || len(snap.Repos) != 1 || snap.Repos[0].Count != 3 {
		t.Fatalf("rows %d repos %+v", len(snap.Rows), snap.Repos)
	}
	want := map[string]wtkit.Verdict{fx.Merged: wtkit.Safe, fx.Dirty: wtkit.Risky, fx.Gone: wtkit.Safe}
	for path, verdict := range want {
		if got := rowFor(t, snap, path).Verdict; got != verdict {
			t.Errorf("%s: %s, want %s", filepath.Base(path), got, verdict)
		}
	}
	if snap.Target.Kind != "folder" || snap.Targets[2].Target.Path != snap.Target.Path {
		t.Errorf("folder should be the target and first recent: %+v", snap.Targets)
	}

	merged, gone := rowFor(t, snap, fx.Merged), rowFor(t, snap, fx.Gone)
	var started map[string]int
	code := c.call("POST", "/api/delete", map[string]any{
		"paths": []string{merged.Path, gone.Path}, "deleteBranches": true,
	}, &started)
	if code != http.StatusOK || started["job"] != 1 {
		t.Fatalf("delete: %d %v", code, started)
	}
	snap = c.waitUntil("delete", func(s Snapshot) bool { return s.Job != nil && !s.Job.Running })

	report := snap.Job.Report
	if report.Removed != 2 || len(report.Failures) != 0 || report.BranchesDeleted != 2 {
		t.Fatalf("report: %+v", report)
	}
	if len(snap.Rows) != 1 || wtkit.Key(wtkit.Real(snap.Rows[0].Path)) != wtkit.Key(wtkit.Real(fx.Dirty)) {
		t.Fatalf("only the dirty worktree should remain: %+v", snap.Rows)
	}
	left := testrepo.Worktrees(t, fx.Repo)
	if len(left) != 1 || wtkit.Key(wtkit.Real(left[0])) != wtkit.Key(wtkit.Real(fx.Dirty)) {
		t.Fatalf("git still lists %v", left)
	}
	branches := testrepo.Git(t, fx.Repo, "branch", "--format=%(refname:short)")
	if strings.Contains(branches, "feat/merged") || !strings.Contains(branches, "feat/dirty") {
		t.Fatalf("branches: %q", branches)
	}
	if wtkit.Exists(fx.Merged) {
		t.Fatal("merged worktree folder still exists")
	}
}

func TestEndToEndRejections(t *testing.T) {
	testrepo.RequireGit(t)
	root := t.TempDir()
	fx := testrepo.NewFixture(t, root)
	c := startServer(t)
	c.call("POST", "/api/scan", Target{Kind: "folder", Path: root}, nil)
	snap := c.waitUntil("scan", func(s Snapshot) bool { return s.Progress.Phase == wtkit.PhaseIdle && s.LastScan != nil })
	dirty := rowFor(t, snap, fx.Dirty)

	cases := []struct {
		name, path string
		body       any
		want       int
	}{
		{"main checkout is not in the scan", "/api/delete", map[string]any{"paths": []string{fx.Repo}}, 400},
		{"empty delete", "/api/delete", map[string]any{"paths": []string{}}, 400},
		{"bad json", "/api/delete", "not an object", 400},
		{"unknown target", "/api/scan", Target{Kind: "everything"}, 400},
		{"relative folder", "/api/scan", Target{Kind: "folder", Path: "nope-not-here"}, 400},
		{"unknown open action", "/api/open", map[string]string{"path": dirty.Path, "action": "format"}, 400},
		{"open outside scan", "/api/open", map[string]string{"path": fx.Repo, "action": "reveal"}, 400},
	}
	for _, tc := range cases {
		var out map[string]any
		if got := c.call("POST", tc.path, tc.body, &out); got != tc.want || out["error"] == nil {
			t.Errorf("%s: got %d %v, want %d with error", tc.name, got, out, tc.want)
		}
	}
	if !wtkit.IsDirectory(fx.Repo) || !wtkit.IsDirectory(fx.Dirty) {
		t.Fatal("a rejected request deleted something")
	}
}

func TestEndToEndOpenUsesPlatformCommand(t *testing.T) {
	testrepo.RequireGit(t)
	root := t.TempDir()
	fx := testrepo.NewFixture(t, root)
	started := recordCommands(t)
	// CI runners have no terminal emulator or Windows Terminal; pretend they do.
	asOS(t, goos, map[string]bool{"wt.exe": true, "x-terminal-emulator": true})
	c := startServer(t)
	c.call("POST", "/api/scan", Target{Kind: "folder", Path: root}, nil)
	snap := c.waitUntil("scan", func(s Snapshot) bool { return s.Progress.Phase == wtkit.PhaseIdle && s.LastScan != nil })
	path := rowFor(t, snap, fx.Dirty).Path

	for _, action := range []string{"reveal", "terminal"} {
		if code := c.call("POST", "/api/open", map[string]string{"path": path, "action": action}, nil); code != http.StatusOK {
			t.Fatalf("%s: %d", action, code)
		}
	}
	if len(*started) != 2 {
		t.Fatalf("started %d commands", len(*started))
	}
	if !slices.ContainsFunc((*started)[0].Args, func(a string) bool { return strings.Contains(a, filepath.Base(path)) }) {
		t.Errorf("reveal args %v lack the path", (*started)[0].Args)
	}
}

func TestEndToEndStopScan(t *testing.T) {
	testrepo.RequireGit(t)
	c := startServer(t)
	root := t.TempDir()
	testrepo.NewFixture(t, root)
	c.call("POST", "/api/scan", Target{Kind: "folder", Path: root}, nil)
	if code := c.call("POST", "/api/stop", map[string]any{}, nil); code != http.StatusOK {
		t.Fatalf("stop: %d", code)
	}
	if snap := c.state(); snap.Progress.Phase != wtkit.PhaseIdle {
		t.Fatalf("phase after stop: %s", snap.Progress.Phase)
	}
}
