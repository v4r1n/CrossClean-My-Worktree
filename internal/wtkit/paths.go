package wtkit

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const isWindows = runtime.GOOS == "windows"

// Home is the current user's home folder.
func Home() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return string(filepath.Separator)
}

// Real returns the absolute path with symlinks resolved, or the cleaned
// absolute path when it can't be resolved (e.g. it no longer exists).
func Real(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// Key normalises a path for comparison: cleaned, and lower-cased on Windows,
// whose file systems are case-insensitive.
func Key(path string) string {
	cleaned := filepath.Clean(path)
	if isWindows {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

// IsWithin reports whether child is parent or lies inside it.
func IsWithin(child, parent string) bool {
	c, p := Key(child), Key(parent)
	if c == p {
		return true
	}
	if !strings.HasSuffix(p, string(filepath.Separator)) {
		p += string(filepath.Separator)
	}
	return strings.HasPrefix(c, p)
}

// Abbreviate shortens paths inside the home folder to `~/...`.
func Abbreviate(path string) string {
	home := Home()
	if Key(path) == Key(home) {
		return "~"
	}
	if IsWithin(path, home) {
		return "~" + path[len(filepath.Clean(home)):]
	}
	return path
}

// ExpandTilde expands a leading `~` to the home folder.
func ExpandTilde(path string) string {
	if path == "~" {
		return Home()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		return filepath.Join(Home(), path[2:])
	}
	return path
}

// Resolve resolves path relative to base when it isn't absolute.
func Resolve(path, base string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return filepath.Clean(path)
}

// Exists reports whether anything exists at path (without following a final symlink).
func Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// IsDirectory reports whether path is a directory (following symlinks).
func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// IsRegularFile reports whether path is a regular file (not following symlinks).
func IsRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// ModTime is the modification time of path, or nil when it can't be read.
func ModTime(path string) *time.Time {
	info, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	t := info.ModTime()
	return &t
}

// readFirstLine returns the first line of a small text file, trimmed.
func readFirstLine(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(line), true
}
