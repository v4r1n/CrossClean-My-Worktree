//go:build unix && !linux

package wtkit

import (
	"os/exec"
	"strings"
)

// ProcessWorkingDirectories returns the working directories of the processes
// this user can read, using lsof.
func ProcessWorkingDirectories() []string {
	lsof, err := exec.LookPath("lsof")
	if err != nil {
		lsof = "/usr/sbin/lsof"
	}
	var dirs []string
	for _, line := range Run(lsof, []string{"-nP", "-d", "cwd", "-Fn"}, "", nil).Lines() {
		if strings.HasPrefix(line, "n/") {
			dirs = append(dirs, line[1:])
		}
	}
	return dirs
}
