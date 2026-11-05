import Foundation

public enum WorktreeInspector {
    struct ListEntry {
        var path = ""
        var head: String?
        var branch: String?
        var isBare = false
        var isDetached = false
        var isLocked = false
        var lockReason: String?
        var isPrunable = false
    }

    /// Loads a repository and its linked worktrees (the main working tree is excluded).
    public static func load(commonDir: String) -> (Repository, [Worktree])? {
        let repoArgs = Git.repoArgs(commonDir)
        let list = Git.run(repoArgs + ["worktree", "list", "--porcelain"])
        guard list.ok else { return nil }
        let entries = parseWorktreeList(list.stdout)
        guard let main = entries.first else { return nil }

        let mainPath = main.isBare ? nil : main.path
        var name = ((mainPath ?? commonDir) as NSString).lastPathComponent
        if main.isBare, name.hasSuffix(".git") { name = String(name.dropLast(4)) }

        let hasRemotes = !Git.run(repoArgs + ["remote"]).output.isEmpty
        let repo = Repository(
            commonDir: commonDir,
            name: name,
            mainPath: mainPath,
            baseRef: resolveBaseRef(repoArgs),
            hasRemotes: hasRemotes
        )

        let adminDirs = adminDirectories(commonDir)
        let worktrees = entries.dropFirst().filter { !$0.isBare }.map { entry -> Worktree in
            var wt = Worktree(path: entry.path, repoID: commonDir, repoName: name)
            wt.head = entry.head
            wt.branch = entry.branch
            wt.isDetached = entry.isDetached
            wt.isLocked = entry.isLocked
            wt.lockReason = entry.lockReason
            wt.isPrunable = entry.isPrunable || !Paths.isDirectory(entry.path)
            wt.adminDir = adminDirs[entry.path] ?? adminDirs[Paths.real(entry.path)]
            wt.lastActivity = lastActivity(adminDir: wt.adminDir, path: entry.path)
            return wt
        }
        return (repo, worktrees)
    }

    /// A worktree whose repository has been deleted.
    public static func orphan(path: String) -> Worktree {
        var wt = Worktree(path: path, repoID: Worktree.orphanRepoID, repoName: "Main repo missing")
        wt.isOrphan = true
        wt.lastActivity = Paths.modificationDate(path)
        var status = WorktreeStatus()
        status.error = "The repository this worktree belongs to no longer exists."
        wt.status = status
        return wt
    }

    public static func status(of worktree: Worktree, in repo: Repository) -> WorktreeStatus {
        var status = WorktreeStatus()
        let result = Git.run(["-C", worktree.path, "status", "--porcelain=v2", "--branch"])
        guard result.ok else {
            status.error = result.errorOutput.isEmpty ? "git status failed" : result.errorOutput
            return status
        }

        for line in result.stdout.split(separator: "\n") {
            if line.hasPrefix("# branch.upstream ") {
                status.upstream = String(line.dropFirst("# branch.upstream ".count))
            } else if line.hasPrefix("# branch.ab ") {
                let parts = line.dropFirst("# branch.ab ".count).split(separator: " ")
                if parts.count == 2 {
                    status.ahead = Int(parts[0].dropFirst())
                    status.behind = Int(parts[1].dropFirst())
                }
            } else if line.hasPrefix("? ") {
                status.untrackedFiles += 1
            } else if line.hasPrefix("1 ") || line.hasPrefix("2 ") || line.hasPrefix("u ") {
                status.changedFiles += 1
            }
        }

        if repo.hasRemotes {
            let count = Git.run(["-C", worktree.path, "rev-list", "--count", "HEAD", "--not", "--remotes"])
            if count.ok { status.unpushedCommits = Int(count.output) }
        }
        if let base = repo.baseRef {
            let merged = Git.run(["-C", worktree.path, "merge-base", "--is-ancestor", "HEAD", base])
            if merged.status == 0 { status.mergedIntoBase = true }
            else if merged.status == 1 { status.mergedIntoBase = false }
        }
        return status
    }

    /// Paths of directories that processes are currently using as their working directory.
    public static func processWorkingDirectories() -> [String] {
        let result = Shell.run("/usr/sbin/lsof", ["-nP", "-d", "cwd", "-Fn"])
        return result.stdout.split(separator: "\n")
            .filter { $0.hasPrefix("n/") }
            .map { String($0.dropFirst()) }
    }

    // MARK: - Helpers

    static func parseWorktreeList(_ output: String) -> [ListEntry] {
        var entries: [ListEntry] = []
        var current: ListEntry?
        for line in output.split(separator: "\n", omittingEmptySubsequences: false) {
            if line.isEmpty {
                if let entry = current { entries.append(entry) }
                current = nil
                continue
            }
            let parts = line.split(separator: " ", maxSplits: 1)
            let key = String(parts[0])
            let value = parts.count > 1 ? String(parts[1]) : nil
            switch key {
            case "worktree":
                if let entry = current { entries.append(entry) }
                current = ListEntry(path: value ?? "")
            case "HEAD": current?.head = value
            case "branch": current?.branch = value.map { $0.hasPrefix("refs/heads/") ? String($0.dropFirst(11)) : $0 }
            case "bare": current?.isBare = true
            case "detached": current?.isDetached = true
            case "locked":
                current?.isLocked = true
                current?.lockReason = value
            case "prunable": current?.isPrunable = true
            default: break
            }
        }
        if let entry = current { entries.append(entry) }
        return entries
    }

    /// Picks the branch a worktree counts as "merged" into: origin's default branch if known.
    static func resolveBaseRef(_ repoArgs: [String]) -> String? {
        let originHead = Git.run(repoArgs + ["symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"])
        if originHead.ok, !originHead.output.isEmpty { return originHead.output }
        for candidate in ["origin/main", "origin/master", "origin/develop", "main", "master", "develop"] {
            if Git.run(repoArgs + ["rev-parse", "--verify", "-q", candidate + "^{commit}"]).ok {
                return candidate
            }
        }
        return nil
    }

    /// Maps worktree path -> `<common>/worktrees/<name>` using each admin dir's `gitdir` file.
    static func adminDirectories(_ commonDir: String) -> [String: String] {
        let root = (commonDir as NSString).appendingPathComponent("worktrees")
        var map: [String: String] = [:]
        for name in (try? FileManager.default.contentsOfDirectory(atPath: root)) ?? [] {
            let adminDir = (root as NSString).appendingPathComponent(name)
            guard let gitdir = try? String(contentsOfFile: (adminDir as NSString).appendingPathComponent("gitdir"), encoding: .utf8) else { continue }
            let dotGit = Paths.resolve(gitdir.trimmingCharacters(in: .whitespacesAndNewlines), relativeTo: adminDir)
            let worktreePath = (dotGit as NSString).deletingLastPathComponent
            map[worktreePath] = adminDir
            map[Paths.real(worktreePath)] = adminDir
        }
        return map
    }

    /// Last time git touched the worktree (checkout, commit, staging).
    static func lastActivity(adminDir: String?, path: String) -> Date? {
        guard let adminDir else { return Paths.modificationDate(path) }
        return ["HEAD", "index", "logs/HEAD"]
            .compactMap { Paths.modificationDate((adminDir as NSString).appendingPathComponent($0)) }
            .max()
    }
}
