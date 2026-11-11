package app

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/v4r1n/CrossClean-My-Worktree/internal/wtkit"
)

// OpenURL opens url in the default browser.
func OpenURL(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}

// Reveal shows path in the file manager, selected when the platform allows it.
// A folder that no longer exists reveals its parent instead.
func Reveal(path string) error {
	if !wtkit.Exists(path) {
		path = filepath.Dir(path)
	}
	switch runtime.GOOS {
	case "windows":
		// explorer.exe returns 1 even on success, so don't wait for it.
		return exec.Command("explorer", "/select,", path).Start()
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	}
	return exec.Command("xdg-open", filepath.Dir(path)).Start()
}

// OpenTerminal opens a terminal window in path.
func OpenTerminal(path string) error {
	if !wtkit.IsDirectory(path) {
		return errors.New("the folder no longer exists")
	}
	switch runtime.GOOS {
	case "windows":
		// Windows Terminal treats ';' in its arguments as a command separator.
		if wt, err := exec.LookPath("wt.exe"); err == nil && !strings.Contains(path, ";") {
			return exec.Command(wt, "-d", path).Start()
		}
		// Start cmd.exe directly in path: no shell ever parses the path.
		cmd := exec.Command("cmd.exe")
		cmd.Dir = path
		newConsole(cmd)
		return cmd.Start()
	case "darwin":
		return exec.Command("open", "-a", "Terminal", path).Start()
	}
	for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xterm"} {
		if bin, err := exec.LookPath(term); err == nil {
			cmd := exec.Command(bin)
			cmd.Dir = path
			return cmd.Start()
		}
	}
	return errors.New("no terminal emulator found")
}
