// Package app is the Clean My Worktree web app: a local HTTP server that runs
// scans and deletions with wtkit and serves a browser UI.
package app

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

// Target is what to scan.
type Target struct {
	Kind string `json:"kind"` // "home", "all" or "folder"
	Path string `json:"path,omitempty"`
}

// Roots are the folders a target walks.
func (t Target) Roots() []string {
	switch t.Kind {
	case "all":
		return wtkit.EntireComputerRoots()
	case "folder":
		return []string{t.Path}
	}
	return []string{wtkit.Home()}
}

// DeleteReport is the outcome of a delete job.
type DeleteReport struct {
	Removed         int       `json:"removed"`
	EstimatedFreed  int64     `json:"estimatedFreed"`
	MeasuredFreed   *int64    `json:"measuredFreed,omitempty"`
	Failures        []Failure `json:"failures"`
	BranchesDeleted int       `json:"branchesDeleted"`
	BranchesKept    []string  `json:"branchesKept"`
}

// Failure is one worktree that couldn't be deleted.
type Failure struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// DeleteJob tracks a running or finished delete.
type DeleteJob struct {
	ID      int           `json:"id"`
	Done    int           `json:"done"`
	Total   int           `json:"total"`
	Running bool          `json:"running"`
	Report  *DeleteReport `json:"report,omitempty"`
}

var (
	errBusy    = errors.New("a scan or delete is already running")
	errUnknown = errors.New("not a worktree from the current scan")
)

// State is the app's shared model. Every field is guarded by mu.
type State struct {
	mu        sync.Mutex
	settings  *Settings
	repos     map[string]wtkit.Repository
	worktrees []wtkit.Worktree
	progress  wtkit.Progress
	cancel    context.CancelFunc
	scanGen   int
	job       *DeleteJob
	lastScan  *time.Time
}

// NewState creates the model with saved settings.
func NewState(settings *Settings) *State {
	return &State{settings: settings, repos: map[string]wtkit.Repository{}, progress: wtkit.Progress{Phase: wtkit.PhaseIdle}}
}

func (s *State) scanning() bool { return s.progress.Phase != wtkit.PhaseIdle }

func (s *State) deleting() bool { return s.job != nil && s.job.Running }

// StartScan cancels any running scan and starts a new one.
func (s *State) StartScan(target Target) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleting() {
		return errBusy
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.settings.SetTarget(target)
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.scanGen++
	gen := s.scanGen
	s.repos = map[string]wtkit.Repository{}
	s.worktrees = nil
	s.progress = wtkit.Progress{Phase: wtkit.PhaseScanning, Detail: "Looking for repositories…"}

	go s.runScan(ctx, gen, target.Roots())
	return nil
}

func (s *State) runScan(ctx context.Context, gen int, roots []string) {
	// apply runs f only if this scan is still the current one.
	apply := func(f func()) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if gen == s.scanGen && ctx.Err() == nil {
			f()
		}
	}
	wtkit.FullScan(ctx, wtkit.Options{Roots: roots, MeasureSizes: true}, wtkit.Events{
		Progress: func(p wtkit.Progress) { apply(func() { s.progress = p }) },
		Listed: func(repos map[string]wtkit.Repository, items []wtkit.Worktree) {
			apply(func() { s.repos, s.worktrees = repos, items })
		},
		Updated: func(wt wtkit.Worktree) {
			apply(func() {
				if i := s.indexOf(wt.Path); i >= 0 {
					s.worktrees[i] = wt
				}
			})
		},
	})
	apply(func() {
		now := time.Now()
		s.lastScan = &now
		s.progress = wtkit.Progress{Phase: wtkit.PhaseIdle}
		s.cancel = nil
	})
}

// StopScan cancels the running scan, keeping what was found so far.
func (s *State) StopScan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.scanGen++
	s.progress = wtkit.Progress{Phase: wtkit.PhaseIdle}
}

