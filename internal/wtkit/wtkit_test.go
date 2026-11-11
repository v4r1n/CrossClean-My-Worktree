package wtkit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// git runs git in dir for test setup, failing the test on error.
func git(t *testing.T, dir string, args ...string) string {
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo creates a repository with one commit on main.
func newRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "README"), "hello\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "init")
}

func byName(t *testing.T, items []Worktree, name string) Worktree {
	t.Helper()
	for _, wt := range items {
		if wt.Name() == name {
			return wt
		}
	}
	t.Fatalf("worktree %q not found among %d", name, len(items))
	return Worktree{}
}

func requireGit(t *testing.T) {
	t.Helper()
	if !GitAvailable() {
		t.Skip("git not available")
	}
}

func TestFullScanClassifiesWorktrees(t *testing.T) {
	requireGit(t)
	root := Real(t.TempDir())
	repo := filepath.Join(root, "app")
	newRepo(t, repo)

	git(t, repo, "worktree", "add", "-q", "-b", "merged", filepath.Join(root, "wt-merged"))
	git(t, repo, "worktree", "add", "-q", "-b", "dirty", filepath.Join(root, "wt-dirty"))
	writeFile(t, filepath.Join(root, "wt-dirty", "new.txt"), "work in progress\n")
	git(t, repo, "worktree", "add", "-q", "-b", "ahead", filepath.Join(root, "wt-ahead"))
	writeFile(t, filepath.Join(root, "wt-ahead", "feature.txt"), "x\n")
	git(t, filepath.Join(root, "wt-ahead"), "add", ".")
	git(t, filepath.Join(root, "wt-ahead"), "commit", "-q", "-m", "feature")
	git(t, repo, "worktree", "add", "-q", "-b", "gone", filepath.Join(root, "wt-gone"))
	if err := os.RemoveAll(filepath.Join(root, "wt-gone")); err != nil {
		t.Fatal(err)
	}

	// A worktree whose repository was deleted.
	lost := filepath.Join(root, "lost")
	newRepo(t, lost)
	git(t, lost, "worktree", "add", "-q", "-b", "orphan", filepath.Join(root, "wt-orphan"))
	if err := os.RemoveAll(lost); err != nil {
		t.Fatal(err)
	}

	report := FullScan(context.Background(), Options{Roots: []string{root}, MeasureSizes: true}, Events{})

	if len(report.Repos) != 1 {
		t.Fatalf("want 1 repo, got %d", len(report.Repos))
	}
	cases := map[string]Verdict{
		"wt-merged": Safe, "wt-dirty": Risky, "wt-ahead": Risky, "wt-gone": Safe, "wt-orphan": Review,
	}
	for name, want := range cases {
		wt := byName(t, report.Worktrees, name)
		if got := wt.Verdict(); got != want {
			t.Errorf("%s: verdict %s, want %s (status %+v)", name, got, want, wt.Status)
		}
	}
	if !byName(t, report.Worktrees, "wt-gone").Prunable {
		t.Error("wt-gone should be prunable")
	}
	if !byName(t, report.Worktrees, "wt-orphan").Orphan {
		t.Error("wt-orphan should be an orphan")
	}
	if u := byName(t, report.Worktrees, "wt-merged").Usage; u == nil || u.Allocated <= 0 {
		t.Errorf("wt-merged usage not measured: %+v", u)
	}
	if dirty := byName(t, report.Worktrees, "wt-dirty"); dirty.Status.UntrackedFiles != 1 {
		t.Errorf("wt-dirty untracked = %d, want 1", dirty.Status.UntrackedFiles)
	}
}

func TestFullScanFindsRepoOutsideRootViaGitFile(t *testing.T) {
	requireGit(t)
	base := Real(t.TempDir())
	repo := filepath.Join(base, "elsewhere", "repo")
	scanned := filepath.Join(base, "scanned")
	newRepo(t, repo)
	git(t, repo, "worktree", "add", "-q", "-b", "feat", filepath.Join(scanned, "wt"))

	report := FullScan(context.Background(), Options{Roots: []string{scanned}}, Events{})
	if len(report.Worktrees) != 1 || report.Worktrees[0].RepoName != "repo" {
		t.Fatalf("want the repo's worktree, got %+v", report.Worktrees)
	}
}

