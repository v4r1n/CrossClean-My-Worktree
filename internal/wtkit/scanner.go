package wtkit

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ScanResult is what a folder walk found.
type ScanResult struct {
	// CommonDirs are common git dirs of repositories that have linked worktrees.
	CommonDirs []string
	// OrphanWorktrees are worktree directories whose repository no longer exists.
	OrphanWorktrees []string
	DirsVisited     int
}

// skippedNames are folder names that never hold worktrees worth finding and are
// expensive to walk.
var skippedNames = map[string]bool{
	"node_modules": true, ".Trash": true, ".cache": true, ".npm": true, ".pnpm-store": true,
	".yarn": true, ".bun": true, ".cargo": true, ".rustup": true, ".gradle": true, ".m2": true,
	".cocoapods": true, ".android": true, ".docker": true, ".colima": true, ".orbstack": true,
	".vscode": true, ".cursor": true, ".nvm": true, ".pyenv": true, ".rbenv": true,
	"DerivedData": true, "Pods": true, ".build": true, ".venv": true, "venv": true,
	"__pycache__": true, ".tox": true, ".next": true, ".nuxt": true, ".turbo": true,
	".svelte-kit": true, ".terraform": true, ".expo": true, ".nuget": true,
	"$Recycle.Bin": true, "System Volume Information": true,
}

// skippedPaths are system locations that matter only when scanning a whole disk.
func skippedPaths() map[string]bool {
	home := Home()
	var paths []string
	switch runtime.GOOS {
	case "windows":
		for _, env := range []string{"SystemRoot", "ProgramFiles", "ProgramFiles(x86)", "ProgramData"} {
			if dir := os.Getenv(env); dir != "" {
				paths = append(paths, dir)
			}
		}
		local := filepath.Join(home, "AppData", "Local")
		paths = append(paths,
			filepath.Join(local, "Packages"), filepath.Join(local, "Microsoft"),
			filepath.Join(home, "AppData", "Roaming", "Microsoft"), filepath.Join(home, "go", "pkg"))
	case "darwin":
		paths = []string{
			"/System", "/Library", "/Applications", "/dev", "/Volumes", "/Network", "/cores",
			"/bin", "/sbin", "/usr", "/private/var", "/opt/homebrew", "/.Spotlight-V100",
			"/.fseventsd", "/.DocumentRevisions-V100", "/.vol",
			filepath.Join(home, "Library"), filepath.Join(home, ".local"),
			filepath.Join(home, "go", "pkg"), filepath.Join(home, ".kube"),
		}
	default:
		paths = []string{
			"/proc", "/sys", "/dev", "/run", "/snap", "/boot", "/bin", "/sbin", "/lib", "/lib32",
			"/lib64", "/usr", "/var/lib", "/var/cache", "/nix/store",
			filepath.Join(home, ".local"), filepath.Join(home, "go", "pkg"), filepath.Join(home, ".kube"),
		}
	}
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[Key(p)] = true
	}
	return set
}

// ScanRepos walks roots looking for repositories with worktrees.
//
// A repository's worktree list includes worktrees anywhere on disk (e.g. a temp
// folder), so finding the repository is enough; `.git` files under a root also
// lead back to repositories that live outside it. Symlinks and Windows
// junctions are never followed.
func ScanRepos(ctx context.Context, roots []string, progress func(dirs int, path string)) ScanResult {
	var result ScanResult
	seen := map[string]bool{}
	skipped := skippedPaths()
	addRepo := func(commonDir string) {
		real := Real(commonDir)
		if k := Key(real); !seen[k] {
			seen[k] = true
			result.CommonDirs = append(result.CommonDirs, real)
		}
	}

	for _, root := range roots {
		root = Real(root)
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return filepath.SkipAll
			}
			if err != nil {
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if !d.IsDir() {
				if name == ".git" && d.Type().IsRegular() {
					switch target, commonDir := resolveGitFile(path); target {
					case gitFileWorktree:
						addRepo(commonDir)
					case gitFileOrphan:
						result.OrphanWorktrees = append(result.OrphanWorktrees, filepath.Dir(path))
					}
				}
				return nil
			}

			result.DirsVisited++
			if progress != nil && result.DirsVisited%2000 == 0 {
				progress(result.DirsVisited, path)
			}
			if path == root {
				return nil
			}
			if name == ".git" {
				if hasLinkedWorktrees(path) {
					addRepo(path)
				}
				return filepath.SkipDir
			}
			if skippedNames[name] || strings.HasSuffix(name, ".app") || strings.HasSuffix(name, ".photoslibrary") ||
				skipped[Key(path)] || isOtherUsersLibrary(path) {
				return filepath.SkipDir
			}
			return nil
		})
	}
	return result
}

type gitFileTarget int

const (
	// gitFileOther is a submodule or something else we don't manage.
	gitFileOther gitFileTarget = iota
	gitFileWorktree
	gitFileOrphan
)

// resolveGitFile reads a worktree's `.git` file (`gitdir: <common>/worktrees/<name>`).
func resolveGitFile(gitFile string) (gitFileTarget, string) {
	line, ok := readFirstLine(gitFile)
	if !ok || !strings.HasPrefix(line, "gitdir:") {
		return gitFileOther, ""
	}
	worktreeDir := filepath.Dir(gitFile)
	adminDir := Resolve(strings.TrimSpace(strings.TrimPrefix(line, "gitdir:")), worktreeDir)

	// Linked worktrees point into `<common>/worktrees/`; submodules point into `modules/`.
	if filepath.Base(filepath.Dir(adminDir)) != "worktrees" {
		return gitFileOther, ""
	}
	if !IsDirectory(adminDir) {
		return gitFileOrphan, ""
	}
	commonDir := filepath.Dir(filepath.Dir(adminDir))
	if relative, ok := readFirstLine(filepath.Join(adminDir, "commondir")); ok && relative != "" {
		commonDir = Resolve(relative, adminDir)
	}
	if !IsDirectory(commonDir) {
		return gitFileOrphan, ""
	}
	return gitFileWorktree, commonDir
}

func hasLinkedWorktrees(gitDir string) bool {
	entries, err := os.ReadDir(filepath.Join(gitDir, "worktrees"))
	return err == nil && len(entries) > 0
}

// isOtherUsersLibrary matches /Users/<someone>/Library on macOS.
func isOtherUsersLibrary(path string) bool {
	return runtime.GOOS == "darwin" && filepath.Base(path) == "Library" &&
		filepath.Dir(filepath.Dir(path)) == "/Users"
}

// EntireComputerRoots are the roots for a whole-computer scan: every drive on
// Windows, `/` elsewhere.
func EntireComputerRoots() []string {
	if !isWindows {
		return []string{"/"}
	}
	var roots []string
	for letter := 'A'; letter <= 'Z'; letter++ {
		root := string(letter) + `:\`
		if IsDirectory(root) {
			roots = append(roots, root)
		}
	}
	return roots
}
