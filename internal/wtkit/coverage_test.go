package wtkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBadgesAndDetail(t *testing.T) {
	zero, three, ahead, behind := 0, 3, 1, 2
	yes, no := true, false
	cases := []struct {
		name       string
		wt         Worktree
		badges     string
		detailHas  []string
		branchName string
	}{
		{"prunable", Worktree{Path: "/w", Prunable: true, Branch: "b"}, "Folder already gone",
			[]string{"no longer exists", "Branch: b"}, "b"},
		{"checking", Worktree{Path: "/w", InUse: true, Locked: true, LockReason: "agent"}, "In use,Locked,Checking…",
			[]string{"Locked: agent", "running process"}, "—"},
		{"orphan", OrphanWorktree("/w"), "Repo not found", []string{"can't be found"}, "—"},
		{"dirty and unpushed", Worktree{Path: "/w", Detached: true, Head: "0123456789abcdef", Status: &Status{
			ChangedFiles: 1, UntrackedFiles: 2, Upstream: "origin/x", Ahead: &ahead, Behind: &behind, Unpushed: &three, Merged: &no,
		}}, "3 uncommitted,3 unpushed", []string{"Detached HEAD at 01234567", "ahead 1, behind 2", "1 modified", "2 untracked",
			"3 commits not on any remote", "Not merged"}, "detached @ 0123456"},
		{"merged", Worktree{Path: "/w", Status: &Status{Merged: &yes}}, "Merged", []string{"Fully merged"}, "—"},
		{"pushed", Worktree{Path: "/w", Status: &Status{Unpushed: &zero}}, "Pushed", nil, "—"},
		{"clean", Worktree{Path: "/w", Status: &Status{}}, "Clean", nil, "—"},
		{"git error", Worktree{Path: "/w", Status: &Status{Error: "boom"}}, "Git error", []string{"boom"}, "—"},
		{"hard links", Worktree{Path: "/w", Status: &Status{}, Usage: &Usage{Allocated: 5000, Reclaimable: 1000}}, "Clean",
			[]string{"4 KB is hard-linked"}, "—"},
	}
	for _, c := range cases {
		var texts []string
		for _, b := range c.wt.Badges() {
			texts = append(texts, b.Text)
		}
		if got := strings.Join(texts, ","); got != c.badges {
			t.Errorf("%s: badges %q, want %q", c.name, got, c.badges)
		}
		detail := c.wt.Detail()
		for _, want := range c.detailHas {
			if !strings.Contains(detail, want) {
				t.Errorf("%s: detail lacks %q:\n%s", c.name, want, detail)
			}
		}
		if got := c.wt.BranchLabel(); got != c.branchName {
			t.Errorf("%s: branch label %q, want %q", c.name, got, c.branchName)
		}
	}
	if got := (Worktree{Detached: true}).BranchLabel(); got != "detached @ ?" {
		t.Errorf("detached without head: %q", got)
	}
}

func TestVerdictJSONRoundTrip(t *testing.T) {
	for _, v := range []Verdict{Safe, Review, Risky, Pending} {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var back Verdict
		if err := json.Unmarshal(raw, &back); err != nil || back != v {
			t.Errorf("%s: %s → %v (%v)", v, raw, back, err)
		}
	}
	var v Verdict
	if json.Unmarshal([]byte(`"maybe"`), &v) == nil || json.Unmarshal([]byte(`3`), &v) == nil {
		t.Error("bad verdicts should not decode")
	}
}

func TestBytesFreed(t *testing.T) {
	if (Worktree{}).BytesFreed() != 0 || (Worktree{Usage: &Usage{Reclaimable: 7}}).BytesFreed() != 7 {
		t.Error("BytesFreed")
	}
}

func TestHomePathHelpers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if Home() != home {
		t.Fatalf("Home() = %s", Home())
	}
	inside := filepath.Join(home, "code", "wt")
	cases := map[string]string{
		home:   "~",
		inside: "~" + string(filepath.Separator) + filepath.Join("code", "wt"),
		filepath.Join(filepath.Dir(home), "other"): filepath.Join(filepath.Dir(home), "other"),
	}
	for in, want := range cases {
		if got := Abbreviate(in); got != want {
			t.Errorf("Abbreviate(%s) = %s, want %s", in, got, want)
		}
	}
	if ExpandTilde("~") != home || ExpandTilde("~/code") != filepath.Join(home, "code") ||
		ExpandTilde(`~\code`) != filepath.Join(home, "code") || ExpandTilde("/abs") != "/abs" {
		t.Error("ExpandTilde")
	}
}

