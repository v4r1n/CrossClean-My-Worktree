package app

import (
	"os/exec"
	"syscall"
)

const createNewConsole = 0x00000010

// newConsole gives the child its own console window.
func newConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
}
