//go:build !windows

package app

import "os/exec"

func newConsole(*exec.Cmd) {}
