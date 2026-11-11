package wtkit

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type listEntry struct {
	path       string
	head       string
	branch     string
	bare       bool
	detached   bool
	locked     bool
	lockReason string
	prunable   bool
}

// LoadRepository loads a repository and its linked worktrees (the main working
// tree is excluded). ok is false when git can't read the repository.
func LoadRepository(commonDir string) (repo Repository, worktrees []Worktree, ok bool) {
	repoArgs := RepoArgs(commonDir)
	list := Git(append(repoArgs, "worktree", "list", "--porcelain")...)
	if !list.OK() {
		return Repository{}, nil, false
	}
	entries := parseWorktreeList(list.Stdout)
	if len(entries) == 0 {
		return Repository{}, nil, false
	}
	main := entries[0]

	mainPath := ""
	if !main.bare {
		mainPath = main.path
	}
	name := filepath.Base(commonDir)
	if mainPath != "" {
		name = filepath.Base(mainPath)
	}
	if main.bare {
		name = strings.TrimSuffix(name, ".git")
	}

	repo = Repository{
		CommonDir:  commonDir,
		Name:       name,
		MainPath:   mainPath,
		BaseRef:    resolveBaseRef(repoArgs),
		HasRemotes: Git(append(repoArgs, "remote")...).Output() != "",
	}

	adminDirs := adminDirectories(commonDir)
	for _, entry := range entries[1:] {
		if entry.bare {
			continue
		}
		wt := Worktree{
			Path:       entry.path,
			RepoID:     commonDir,
			RepoName:   name,
			Head:       entry.head,
			Branch:     entry.branch,
			Detached:   entry.detached,
			Locked:     entry.locked,
			LockReason: entry.lockReason,
			Prunable:   entry.prunable || !IsDirectory(entry.path),
		}
		wt.AdminDir = adminDirs[Key(entry.path)]
		if wt.AdminDir == "" {
			wt.AdminDir = adminDirs[Key(Real(entry.path))]
		}
		wt.LastActivity = lastActivity(wt.AdminDir, entry.path)
		worktrees = append(worktrees, wt)
	}
	return repo, worktrees, true
}

// OrphanWorktree describes a worktree whose repository has been deleted.
func OrphanWorktree(path string) Worktree {
	return Worktree{
		Path:         path,
		RepoID:       OrphanRepoID,
		RepoName:     "Main repo not found",
		Orphan:       true,
		LastActivity: ModTime(path),
		Status:       &Status{Error: "The repository this worktree belongs to can't be found. It was deleted, or it is on a drive that is not connected."},
	}
}

// InspectStatus runs git status and related checks for one worktree.
func InspectStatus(wt Worktree, repo Repository) Status {
	var status Status
	result := Git("-C", wt.Path, "status", "--porcelain=v2", "--branch")
	if !result.OK() {
		status.Error = result.ErrorOutput()
		if status.Error == "" {
			status.Error = "git status failed"
		}
		return status
	}
	parseStatus(result.Lines(), &status)

	if repo.HasRemotes {
		count := Git("-C", wt.Path, "rev-list", "--count", "HEAD", "--not", "--remotes")
		if n, err := strconv.Atoi(count.Output()); count.OK() && err == nil {
			status.Unpushed = &n
		}
	}
	if repo.BaseRef != "" {
		merged := Git("-C", wt.Path, "merge-base", "--is-ancestor", "HEAD", repo.BaseRef)
		switch merged.Status {
		case 0:
			status.Merged = ptr(true)
		case 1:
			status.Merged = ptr(false)
		}
	}
	return status
}

func parseStatus(lines []string, status *Status) {
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "# branch.upstream "):
			status.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
		case strings.HasPrefix(line, "# branch.ab "):
			parts := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(parts) == 2 {
				if ahead, err := strconv.Atoi(strings.TrimPrefix(parts[0], "+")); err == nil {
					status.Ahead = &ahead
				}
				if behind, err := strconv.Atoi(strings.TrimPrefix(parts[1], "-")); err == nil {
					status.Behind = &behind
				}
			}
		case strings.HasPrefix(line, "? "):
			status.UntrackedFiles++
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "), strings.HasPrefix(line, "u "):
			status.ChangedFiles++
		}
	}
}

func parseWorktreeList(output string) []listEntry {
	var entries []listEntry
	var current *listEntry
	flush := func() {
		if current != nil {
			entries = append(entries, *current)
			current = nil
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if line == "" {
			flush()
			continue
		}
		key, value, _ := strings.Cut(line, " ")
		if key == "worktree" {
			flush()
			current = &listEntry{path: filepath.Clean(value)}
			continue
		}
		if current == nil {
			continue
		}
		switch key {
		case "HEAD":
			current.head = value
		case "branch":
			current.branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			current.bare = true
		case "detached":
			current.detached = true
		case "locked":
			current.locked = true
			current.lockReason = value
		case "prunable":
			current.prunable = true
		}
	}
	flush()
	return entries
}

// resolveBaseRef picks the branch a worktree counts as "merged" into: origin's
// default branch if known.
func resolveBaseRef(repoArgs []string) string {
	originHead := Git(append(repoArgs, "symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD")...)
	if originHead.OK() && originHead.Output() != "" {
		return originHead.Output()
	}
	for _, candidate := range []string{"origin/main", "origin/master", "origin/develop", "main", "master", "develop"} {
		if Git(append(repoArgs, "rev-parse", "--verify", "-q", candidate+"^{commit}")...).OK() {
			return candidate
		}
	}
	return ""
}

// adminDirectories maps Key(worktree path) to `<common>/worktrees/<name>` using
// each admin dir's `gitdir` file.
func adminDirectories(commonDir string) map[string]string {
	root := filepath.Join(commonDir, "worktrees")
	result := map[string]string{}
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		adminDir := filepath.Join(root, entry.Name())
		gitdir, ok := readFirstLine(filepath.Join(adminDir, "gitdir"))
		if !ok || gitdir == "" {
			continue
		}
		worktreePath := filepath.Dir(Resolve(gitdir, adminDir))
		result[Key(worktreePath)] = adminDir
		result[Key(Real(worktreePath))] = adminDir
	}
	return result
}

// lastActivity is the last time git touched the worktree (checkout, commit, staging).
func lastActivity(adminDir, path string) *time.Time {
	if adminDir == "" {
		return ModTime(path)
	}
	var latest *time.Time
	for _, name := range []string{"HEAD", "index", filepath.Join("logs", "HEAD")} {
		if t := ModTime(filepath.Join(adminDir, name)); t != nil && (latest == nil || t.After(*latest)) {
			latest = t
		}
	}
	return latest
}

func ptr[T any](v T) *T { return &v }
