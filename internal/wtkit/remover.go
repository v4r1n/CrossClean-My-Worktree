package wtkit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Remove permanently removes a linked worktree and git's record of it.
func Remove(wt Worktree) error {
	if wt.Prunable {
		// Same as `git worktree prune`, limited to this one entry.
		if wt.AdminDir != "" && IsDirectory(wt.AdminDir) {
			return removeAll(wt.AdminDir)
		}
		if wt.RepoID != OrphanRepoID {
			Git(append(RepoArgs(wt.RepoID), "worktree", "prune")...)
		}
		return nil
	}

	// Guard against ever deleting a main checkout: linked worktrees have a `.git` *file*.
	if !IsRegularFile(filepath.Join(wt.Path, ".git")) {
		return fmt.Errorf("refusing to delete %s: it has no .git file, so it doesn't look like a linked worktree", wt.Path)
	}

	if wt.Orphan {
		return removeAll(wt.Path)
	}

	args := append(RepoArgs(wt.RepoID), "worktree", "remove", "--force")
	if wt.Locked {
		args = append(args, "--force")
	}
	result := Git(append(args, wt.Path)...)
	if result.OK() {
		return nil
	}

	// git refuses some cases (e.g. worktrees containing submodules); finish the job by hand.
	var problems []string
	if msg := result.ErrorOutput(); msg != "" {
		problems = append(problems, msg)
	}
	if Exists(wt.Path) {
		if err := removeAll(wt.Path); err != nil {
			return errors.New(strings.Join(append(problems, err.Error()), "\n"))
		}
	}
	if wt.AdminDir != "" && Exists(wt.AdminDir) {
		if err := removeAll(wt.AdminDir); err != nil {
			return errors.New(strings.Join(append(problems, err.Error()), "\n"))
		}
	}
	return nil
}

// DeleteBranch runs `git branch -d`, which refuses branches with unmerged commits.
func DeleteBranch(branch, repoID string) Result {
	return Git(append(RepoArgs(repoID), "branch", "-d", branch)...)
}

// removeAll is os.RemoveAll that also copes with read-only files on Windows.
// Neither pass follows symlinks or junctions, so nothing outside path is touched.
func removeAll(path string) error {
	err := os.RemoveAll(path)
	if err == nil || !isWindows {
		return err
	}
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil && (d.IsDir() || d.Type().IsRegular()) {
			_ = os.Chmod(p, 0o777)
		}
		return nil
	})
	return os.RemoveAll(path)
}
