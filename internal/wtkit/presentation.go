package wtkit

import (
	"fmt"
	"strings"
)

// Tone colours a badge.
type Tone string

const (
	ToneGood    Tone = "good"
	ToneInfo    Tone = "info"
	ToneWarn    Tone = "warn"
	ToneBad     Tone = "bad"
	ToneNeutral Tone = "neutral"
)

// Badge is a short status label shown next to a worktree.
type Badge struct {
	Text string `json:"text"`
	Tone Tone   `json:"tone"`
}

// Badges summarise the worktree's state for display.
func (w Worktree) Badges() []Badge {
	if w.Prunable {
		return []Badge{{"Folder already gone", ToneInfo}}
	}
	var out []Badge
	if w.InUse {
		out = append(out, Badge{"In use", ToneBad})
	}
	if w.Orphan {
		out = append(out, Badge{"Repo not found", ToneWarn})
	}
	if w.Locked {
		out = append(out, Badge{"Locked", ToneWarn})
	}
	s := w.Status
	if s == nil {
		return append(out, Badge{"Checking…", ToneNeutral})
	}
	isMerged := s.Merged != nil && *s.Merged
	if s.Error != "" && !w.Orphan {
		out = append(out, Badge{"Git error", ToneWarn})
	}
	if n := s.Uncommitted(); n > 0 {
		out = append(out, Badge{fmt.Sprintf("%d uncommitted", n), ToneBad})
	}
	if s.Unpushed != nil && *s.Unpushed > 0 && !isMerged {
		out = append(out, Badge{fmt.Sprintf("%d unpushed", *s.Unpushed), ToneBad})
	}
	if isMerged {
		out = append(out, Badge{"Merged", ToneGood})
	} else if s.Unpushed != nil && *s.Unpushed == 0 {
		out = append(out, Badge{"Pushed", ToneGood})
	}
	if len(out) == 0 {
		out = append(out, Badge{"Clean", ToneNeutral})
	}
	return out
}

// Detail is a multi-line explanation of the worktree's state, for tooltips.
func (w Worktree) Detail() string {
	lines := []string{w.Path}
	if w.Branch != "" {
		lines = append(lines, "Branch: "+w.Branch)
	} else if w.Detached {
		lines = append(lines, "Detached HEAD at "+shortHash(w.Head, 8))
	}
	if w.Prunable {
		lines = append(lines, "The folder no longer exists; deleting only cleans up git's record of it.")
	}
	if w.LockReason != "" {
		lines = append(lines, "Locked: "+w.LockReason)
	}
	if w.InUse {
		lines = append(lines, "A running process (terminal, editor, agent) is working inside this folder.")
	}
	if s := w.Status; s != nil {
		if s.Upstream != "" {
			lines = append(lines, fmt.Sprintf("Upstream: %s (ahead %d, behind %d)", s.Upstream, deref(s.Ahead), deref(s.Behind)))
		}
		if s.ChangedFiles > 0 {
			lines = append(lines, fmt.Sprintf("%d modified tracked files", s.ChangedFiles))
		}
		if s.UntrackedFiles > 0 {
			lines = append(lines, fmt.Sprintf("%d untracked files", s.UntrackedFiles))
		}
		if s.Unpushed != nil {
			lines = append(lines, fmt.Sprintf("%d commits not on any remote", *s.Unpushed))
		}
		if s.Merged != nil {
			lines = append(lines, map[bool]string{true: "Fully merged into the default branch", false: "Not merged into the default branch"}[*s.Merged])
		}
		if s.Error != "" {
			lines = append(lines, s.Error)
		}
	}
	if u := w.Usage; u != nil && u.Reclaimable < u.Allocated {
		lines = append(lines, FormatBytes(u.Allocated-u.Reclaimable)+" is hard-linked from elsewhere (e.g. a pnpm store) and won't be freed.")
	}
	return strings.Join(lines, "\n")
}

// BranchLabel is the branch name, or a description of a detached HEAD.
func (w Worktree) BranchLabel() string {
	switch {
	case w.Branch != "":
		return w.Branch
	case w.Detached:
		return "detached @ " + shortHash(w.Head, 7)
	}
	return "—"
}

// FormatBytes formats a byte count with decimal units, like Finder.
func FormatBytes(n int64) string {
	const unit = 1000
	if n < unit {
		if n == 1 {
			return "1 byte"
		}
		return fmt.Sprintf("%d bytes", n)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	decimals := []int{0, 1, 2, 2, 2}
	value := float64(n)
	i := -1
	for value >= unit && i < len(units)-1 {
		value /= unit
		i++
	}
	return fmt.Sprintf("%.*f %s", decimals[i], value, units[i])
}

func shortHash(hash string, n int) string {
	if hash == "" {
		return "?"
	}
	if len(hash) > n {
		return hash[:n]
	}
	return hash
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
