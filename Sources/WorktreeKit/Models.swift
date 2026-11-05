import Foundation

public struct Repository: Identifiable, Hashable, Sendable {
    public var id: String { commonDir }
    /// The shared git dir (`<repo>/.git`, or the bare repo dir).
    public let commonDir: String
    public let name: String
    /// Main working tree; nil for bare repositories.
    public let mainPath: String?
    /// Branch used to decide "merged", e.g. `origin/develop`.
    public let baseRef: String?
    public let hasRemotes: Bool

    public init(commonDir: String, name: String, mainPath: String?, baseRef: String?, hasRemotes: Bool) {
        self.commonDir = commonDir
        self.name = name
        self.mainPath = mainPath
        self.baseRef = baseRef
        self.hasRemotes = hasRemotes
    }
}

public struct WorktreeStatus: Hashable, Sendable {
    public var changedFiles = 0
    public var untrackedFiles = 0
    public var upstream: String?
    public var ahead: Int?
    public var behind: Int?
    /// Commits reachable from HEAD that are on no remote branch; nil when the repo has no remotes.
    public var unpushedCommits: Int?
    public var mergedIntoBase: Bool?
    public var error: String?

    public init() {}

    public var uncommitted: Int { changedFiles + untrackedFiles }
}

public struct DiskUsage: Hashable, Sendable {
    /// Bytes allocated on disk (like `du`).
    public var allocated: Int64 = 0
    /// Bytes that deleting would actually free: excludes files hard-linked from
    /// outside the tree (e.g. a pnpm store).
    public var reclaimable: Int64 = 0
    public var fileCount = 0

    public init() {}
}

public enum Verdict: Int, Comparable, Sendable {
    case safe = 0, review = 1, risky = 2, pending = 3

    public static func < (lhs: Verdict, rhs: Verdict) -> Bool { lhs.rawValue < rhs.rawValue }
}

public struct Worktree: Identifiable, Hashable, Sendable {
    public static let orphanRepoID = "orphan:"

    public var id: String { path }
    public let path: String
    /// Common dir of the owning repo, or `orphanRepoID` when the repo is gone.
    public let repoID: String
    public let repoName: String
    public var head: String?
    public var branch: String?
    public var isDetached = false
    public var isLocked = false
    public var lockReason: String?
    /// Registered with git but the directory no longer exists.
    public var isPrunable = false
    /// Has a `.git` file but the repository it points to is gone.
    public var isOrphan = false
    /// `<common>/worktrees/<name>`.
    public var adminDir: String?
    public var lastActivity: Date?
    public var status: WorktreeStatus?
    public var usage: DiskUsage?
    /// Some process has its working directory inside this worktree.
    public var inUse = false

    public init(path: String, repoID: String, repoName: String) {
        self.path = path
        self.repoID = repoID
        self.repoName = repoName
    }

    public var name: String { (path as NSString).lastPathComponent }

    public var verdict: Verdict {
        if isPrunable { return .safe }
        if inUse { return .risky }
        if isOrphan { return .review }
        guard let status else { return .pending }
        if status.error != nil { return .review }
        if status.uncommitted > 0 { return .risky }
        if isLocked { return .review }
        if status.mergedIntoBase == true || status.unpushedCommits == 0 { return .safe }
        if let unpushed = status.unpushedCommits, unpushed > 0 { return .risky }
        // No remote to compare against, and the commits aren't merged: local-only work.
        if status.unpushedCommits == nil, status.mergedIntoBase == false { return .risky }
        return .review
    }

    /// Best estimate of the space a delete returns.
    public var bytesFreed: Int64 { usage?.reclaimable ?? 0 }
}
