package wtkit

import (
	"os"
	"path/filepath"
)

// ProcessWorkingDirectories returns the working directories of the processes
// this user can read, from /proc/<pid>/cwd.
func ProcessWorkingDirectories() []string {
	links, _ := filepath.Glob("/proc/[0-9]*/cwd")
	var dirs []string
	for _, link := range links {
		if dir, err := os.Readlink(link); err == nil {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}