func TestRemoveDeletesWorktreeAndRecord(t *testing.T) {
	requireGit(t)
	root := Real(t.TempDir())
	repo := filepath.Join(root, "app")
	newRepo(t, repo)
	wtPath := filepath.Join(root, "wt")
	git(t, repo, "worktree", "add", "-q", "-b", "feat", wtPath)
	writeFile(t, filepath.Join(wtPath, "scratch.txt"), "x\n")

	commonDir := filepath.Join(repo, ".git")
	_, worktrees, ok := LoadRepository(commonDir)
	if !ok || len(worktrees) != 1 {
		t.Fatalf("LoadRepository: ok=%v worktrees=%v", ok, worktrees)
	}
	if err := Remove(worktrees[0]); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if Exists(wtPath) {
		t.Error("worktree folder still exists")
	}
	if out := git(t, repo, "worktree", "list", "--porcelain"); strings.Count(out, "worktree ") != 1 {
		t.Errorf("git still lists the worktree:\n%s", out)
	}
	if r := DeleteBranch("feat", commonDir); !r.OK() {
		t.Errorf("merged branch should delete: %s", r.ErrorOutput())
	}
}

func TestRemoveRefusesMainCheckout(t *testing.T) {
	requireGit(t)
	repo := filepath.Join(Real(t.TempDir()), "app")
	newRepo(t, repo)
	err := Remove(Worktree{Path: repo, RepoID: filepath.Join(repo, ".git")})
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("want refusal, got %v", err)
	}
	if !Exists(filepath.Join(repo, "README")) {
		t.Fatal("main checkout was touched")
	}
}

func TestRemoveAllClearsReadOnlyFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tree")
	file := filepath.Join(dir, "sub", "locked.txt")
	writeFile(t, file, "x")
	if err := os.Chmod(file, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := removeAll(dir); err != nil {
		t.Fatalf("removeAll: %v", err)
	}
	if Exists(dir) {
		t.Fatal("tree still exists")
	}
}

func TestMeasureExcludesFilesHardLinkedFromOutside(t *testing.T) {
	base := t.TempDir()
	tree := filepath.Join(base, "tree")
	inside := filepath.Join(tree, "own.bin")
	shared := filepath.Join(tree, "shared.bin")
	writeFile(t, inside, strings.Repeat("a", 100_000))
	writeFile(t, shared, strings.Repeat("b", 100_000))
	if err := os.Link(shared, filepath.Join(base, "store.bin")); err != nil {
		t.Skipf("hard links unsupported: %v", err)
	}

	u := Measure(context.Background(), tree)
	if u.Files != 2 {
		t.Errorf("files = %d, want 2", u.Files)
	}
	if u.Reclaimable >= u.Allocated {
		t.Errorf("reclaimable %d should be below allocated %d", u.Reclaimable, u.Allocated)
	}
	if u.Reclaimable < 100_000 {
		t.Errorf("reclaimable %d should include the unshared file", u.Reclaimable)
	}
}

func TestParseWorktreeList(t *testing.T) {
	out := "worktree /r\r\nHEAD abc\r\nbranch refs/heads/main\r\n\r\n" +
		"worktree /r/wt\nHEAD def\ndetached\nlocked agent busy\nprunable gitdir file points to non-existent location\n\n" +
		"worktree /bare\nbare\n"
	entries := parseWorktreeList(out)
	if len(entries) != 3 {
		t.Fatalf("got %d entries", len(entries))
	}
	if entries[0].branch != "main" || entries[0].head != "abc" {
		t.Errorf("entry 0: %+v", entries[0])
	}
	e := entries[1]
	if !e.detached || !e.locked || e.lockReason != "agent busy" || !e.prunable {
		t.Errorf("entry 1: %+v", e)
	}
	if !entries[2].bare {
		t.Errorf("entry 2: %+v", entries[2])
	}
}

