package app

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

// Seams for tests: which OS to act as, how to find programs, and how to start
// them (tests record commands instead of opening windows).
var (
	goos         = runtime.GOOS
	lookPath     = exec.LookPath
	startCommand = func(cmd *exec.Cmd) error { return cmd.Start() }
)

// OpenURL opens url in the default browser.
func OpenURL(url string) error {
	switch goos {
	case "windows":
		return startCommand(exec.Command("rundll32", "url.dll,FileProtocolHandler", url))
	case "darwin":
		return startCommand(exec.Command("open", url))
	}
	return startCommand(exec.Command("xdg-open", url))
}

// Reveal shows path in the file manager, selected when the platform allows it.
// A folder that no longer exists reveals its parent instead.
func Reveal(path string) error {
	if !wtkit.Exists(path) {
		path = filepath.Dir(path)
	}
	switch goos {
	case "windows":
		// explorer.exe returns 1 even on success, so don't wait for it.
		return startCommand(exec.Command("explorer", "/select,", path))
	case "darwin":
		return startCommand(exec.Command("open", "-R", path))
	}
	return startCommand(exec.Command("xdg-open", filepath.Dir(path)))
}

// OpenTerminal opens a terminal window in path. The path is never parsed by a
// shell, so folder names with `&`, `^`, `%` or `;` can't run commands.
func OpenTerminal(path string) error {
	if !wtkit.IsDirectory(path) {
		return errors.New("the folder no longer exists")
	}
	switch goos {
	case "windows":
		// Windows Terminal treats ';' in its arguments as a command separator.
		if wt, err := lookPath("wt.exe"); err == nil && !strings.Contains(path, ";") {
			return startCommand(exec.Command(wt, "-d", path))
		}
		cmd := exec.Command("cmd.exe")
		cmd.Dir = path
		newConsole(cmd)
		return startCommand(cmd)
	case "darwin":
		return startCommand(exec.Command("open", "-a", "Terminal", path))
	}
	for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xterm"} {
		if bin, err := lookPath(term); err == nil {
			cmd := exec.Command(bin)
			cmd.Dir = path
			return startCommand(cmd)
		}
	}
	return errors.New("no terminal emulator found")
}