func TestSmallHelpers(t *testing.T) {
	if (Result{Stderr: " oops \n"}).ErrorOutput() != "oops" {
		t.Error("ErrorOutput")
	}
	if got := (Result{Stdout: "a\r\nb\n"}).Lines(); len(got) != 2 || got[1] != "b" {
		t.Errorf("Lines %q", got)
	}
	if args := RepoArgs("/srv/app.git"); args[0] != "--git-dir" {
		t.Errorf("bare RepoArgs %v", args)
	}
	if workers(3, 8) != 3 || workers(0, 8) != 8 {
		t.Error("workers")
	}
	if len(gitCandidates()) == 0 {
		t.Error("no git fallback locations")
	}
	if roots := EntireComputerRoots(); len(roots) == 0 {
		t.Error("no roots for an entire-computer scan")
	}
	if free, ok := AvailableBytes(t.TempDir()); !ok || free <= 0 {
		t.Errorf("AvailableBytes = %d, %v", free, ok)
	}
	if _, ok := AvailableBytes(filepath.Join(t.TempDir(), "missing", "x")); ok && isWindows {
		t.Error("missing path should fail on Windows")
	}
	if ModTime(filepath.Join(t.TempDir(), "missing")) != nil {
		t.Error("ModTime of missing file")
	}
}

func TestInspectStatusWithRemote(t *testing.T) {
	requireGit(t)
	root := Real(t.TempDir())
	remote := filepath.Join(root, "remote.git")
	git(t, root, "init", "-q", "--bare", "-b", "main", remote)
	repo := filepath.Join(root, "app")
	newRepo(t, repo)
	git(t, repo, "remote", "add", "origin", remote)
	git(t, repo, "push", "-q", "-u", "origin", "main")
	git(t, repo, "remote", "set-head", "origin", "main")

	pushed := filepath.Join(root, "pushed")
	git(t, repo, "worktree", "add", "-q", "-b", "pushed", pushed)
	writeFile(t, filepath.Join(pushed, "f"), "x")
	git(t, pushed, "add", ".")
	git(t, pushed, "commit", "-q", "-m", "f")
	git(t, pushed, "push", "-q", "-u", "origin", "pushed")
	local := filepath.Join(root, "local")
	git(t, repo, "worktree", "add", "-q", "-b", "local", local)
	writeFile(t, filepath.Join(local, "g"), "y")
	git(t, local, "add", ".")
	git(t, local, "commit", "-q", "-m", "g")

	r, worktrees, ok := LoadRepository(filepath.Join(repo, ".git"))
	if !ok || !r.HasRemotes || r.BaseRef != "origin/main" {
		t.Fatalf("repo %+v ok=%v", r, ok)
	}
	for _, wt := range worktrees {
		s := InspectStatus(wt, r)
		wt.Status = &s
		switch wt.Name() {
		case "pushed":
			if wt.Verdict() != Safe || s.Upstream != "origin/pushed" || deref(s.Unpushed) != 0 || *s.Merged {
				t.Errorf("pushed: %s %+v", wt.Verdict(), s)
			}
		case "local":
			if wt.Verdict() != Risky || deref(s.Unpushed) != 1 {
				t.Errorf("local: %s %+v", wt.Verdict(), s)
			}
		}
	}
}

func TestInspectStatusReportsGitErrors(t *testing.T) {
	requireGit(t)
	s := InspectStatus(Worktree{Path: t.TempDir()}, Repository{})
	if s.Error == "" {
		t.Fatal("git status outside a repository should report an error")
	}
}

func TestLoadBareRepository(t *testing.T) {
	requireGit(t)
	root := Real(t.TempDir())
	src := filepath.Join(root, "src")
	newRepo(t, src)
	bare := filepath.Join(root, "proj.git")
	git(t, root, "clone", "-q", "--bare", src, bare)
	wt := filepath.Join(root, "wt")
	git(t, bare, "worktree", "add", "-q", "-b", "feat", wt)

	repo, worktrees, ok := LoadRepository(bare)
	if !ok || repo.Name != "proj" || repo.MainPath != "" || len(worktrees) != 1 {
		t.Fatalf("bare repo %+v %v %v", repo, worktrees, ok)
	}
	if _, _, ok := LoadRepository(filepath.Join(root, "nothing")); ok {
		t.Error("missing repo should not load")
	}
}