func (s *State) indexOf(path string) int {
	for i, wt := range s.worktrees {
		if wt.Path == path {
			return i
		}
	}
	return -1
}

// Lookup returns the worktrees from the current scan with the given paths. Paths
// that aren't in the scan are an error, so the UI can never delete arbitrary folders.
func (s *State) Lookup(paths []string) ([]wtkit.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []wtkit.Worktree
	for _, path := range paths {
		i := s.indexOf(path)
		if i < 0 {
			return nil, errUnknown
		}
		out = append(out, s.worktrees[i])
	}
	return out, nil
}

// StartDelete removes the given worktrees in the background.
func (s *State) StartDelete(paths []string, deleteBranches bool) (int, error) {
	items, err := s.Lookup(paths)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleting() || s.scanning() {
		return 0, errBusy
	}
	id := 1
	if s.job != nil {
		id = s.job.ID + 1
	}
	s.job = &DeleteJob{ID: id, Total: len(items), Running: true}
	go s.runDelete(s.job, items, deleteBranches)
	return id, nil
}

func (s *State) runDelete(job *DeleteJob, items []wtkit.Worktree, deleteBranches bool) {
	report := &DeleteReport{Failures: []Failure{}, BranchesKept: []string{}}
	volume, sameVolume := sharedVolume(items)
	before, haveBefore := wtkit.AvailableBytes(volume)
	haveBefore = haveBefore && sameVolume
	var removed []wtkit.Worktree
	var mu sync.Mutex

	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, wt := range items {
		sem <- struct{}{}
		wg.Add(1)
		go func(wt wtkit.Worktree) {
			defer func() { <-sem; wg.Done() }()
			err := wtkit.Remove(wt)
			mu.Lock()
			if err != nil {
				report.Failures = append(report.Failures, Failure{Path: wt.Path, Message: err.Error()})
			} else {
				removed = append(removed, wt)
				report.EstimatedFreed += wt.BytesFreed()
			}
			mu.Unlock()
			s.mu.Lock()
			job.Done++
			if err == nil {
				if i := s.indexOf(wt.Path); i >= 0 {
					s.worktrees = append(s.worktrees[:i:i], s.worktrees[i+1:]...)
				}
			}
			s.mu.Unlock()
		}(wt)
	}
	wg.Wait()
	report.Removed = len(removed)

	if deleteBranches {
		for _, wt := range removed {
			if wt.Branch == "" || wt.Orphan {
				continue
			}
			if wtkit.DeleteBranch(wt.Branch, wt.RepoID).OK() {
				report.BranchesDeleted++
			} else {
				report.BranchesKept = append(report.BranchesKept, wt.Branch)
			}
		}
	}
	if after, ok := wtkit.AvailableBytes(volume); ok && haveBefore {
		freed := max(0, after-before)
		report.MeasuredFreed = &freed
	}
	sort.Slice(report.Failures, func(i, j int) bool { return report.Failures[i].Path < report.Failures[j].Path })

	s.mu.Lock()
	job.Report = report
	job.Running = false
	s.mu.Unlock()
}

// sharedVolume picks a folder on the drive the worktrees live on, so the
// before/after free-space reading measures the right disk. ok is false when
// the worktrees span several Windows drives; the reading is then skipped.
func sharedVolume(items []wtkit.Worktree) (string, bool) {
	if len(items) == 0 {
		return wtkit.Home(), false
	}
	volume := filepath.VolumeName(items[0].Path)
	for _, wt := range items[1:] {
		if !strings.EqualFold(filepath.VolumeName(wt.Path), volume) {
			return "", false
		}
	}
	// Free space is read at an ancestor that survives the delete.
	dir := filepath.Dir(items[0].Path)
	for !wtkit.IsDirectory(dir) && filepath.Dir(dir) != dir {
		dir = filepath.Dir(dir)
	}
	return dir, true
}
