# Clean My Worktree

**Find and remove forgotten git worktrees on your Mac, and get your disk space back.**

![macOS 14+](https://img.shields.io/badge/macOS-14%2B-blue)
![Swift 5.10+](https://img.shields.io/badge/Swift-5.10%2B-orange)
![License: MIT](https://img.shields.io/badge/license-MIT-green)
[![Latest release](https://img.shields.io/github/v/release/poomsc/Clean-My-Worktree)](https://github.com/poomsc/Clean-My-Worktree/releases/latest)

![Clean My Worktree: every worktree on your Mac in one window, with repository, branch, status, last activity and disk usage](docs/promo.png)

If you use `git worktree`, or AI coding agents that create a worktree for every task, they pile up quickly. Each one is a full checkout, often with its own `node_modules` or build output, and it's easy to forget them in `.worktrees/`, `.claude/worktrees/`, `/tmp` or wherever the last session left them.

Clean My Worktree (CleanMyWT) scans your Mac and lists every worktree in one window. You can see which repo each one belongs to, whether it still holds unsaved work, and how much space deleting it gives back. Then you delete them all at once or pick the ones you want.

## Features

- **Finds worktrees anywhere.** Scan your home folder, the entire Mac or any folder you choose. Worktrees outside that folder (such as `/tmp`) still show up, because every repository found is asked for its full worktree list.
- **Shows what's at stake.** Each worktree gets a verdict based on uncommitted files, unpushed commits, whether its branch is merged, and whether a running process (terminal, editor, agent) is using it.
- **Reports real disk usage.** The **Size** column is what's on disk. The **Frees** column is what deleting gives back; files hard-linked from elsewhere, such as a pnpm store, are left out.
- **Groups by repository.** The sidebar shows each repo with its number of worktrees and total size.
- **Deletes in bulk, carefully.** Mark worktrees with checkboxes (shift-click selects a range) or mark everything that's safe in one step. A confirmation sheet lists anything risky before you delete. Local branches can optionally be deleted too, but only if they're fully merged.
- **Cleans up stale records.** It finds worktrees whose folder is already gone and worktrees whose repository was deleted.
- **Includes a command-line tool.** `wtscan` prints the same report in the terminal.

## Requirements

- **macOS 14 Sonoma or later**, on Apple silicon or Intel.
- **git.** If you already use git on this Mac, you have it. If not, run `xcode-select --install` in Terminal (or `brew install git`). Without git, macOS offers to install it the first time the app scans.

## Install

### Download (recommended)

1. Download **CleanMyWT-x.y.z.zip** from the [latest release](https://github.com/poomsc/Clean-My-Worktree/releases/latest).
2. Unzip it, then drag **CleanMyWT.app** into your **Applications** folder.
3. Open the app. The first time, macOS blocks it, because it isn't notarized by Apple (notarizing needs a paid Apple developer account). Allow it once:
   - **macOS 15 Sequoia or later:** in the "not opened" dialog, click **Done**. Then open **System Settings → Privacy & Security**, scroll down to the message about CleanMyWT, click **Open Anyway** and confirm with your password or Touch ID.
   - **macOS 14 Sonoma:** right-click (or Control-click) the app, choose **Open**, then click **Open** again.
   - **Or, in Terminal:** `xattr -dr com.apple.quarantine /Applications/CleanMyWT.app`

   After that it opens normally. If you'd rather not run an unnotarized download, [build it from source](#build-from-source) instead.

To check the download, compare `shasum -a 256 CleanMyWT-x.y.z.zip` with the value in `SHA256SUMS.txt` on the release page.

### Command-line tool (optional)

Download **wtscan-x.y.z.zip** from the same release, then:

```bash
unzip wtscan-*.zip
xattr -d com.apple.quarantine wtscan
sudo mkdir -p /usr/local/bin && sudo mv wtscan /usr/local/bin/
```

### Build from source

You need the Xcode Command Line Tools (`xcode-select --install`); full Xcode isn't required. A build takes about a minute.

```bash
git clone https://github.com/poomsc/Clean-My-Worktree.git
cd Clean-My-Worktree
./scripts/build-app.sh --install   # builds the app and copies it to /Applications
```

The script also creates `build/wtscan`. Other options: `--universal` builds for both Apple silicon and Intel, and `--zip` writes release archives to `dist/`. An app you build yourself opens without the Gatekeeper prompt.

### First launch

- The app scans your home folder straight away.
- macOS asks whether CleanMyWT may access folders such as **Documents**, **Desktop** and **Downloads**. Allow it so the app can find worktrees there. If you click **Don't Allow**, those folders are skipped; you can change this later in **System Settings → Privacy & Security → Files & Folders**.

### Uninstall

Drag **CleanMyWT.app** to the Trash. To also remove its saved settings (scan location and recent folders), run `defaults delete app.cleanmywt.CleanMyWT`.

## Usage

1. Open CleanMyWT. It scans your home folder automatically.
2. To scan somewhere else, use the folder button in the toolbar: home folder, entire Mac, a recent folder, or **Choose Folder…** (⌘O). Press ⌘R to rescan.
3. Review the list. Filter it with **Safe to delete** / **Needs review**, search by name, repo or branch, or sort by size.
4. Mark the worktrees you want to remove, then click **Delete Marked…** (⌘⌫).

### What the verdicts mean

| Verdict | When | What to do |
| --- | --- | --- |
| ✅ **Safe** | No uncommitted or untracked files, and every commit is merged into the default branch or pushed to a remote. Also any worktree whose folder is already gone. | Delete freely. |
| ❓ **Review** | Locked, its repository was deleted, or git couldn't read it. | Look before deleting. |
| ⚠️ **Risky** | Has uncommitted or untracked files, commits that exist only locally, or a process is working inside it. | Deleting loses that work or breaks a running session. |

Hover over the status badges for details such as upstream, ahead/behind counts and file counts.

### Marking shortcuts

- **Click** a checkbox to mark one worktree.
- **Shift-click** a second checkbox to mark or unmark every row between the two.
- **Highlight rows** with Shift-click or ⌘-click, then press **Space** to mark or unmark them.
- **Mark menu → Safe to Delete** marks every safe worktree currently shown.
- **Right-click** a row to reveal it in Finder, open it in Terminal or copy its path.

### "Folder already gone"

Git keeps a record of each worktree in `<repo>/.git/worktrees/<name>`. If the folder is deleted some other way (`rm -rf`, the Finder, or macOS clearing `/tmp` on restart), that record stays behind. It takes almost no space, but the branch still counts as checked out, so `git checkout <branch>` elsewhere fails with *"already used by worktree"*. Deleting one of these only removes the stale record, which is the same as `git worktree prune` for that one entry.

## Command line

```bash
wtscan                     # scan your home folder
wtscan ~/code ~/work       # scan specific folders
wtscan --no-size           # skip disk usage (much faster)
```

(If you built from source and didn't install it, run `build/wtscan` instead.)

```
SAFE    my-api        388.6 MB  388.6 MB  feat/new-endpoint     merged          ~/code/.worktrees/new-endpoint
RISKY   my-web        2.55 GB   2.55 GB   feat/redesign         2 uncommitted   ~/code/my-web/.claude/worktrees/redesign
...
88 worktrees · 19.84 GB on disk · 19.84 GB reclaimable · 82 safe to delete (17.08 GB)
```

## How it works

1. **Find repositories.** It walks the chosen folders with `fts(3)` and looks for `.git` entries. A `.git` *directory* with a `worktrees/` subfolder is a repository with worktrees. A `.git` *file* is a linked worktree, and its `gitdir:` line leads back to the repository even when that repository is outside the scanned folder. Heavy folders that never contain worktrees (`node_modules`, `~/Library`, package-manager caches, build output) are skipped, so a typical home folder scans in a few seconds.
2. **List worktrees.** It runs `git worktree list --porcelain` for each repository. The main checkout is never listed.
3. **Check status.** For each worktree it runs `git status --porcelain=v2 --branch`, counts commits that are on no remote (`git rev-list HEAD --not --remotes`), and checks whether HEAD is merged into the default branch (`origin/HEAD`, falling back to `main`, `master` or `develop`). `lsof` finds processes whose working directory is inside a worktree.
4. **Measure.** It adds up allocated blocks for every file. A hard-linked file counts only once, and counts as reclaimable only when all of its links are inside the worktree.

### Safety

- **Scanning is read-only.** Git runs with `GIT_OPTIONAL_LOCKS=0`, so a scan never takes `index.lock` in a worktree that another tool is using.
- **Deleting uses `git worktree remove --force`.** If git refuses (for example, because the worktree contains submodules), the app removes the folder and its record directly instead.
- **Main checkouts can't be deleted.** A folder is deleted only if it has a `.git` *file*, which marks a linked worktree.
- **Deletion is permanent.** Folders don't go to the Trash, because the point is to free space. Commits and branches stay in the repository.

## Limitations

- **APFS clones:** files created as clones (for example with `cp -c`, or by some package managers) share blocks with other files, so **Frees** can overstate what you'll get back.
- **Squash merges:** "Merged" means HEAD is an ancestor of the default branch, so squash-merged branches show as not merged. They still count as safe if they were pushed.
- **Protected folders:** folders protected by macOS privacy settings are skipped unless you give the app Full Disk Access (System Settings → Privacy & Security).

## Project structure

```
Sources/
  WorktreeKit/   scanning, git inspection, disk usage and removal (no UI)
  CleanMyWT/     the SwiftUI app
  wtscan/        the command-line tool
scripts/
  build-app.sh   builds and bundles the .app (and release zips)
  make-icon.swift  renders the app icon
Resources/       Info.plist and the app icon
.github/workflows/build.yml  CI build; pushing a v* tag publishes a release
```

For a debug build, run `swift build`, then `.build/debug/CleanMyWT` or `.build/debug/wtscan`.

## Contributing

Issues and pull requests are welcome. Keep the core logic in `WorktreeKit`, so the app and the CLI stay in sync, and test anything that deletes files against a throwaway repository first.

## License

[MIT](LICENSE)