func TestRemoveVariants(t *testing.T) {
	requireGit(t)
	root := Real(t.TempDir())
	repo := filepath.Join(root, "app")
	newRepo(t, repo)
	common := filepath.Join(repo, ".git")
	add := func(name string) string {
		path := filepath.Join(root, name)
		git(t, repo, "worktree", "add", "-q", "-b", name, path)
		return path
	}
	byPath := func(path string) Worktree {
		_, wts, _ := LoadRepository(common)
		for _, wt := range wts {
			if Key(Real(wt.Path)) == Key(Real(path)) || Key(wt.Path) == Key(path) {
				return wt
			}
		}
		t.Fatalf("%s not listed", path)
		return Worktree{}
	}

	// Locked worktree: needs --force twice.
	locked := add("locked")
	git(t, repo, "worktree", "lock", "--reason", "agent", locked)
	if wt := byPath(locked); !wt.Locked || wt.LockReason != "agent" {
		t.Fatalf("lock not parsed: %+v", wt)
	} else if err := Remove(wt); err != nil || Exists(locked) {
		t.Fatalf("locked remove: %v", err)
	}

	// Prunable with an admin dir: only the record goes.
	gone := add("gone")
	wt := byPath(gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	wt.Prunable = true
	if err := Remove(wt); err != nil || Exists(wt.AdminDir) {
		t.Fatalf("prunable remove: %v", err)
	}

	// Prunable without a known admin dir falls back to `git worktree prune`.
	pruned := add("pruned")
	wt = byPath(pruned)
	_ = os.RemoveAll(pruned)
	wt.Prunable, wt.AdminDir = true, ""
	if err := Remove(wt); err != nil {
		t.Fatalf("prune fallback: %v", err)
	}

	// git refuses (unknown repo): finish by hand, including the admin dir.
	manual := add("manual")
	wt = byPath(manual)
	wt.RepoID = filepath.Join(root, "not-a-repo")
	if err := Remove(wt); err != nil || Exists(manual) || Exists(wt.AdminDir) {
		t.Fatalf("manual fallback: %v", err)
	}

	// Orphan: just the folder.
	orphan := filepath.Join(root, "orphan")
	writeFile(t, filepath.Join(orphan, ".git"), "gitdir: /nowhere/.git/worktrees/orphan\n")
	if err := Remove(OrphanWorktree(orphan)); err != nil || Exists(orphan) {
		t.Fatalf("orphan remove: %v", err)
	}

	if len(strings.Split(strings.TrimSpace(git(t, repo, "worktree", "list")), "\n")) != 1 {
		t.Errorf("leftover worktrees:\n%s", git(t, repo, "worktree", "list"))
	}
}

func TestScanSkipsHeavyFolders(t *testing.T) {
	requireGit(t)
	root := Real(t.TempDir())
	hidden := filepath.Join(root, "node_modules", "pkg")
	newRepo(t, hidden)
	git(t, hidden, "worktree", "add", "-q", "-b", "x", filepath.Join(hidden, "..", "wt"))

	var calls int
	result := ScanRepos(context.Background(), []string{root}, func(int, string) { calls++ })
	if len(result.CommonDirs) != 0 {
		t.Errorf("node_modules should be skipped, found %v", result.CommonDirs)
	}
	if !skippedPaths()[Key(filepath.Join(Home(), "go", "pkg"))] {
		t.Error("~/go/pkg should be skipped")
	}
	_ = calls
}

func TestFullScanCancelled(t *testing.T) {
	requireGit(t)
	root := Real(t.TempDir())
	repo := filepath.Join(root, "app")
	newRepo(t, repo)
	git(t, repo, "worktree", "add", "-q", "-b", "x", filepath.Join(root, "wt"))

	ctx, cancel := contextWithCancel(t)
	cancel()
	report := FullScan(ctx, Options{Roots: []string{root}, MeasureSizes: true}, Events{})
	if len(report.Worktrees) != 0 {
		t.Errorf("cancelled scan returned %d worktrees", len(report.Worktrees))
	}
}

func TestFullScanEvents(t *testing.T) {
	requireGit(t)
	root := Real(t.TempDir())
	repo := filepath.Join(root, "app")
	newRepo(t, repo)
	git(t, repo, "worktree", "add", "-q", "-b", "x", filepath.Join(root, "wt"))

	var listed, updated int
	var phases []Phase
	FullScan(context.Background(), Options{Roots: []string{root}, MeasureSizes: true, StatusWorkers: 1, MeasureWorkers: 1}, Events{
		Progress: func(p Progress) { phases = append(phases, p.Phase) },
		Listed:   func(_ map[string]Repository, wts []Worktree) { listed = len(wts) },
		Updated:  func(Worktree) { updated++ },
	})
	if listed != 1 || updated != 2 {
		t.Errorf("listed %d updated %d", listed, updated)
	}
	if !strings.Contains(joinPhases(phases), "measuring") {
		t.Errorf("phases %v", phases)
	}
}

func TestLastActivityFallsBackToFolder(t *testing.T) {
	dir := t.TempDir()
	if got := lastActivity("", dir); got == nil || time.Since(*got) > time.Hour {
		t.Errorf("lastActivity = %v", got)
	}
}

func joinPhases(phases []Phase) string {
	var parts []string
	for _, p := range phases {
		parts = append(parts, string(p))
	}
	return strings.Join(parts, ",")
}

func TestRealResolvesMissingPathsThroughExistingParent(t *testing.T) {
	base := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(base, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	got := Real(filepath.Join(link, "gone", "deeper"))
	want := filepath.Join(Real(base), "gone", "deeper")
	if Key(got) != Key(want) {
		t.Fatalf("Real through symlinked parent = %s, want %s", got, want)
	}
}