func TestParseStatus(t *testing.T) {
	var s Status
	parseStatus([]string{
		"# branch.oid abc", "# branch.head feat", "# branch.upstream origin/feat", "# branch.ab +2 -3",
		"1 .M N... 100644 100644 100644 a b file.go", "2 R. N... 100644 100644 100644 a b R100 n\to",
		"u UU N... 1 2 3 4 a b c f", "? new.txt",
	}, &s)
	if s.Upstream != "origin/feat" || deref(s.Ahead) != 2 || deref(s.Behind) != 3 {
		t.Errorf("branch info: %+v", s)
	}
	if s.ChangedFiles != 3 || s.UntrackedFiles != 1 {
		t.Errorf("counts: changed %d untracked %d", s.ChangedFiles, s.UntrackedFiles)
	}
}

func TestVerdict(t *testing.T) {
	zero, two := 0, 2
	yes, no := true, false
	cases := []struct {
		name string
		wt   Worktree
		want Verdict
	}{
		{"prunable wins", Worktree{Prunable: true, InUse: true}, Safe},
		{"in use", Worktree{InUse: true, Status: &Status{}}, Risky},
		{"orphan", Worktree{Orphan: true}, Review},
		{"pending", Worktree{}, Pending},
		{"git error", Worktree{Status: &Status{Error: "x"}}, Review},
		{"uncommitted", Worktree{Status: &Status{UntrackedFiles: 1, Merged: &yes}}, Risky},
		{"locked", Worktree{Locked: true, Status: &Status{Merged: &yes}}, Review},
		{"merged", Worktree{Status: &Status{Merged: &yes}}, Safe},
		{"pushed", Worktree{Status: &Status{Unpushed: &zero, Merged: &no}}, Safe},
		{"unpushed", Worktree{Status: &Status{Unpushed: &two}}, Risky},
		{"local only", Worktree{Status: &Status{Merged: &no}}, Risky},
		{"unknown", Worktree{Status: &Status{}}, Review},
	}
	for _, c := range cases {
		if got := c.wt.Verdict(); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{0: "0 bytes", 1: "1 byte", 999: "999 bytes", 12_000: "12 KB",
		388_600_000: "388.6 MB", 2_550_000_000: "2.55 GB"}
	for in, want := range cases {
		if got := FormatBytes(in); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestIsWithin(t *testing.T) {
	sep := string(filepath.Separator)
	parent := filepath.Join(sep+"a", "wt")
	if !IsWithin(filepath.Join(parent, "src"), parent) || !IsWithin(parent, parent) {
		t.Error("child should be within parent")
	}
	if IsWithin(parent+"2", parent) {
		t.Error("sibling with shared prefix is not within")
	}
	if isWindows && !IsWithin(`C:\Code\WT\src`, `c:\code\wt`) {
		t.Error("Windows comparison should ignore case")
	}
}

func TestResolveGitFileSkipsSubmodules(t *testing.T) {
	dir := t.TempDir()
	gitFile := filepath.Join(dir, ".git")
	writeFile(t, gitFile, "gitdir: ../.git/modules/lib\n")
	if target, _ := resolveGitFile(gitFile); target != gitFileOther {
		t.Errorf("submodule should be ignored, got %v", target)
	}
}

func TestProcessWorkingDirectoriesSeesChildProcess(t *testing.T) {
	requireGit(t)
	dir := Real(t.TempDir())
	// `git cat-file --batch` waits on stdin, so it sits in dir until we close it.
	cmd := exec.Command("git", "cat-file", "--batch")
	cmd.Dir = dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close(); _ = cmd.Wait() }()

	for _, cwd := range ProcessWorkingDirectories() {
		if Key(Real(cwd)) == Key(dir) {
			return
		}
	}
	t.Fatalf("child process working in %s not found", dir)
}

func TestMeasureCountsFilesBeyondMaxPath(t *testing.T) {
	root := t.TempDir()
	deep := root
	for len(deep) < 300 {
		deep = filepath.Join(deep, "node_modules_nested_folder")
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Skipf("long paths unsupported: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deep, "index.js"), []byte(strings.Repeat("x", 50_000)), 0o644); err != nil {
		t.Skipf("long paths unsupported: %v", err)
	}
	u := Measure(context.Background(), root)
	if u.Files != 1 || u.Allocated < 50_000 {
		t.Fatalf("file beyond MAX_PATH not counted: %+v", u)
	}
}
