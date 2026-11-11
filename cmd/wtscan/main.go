// Command wtscan lists git worktrees under some folders, with status and disk usage.
//
//	wtscan [--no-size] [folder ...]
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

const usage = `usage: wtscan [--no-size] [folder ...]
Lists git worktrees under the folders (default: your home folder), with status and disk usage.

  --no-size   skip disk usage (much faster)`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole program; it returns the exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	// Flags may appear anywhere, e.g. `wtscan ~/code --no-size`, which the flag
	// package would treat as a folder.
	measureSizes := true
	var roots []string
	for _, arg := range args {
		switch arg {
		case "-h", "--help", "-help":
			fmt.Fprintln(stdout, usage)
			return 0
		case "--no-size", "-no-size":
			measureSizes = false
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(stderr, "wtscan: unknown option %s\n\n%s\n", arg, usage)
				return 2
			}
			roots = append(roots, wtkit.Real(wtkit.ExpandTilde(arg)))
		}
	}
	if len(roots) == 0 {
		roots = []string{wtkit.Home()}
	}

	if !wtkit.GitAvailable() {
		fmt.Fprintln(stderr, "wtscan: git was not found. Install git and make sure it is on PATH.")
		return 1
	}

	report := wtkit.FullScan(ctx, wtkit.Options{Roots: roots, MeasureSizes: measureSizes}, wtkit.Events{})
	fmt.Fprintf(stderr, "Scanned %d folders in %.1fs, %d repos with worktrees\n",
		report.DirsVisited, report.WalkTime.Seconds(), len(report.Repos))

	printReport(stdout, report.Worktrees)
	return 0
}

func printReport(w io.Writer, rows []wtkit.Worktree) {
	var total, reclaimable, safeBytes int64
	safe := 0
	for _, wt := range rows {
		fmt.Fprintf(w, "%-7s %-24s %-9s %-9s %-36s %-30s %s\n",
			strings.ToUpper(wt.Verdict().String()), clip(wt.RepoName, 24),
			sizeOrDash(wt.Usage, false), sizeOrDash(wt.Usage, true),
			clip(wt.BranchLabel(), 36), clip(strings.Join(notes(wt), ", "), 30), wtkit.Abbreviate(wt.Path))
		if wt.Usage != nil {
			total += wt.Usage.Allocated
		}
		reclaimable += wt.BytesFreed()
		if wt.Verdict() == wtkit.Safe {
			safe++
			safeBytes += wt.BytesFreed()
		}
	}
	fmt.Fprintf(w, "\n%d worktrees · %s on disk · %s reclaimable · %d safe to delete (%s)\n",
		len(rows), wtkit.FormatBytes(total), wtkit.FormatBytes(reclaimable), safe, wtkit.FormatBytes(safeBytes))
}

func notes(wt wtkit.Worktree) []string {
	var out []string
	if wt.Prunable {
		out = append(out, "missing dir")
	}
	if wt.Orphan {
		out = append(out, "orphan")
	}
	if wt.InUse {
		out = append(out, "in use")
	}
	if wt.Locked {
		out = append(out, "locked")
	}
	if s := wt.Status; s != nil {
		if n := s.Uncommitted(); n > 0 {
			out = append(out, fmt.Sprintf("%d uncommitted", n))
		}
		if s.Unpushed != nil && *s.Unpushed > 0 {
			out = append(out, fmt.Sprintf("%d unpushed", *s.Unpushed))
		}
		if s.Merged != nil && *s.Merged {
			out = append(out, "merged")
		}
		if s.Error != "" && !wt.Orphan {
			out = append(out, "error: "+clip(s.Error, 60))
		}
	}
	return out
}

func sizeOrDash(u *wtkit.Usage, reclaimable bool) string {
	if u == nil {
		return "-"
	}
	if reclaimable {
		return wtkit.FormatBytes(u.Reclaimable)
	}
	return wtkit.FormatBytes(u.Allocated)
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
