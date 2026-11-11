package wtkit

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Phase is the stage a full scan is in.
type Phase string

const (
	PhaseIdle       Phase = "idle"
	PhaseScanning   Phase = "scanning"
	PhaseInspecting Phase = "inspecting"
	PhaseMeasuring  Phase = "measuring"
)

// Progress reports how far a full scan has got.
type Progress struct {
	Phase  Phase  `json:"phase"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
	Detail string `json:"detail,omitempty"`
}

// Events are callbacks a full scan makes as it goes. Calls are serialised, and
// any of them may be nil.
type Events struct {
	Progress func(Progress)
	// Listed delivers every worktree found, before status and sizes are known.
	Listed func(repos map[string]Repository, worktrees []Worktree)
	// Updated delivers a worktree whose status or usage just became known.
	Updated func(Worktree)
}

// Options control a full scan.
type Options struct {
	Roots          []string
	MeasureSizes   bool
	StatusWorkers  int
	MeasureWorkers int
}

// Report is the outcome of a full scan.
type Report struct {
	Repos       map[string]Repository
	Worktrees   []Worktree
	DirsVisited int
	WalkTime    time.Duration
}

// FullScan finds every worktree under the roots, then fills in git status and
// (optionally) disk usage. It returns early, with partial results, when ctx is
// cancelled.
func FullScan(ctx context.Context, opts Options, ev Events) Report {
	var mu sync.Mutex
	emit := func(f func()) { mu.Lock(); defer mu.Unlock(); f() }
	progress := func(p Progress) {
		if ev.Progress != nil {
			emit(func() { ev.Progress(p) })
		}
	}

	// 1. Find repositories that have worktrees.
	progress(Progress{Phase: PhaseScanning, Detail: "Looking for repositories…"})
	started := time.Now()
	found := ScanRepos(ctx, opts.Roots, func(dirs int, path string) {
		progress(Progress{Phase: PhaseScanning, Done: dirs, Detail: fmt.Sprintf("%d folders · %s", dirs, Abbreviate(path))})
	})
	report := Report{Repos: map[string]Repository{}, DirsVisited: found.DirsVisited, WalkTime: time.Since(started)}
	if ctx.Err() != nil {
		return report
	}

	// 2. List each repository's worktrees, and which folders processes are using.
	progress(Progress{Phase: PhaseScanning, Detail: fmt.Sprintf("Reading %d repositories…", len(found.CommonDirs))})
	cwdsDone := make(chan []string, 1)
	go func() {
		var dirs []string
		for _, dir := range ProcessWorkingDirectories() {
			dirs = append(dirs, Real(dir))
		}
		cwdsDone <- dirs
	}()
	var items []Worktree
	forEach(ctx, len(found.CommonDirs), 8, func(i int) {
		repo, worktrees, ok := LoadRepository(found.CommonDirs[i])
		if ok {
			emit(func() { report.Repos[repo.CommonDir] = repo; items = append(items, worktrees...) })
		}
	})
	for _, path := range found.OrphanWorktrees {
		items = append(items, OrphanWorktree(path))
	}
	cwds := <-cwdsDone
	if ctx.Err() != nil {
		return report
	}
	items = prepare(items, cwds)
	report.Worktrees = items
	if ev.Listed != nil {
		emit(func() { ev.Listed(report.Repos, append([]Worktree(nil), items...)) })
	}

	// 3. Git status for each worktree.
	needStatus := indicesWhere(items, func(wt Worktree) bool { return wt.Status == nil && !wt.Prunable })
	runStage(ctx, PhaseInspecting, needStatus, workers(opts.StatusWorkers, 8), items, &mu, ev, progress, func(wt *Worktree) {
		if repo, ok := report.Repos[wt.RepoID]; ok {
			status := InspectStatus(*wt, repo)
			wt.Status = &status
		}
	})
	if ctx.Err() != nil || !opts.MeasureSizes {
		return report
	}

	// 4. Disk usage, the slow part.
	needSize := indicesWhere(items, func(wt Worktree) bool { return !wt.Prunable })
	runStage(ctx, PhaseMeasuring, needSize, workers(opts.MeasureWorkers, 4), items, &mu, ev, progress, func(wt *Worktree) {
		usage := Measure(ctx, wt.Path)
		if wt.AdminDir != "" {
			usage = usage.Add(Measure(ctx, wt.AdminDir))
		}
		wt.Usage = &usage
	})
	return report
}

// prepare de-duplicates worktrees, marks the ones in use and sorts them.
func prepare(items []Worktree, cwds []string) []Worktree {
	seen := map[string]bool{}
	var unique []Worktree
	for _, wt := range items {
		real := Real(wt.Path)
		if seen[Key(real)] {
			continue
		}
		seen[Key(real)] = true
		for _, cwd := range cwds {
			if IsWithin(cwd, real) {
				wt.InUse = true
				break
			}
		}
		unique = append(unique, wt)
	}
	sort.SliceStable(unique, func(i, j int) bool {
		a, b := strings.ToLower(unique[i].RepoName), strings.ToLower(unique[j].RepoName)
		if a != b {
			return a < b
		}
		return unique[i].Path < unique[j].Path
	})
	return unique
}

// runStage applies work to items[i] for each index, reporting progress and updates.
func runStage(ctx context.Context, phase Phase, indices []int, limit int, items []Worktree,
	mu *sync.Mutex, ev Events, progress func(Progress), work func(*Worktree)) {
	done := 0
	progress(Progress{Phase: phase, Total: len(indices)})
	forEach(ctx, len(indices), limit, func(n int) {
		wt := items[indices[n]]
		work(&wt)
		if ctx.Err() != nil {
			return
		}
		mu.Lock()
		items[indices[n]] = wt
		done++
		current := done
		if ev.Updated != nil {
			ev.Updated(wt)
		}
		mu.Unlock()
		progress(Progress{Phase: phase, Done: current, Total: len(indices)})
	})
}

// forEach calls fn(0..n-1) on at most limit goroutines at once, stopping early
// when ctx is cancelled.
func forEach(ctx context.Context, n, limit int, fn func(int)) {
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i := 0; i < n && ctx.Err() == nil; i++ {
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer func() { <-sem; wg.Done() }()
			fn(i)
		}(i)
	}
	wg.Wait()
}

func indicesWhere(items []Worktree, keep func(Worktree) bool) []int {
	var out []int
	for i, wt := range items {
		if keep(wt) {
			out = append(out, i)
		}
	}
	return out
}

func workers(requested, fallback int) int {
	if requested > 0 {
		return requested
	}
	return fallback
}
