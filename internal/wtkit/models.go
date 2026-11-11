// Package wtkit finds git worktrees, inspects their state, measures their disk
// usage and removes them. It has no UI; the CLI and the web app both build on it.
package wtkit

import (
	"encoding/json"
	"path/filepath"
	"time"
)

// Repository is a git repository that has at least one linked worktree.
type Repository struct {
	// CommonDir is the shared git dir (`<repo>/.git`, or the bare repo dir).
	CommonDir string `json:"commonDir"`
	Name      string `json:"name"`
	// MainPath is the main working tree; empty for bare repositories.
	MainPath string `json:"mainPath,omitempty"`
	// BaseRef is the branch used to decide "merged", e.g. `origin/develop`.
	BaseRef    string `json:"baseRef,omitempty"`
	HasRemotes bool   `json:"hasRemotes"`
}

// Status is what `git status` and friends report for one worktree.
type Status struct {
	ChangedFiles   int    `json:"changedFiles"`
	UntrackedFiles int    `json:"untrackedFiles"`
	Upstream       string `json:"upstream,omitempty"`
	Ahead          *int   `json:"ahead,omitempty"`
	Behind         *int   `json:"behind,omitempty"`
	// Unpushed counts commits reachable from HEAD that are on no remote branch;
	// nil when the repository has no remotes.
	Unpushed *int   `json:"unpushed,omitempty"`
	Merged   *bool  `json:"merged,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Uncommitted is the number of modified plus untracked files.
func (s Status) Uncommitted() int { return s.ChangedFiles + s.UntrackedFiles }

// Usage is the disk usage of a directory tree.
type Usage struct {
	// Allocated is bytes allocated on disk (like `du`).
	Allocated int64 `json:"allocated"`
	// Reclaimable is bytes that deleting would actually free: it leaves out files
	// hard-linked from outside the tree (e.g. a pnpm store).
	Reclaimable int64 `json:"reclaimable"`
	Files       int   `json:"files"`
}

// Add returns the sum of two usages.
func (u Usage) Add(other Usage) Usage {
	return Usage{
		Allocated:   u.Allocated + other.Allocated,
		Reclaimable: u.Reclaimable + other.Reclaimable,
		Files:       u.Files + other.Files,
	}
}

// Verdict says how safe a worktree is to delete.
type Verdict int

const (
	Safe Verdict = iota
	Review
	Risky
	Pending
)

var verdictNames = [...]string{"safe", "review", "risky", "pending"}

func (v Verdict) String() string { return verdictNames[v] }

// MarshalJSON encodes the verdict as its name.
func (v Verdict) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }

// OrphanRepoID is the RepoID of worktrees whose repository no longer exists.
const OrphanRepoID = "orphan:"

// Worktree is one linked worktree (never a main checkout).
type Worktree struct {
	Path string `json:"path"`
	// RepoID is the CommonDir of the owning repository, or OrphanRepoID.
	RepoID     string `json:"repoId"`
	RepoName   string `json:"repoName"`
	Head       string `json:"head,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Detached   bool   `json:"detached"`
	Locked     bool   `json:"locked"`
	LockReason string `json:"lockReason,omitempty"`
	// Prunable: registered with git but the directory no longer exists.
	Prunable bool `json:"prunable"`
	// Orphan: has a `.git` file but the repository it points to is gone.
	Orphan bool `json:"orphan"`
	// AdminDir is `<common>/worktrees/<name>`.
	AdminDir     string     `json:"adminDir,omitempty"`
	LastActivity *time.Time `json:"lastActivity,omitempty"`
	Status       *Status    `json:"status,omitempty"`
	Usage        *Usage     `json:"usage,omitempty"`
	// InUse: some process has its working directory inside this worktree.
	InUse bool `json:"inUse"`
}

// Name is the worktree folder's name.
func (w Worktree) Name() string { return filepath.Base(w.Path) }

// BytesFreed is the best estimate of the space a delete returns.
func (w Worktree) BytesFreed() int64 {
	if w.Usage == nil {
		return 0
	}
	return w.Usage.Reclaimable
}

// Verdict classifies the worktree as safe, needing review, or risky to delete.
func (w Worktree) Verdict() Verdict {
	switch {
	case w.Prunable:
		return Safe
	case w.InUse:
		return Risky
	case w.Orphan:
		return Review
	case w.Status == nil:
		return Pending
	}
	s := w.Status
	isMerged := s.Merged != nil && *s.Merged
	switch {
	case s.Error != "":
		return Review
	case s.Uncommitted() > 0:
		return Risky
	case w.Locked:
		return Review
	case isMerged || (s.Unpushed != nil && *s.Unpushed == 0):
		return Safe
	case s.Unpushed != nil && *s.Unpushed > 0:
		return Risky
	case s.Unpushed == nil && s.Merged != nil && !*s.Merged:
		// No remote to compare against and the commits aren't merged: local-only work.
		return Risky
	}
	return Review
}
