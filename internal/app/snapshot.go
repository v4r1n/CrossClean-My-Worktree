package app

import (
	"sort"
	"time"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

// Row is a worktree plus the derived fields the UI shows.
type Row struct {
	wtkit.Worktree
	Name        string        `json:"name"`
	Display     string        `json:"display"`
	Verdict     wtkit.Verdict `json:"verdict"`
	Badges      []wtkit.Badge `json:"badges"`
	Detail      string        `json:"detail"`
	BranchLabel string        `json:"branchLabel"`
	BytesFreed  int64         `json:"bytesFreed"`
}

// RepoSummary is one sidebar entry.
type RepoSummary struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
	Bytes int64  `json:"bytes"`
}

// Snapshot is everything the UI renders, sent as JSON.
type Snapshot struct {
	Target    Target         `json:"target"`
	Targets   []TargetOption `json:"targets"`
	Progress  wtkit.Progress `json:"progress"`
	Rows      []Row          `json:"rows"`
	Repos     []RepoSummary  `json:"repos"`
	Job       *DeleteJob     `json:"job,omitempty"`
	FreeSpace *int64         `json:"freeSpace,omitempty"`
	LastScan  *time.Time     `json:"lastScan,omitempty"`
	Platform  string         `json:"platform"`
}

// TargetOption is one entry of the scan-location menu.
type TargetOption struct {
	Target Target `json:"target"`
	Title  string `json:"title"`
}

// Snapshot copies the current state for the UI.
func (s *State) Snapshot(platform string) Snapshot {
	// Query the disk before locking: a slow drive must not stall the scan.
	free, haveFree := wtkit.AvailableBytes(wtkit.Home())
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := Snapshot{
		Target:   s.settings.Target(),
		Targets:  targetOptions(s.settings.Recent(), platform),
		Progress: s.progress,
		Rows:     make([]Row, 0, len(s.worktrees)),
		Repos:    summarize(s.worktrees),
		LastScan: s.lastScan,
		Platform: platform,
	}
	if s.job != nil {
		job := *s.job
		snap.Job = &job
	}
	if haveFree {
		snap.FreeSpace = &free
	}
	for _, wt := range s.worktrees {
		snap.Rows = append(snap.Rows, Row{
			Worktree:    wt,
			Name:        wt.Name(),
			Display:     wtkit.Abbreviate(wt.Path),
			Verdict:     wt.Verdict(),
			Badges:      wt.Badges(),
			Detail:      wt.Detail(),
			BranchLabel: wt.BranchLabel(),
			BytesFreed:  wt.BytesFreed(),
		})
	}
	return snap
}

func targetOptions(recent []string, platform string) []TargetOption {
	everything := "Entire computer"
	if platform == "darwin" {
		everything = "Entire Mac"
	}
	options := []TargetOption{
		{Target{Kind: "home"}, "Home folder"},
		{Target{Kind: "all"}, everything},
	}
	for _, path := range recent {
		options = append(options, TargetOption{Target{Kind: "folder", Path: path}, wtkit.Abbreviate(path)})
	}
	return options
}

func summarize(items []wtkit.Worktree) []RepoSummary {
	byID := map[string]*RepoSummary{}
	var order []string
	for _, wt := range items {
		summary, ok := byID[wt.RepoID]
		if !ok {
			summary = &RepoSummary{ID: wt.RepoID, Name: wt.RepoName}
			byID[wt.RepoID] = summary
			order = append(order, wt.RepoID)
		}
		summary.Count++
		if wt.Usage != nil {
			summary.Bytes += wt.Usage.Allocated
		}
	}
	out := make([]RepoSummary, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return out[i].Name < out[j].Name
	})
	return out
}
