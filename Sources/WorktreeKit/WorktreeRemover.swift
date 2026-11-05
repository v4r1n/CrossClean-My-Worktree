import Foundation

public struct RemovalError: LocalizedError, Sendable {
    public let message: String
    public var errorDescription: String? { message }
}

public enum WorktreeRemover {
    /// Permanently removes a linked worktree and its git bookkeeping.
    public static func remove(_ worktree: Worktree) throws {
        let fm = FileManager.default

        if worktree.isPrunable {
            // Same as `git worktree prune`, limited to this one entry.
            if let adminDir = worktree.adminDir, Paths.isDirectory(adminDir) {
                try fm.removeItem(atPath: adminDir)
            } else if worktree.repoID != Worktree.orphanRepoID {
                _ = Git.run(Git.repoArgs(worktree.repoID) + ["worktree", "prune"])
            }
            return
        }

        // Guard against ever deleting a main checkout: linked worktrees have a `.git` *file*.
        let dotGit = (worktree.path as NSString).appendingPathComponent(".git")
        guard Paths.isRegularFile(dotGit) else {
            throw RemovalError(message: "Refusing to delete \(worktree.path): it has no .git file, so it doesn't look like a linked worktree.")
        }

        if worktree.isOrphan {
            try fm.removeItem(atPath: worktree.path)
            return
        }

        var args = Git.repoArgs(worktree.repoID) + ["worktree", "remove", "--force"]
        if worktree.isLocked { args.append("--force") }
        args.append(worktree.path)
        let result = Git.run(args)
        if result.ok { return }

        // git refuses some cases (e.g. worktrees containing submodules); finish the job by hand.
        do {
            if Paths.exists(worktree.path) { try fm.removeItem(atPath: worktree.path) }
            if let adminDir = worktree.adminDir, Paths.exists(adminDir) { try fm.removeItem(atPath: adminDir) }
        } catch {
            throw RemovalError(message: [result.errorOutput, error.localizedDescription].filter { !$0.isEmpty }.joined(separator: "\n"))
        }
    }

    /// `git branch -d`: git refuses when the branch has commits that aren't merged.
    public static func deleteBranch(_ branch: String, repoID: String) -> CommandResult {
        Git.run(Git.repoArgs(repoID) + ["branch", "-d", branch])
    }
}
