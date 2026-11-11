# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0] - 2026-11-11

Cross-platform rewrite of [poomsc/Clean-My-Worktree](https://github.com/poomsc/Clean-My-Worktree) 1.0.0 in Go.
Upgrading from the macOS app? See [MIGRATING.md](MIGRATING.md).

### Added

- Windows support (x64 and Arm), alongside macOS and Linux.
- `cleanmywt`: a single binary that serves the app on `127.0.0.1` and opens it in the default browser.
  Flags: `--port`, `--no-browser`.
- "Entire computer" scan target, which walks every drive on Windows.
- "In use" detection on Windows (reads each process's working directory) and on Linux (`/proc`).
- Windows-aware scanning and deletion: case-insensitive path matching, junctions never followed,
  read-only files removed, paths longer than 260 characters measured.
- Right-click / `⋯` row menu: show in Explorer, Finder or the file manager; open a terminal; copy the path.
- Per-launch API token and Host allow-list, so other websites and programs can't drive the local server.
- `wtscan` accepts `--no-size` anywhere on the command line and rejects unknown options with exit code 2.
- Release zips for windows, darwin and linux on amd64 and arm64, with `SHA256SUMS.txt`
  (`scripts/dist.sh`).
- CI that runs `go vet` and `go test -race` on Windows, macOS and Linux.
- Test suite that builds real throwaway repositories: verdicts, removal, main-checkout guard,
  hard links, read-only files, long paths, in-use detection and API auth.

### Changed

- The UI is now a browser page instead of a native SwiftUI window.
- Choosing a folder to scan is a text field (with recent folders in the menu) instead of a native picker.
- Settings are stored in `settings.json` in the user config folder
  (`%AppData%\CleanMyWT`, `~/Library/Application Support/CleanMyWT`, `~/.config/CleanMyWT`)
  instead of macOS `defaults`.
- The "Repo deleted" badge is now "Repo not found", since the repository may be on a drive that isn't connected.
- "Disk shows N more free space" after a delete is measured on the drive the worktrees were on,
  and is skipped when they span several drives.
- Go module path is `github.com/v4r1n/CrossClean-My-Worktree`.

### Removed

- The native macOS `.app` bundle, its icon and `scripts/build-app.sh`.
- Swift sources and `Package.swift`.

### Security

- Opening a terminal never passes the worktree path through a shell, so folder names containing
  `&`, `^` or `%` can't run commands.

## [1.0.0] - 2026-11-10

Original macOS release by [poomsc](https://github.com/poomsc/Clean-My-Worktree/releases/tag/v1.0.0)
(SwiftUI app and `wtscan` CLI). Not part of this repository's own releases.

[Unreleased]: https://github.com/v4r1n/CrossClean-My-Worktree/compare/v2.0.0...HEAD
[2.0.0]: https://github.com/v4r1n/CrossClean-My-Worktree/commits/main
[1.0.0]: https://github.com/poomsc/Clean-My-Worktree/releases/tag/v1.0.0
