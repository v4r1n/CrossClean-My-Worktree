//go:build !windows

package wtkit

import "os/exec"

func hideWindow(*exec.Cmd) {}
