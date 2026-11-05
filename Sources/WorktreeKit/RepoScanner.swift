import Foundation

public struct ScanResult: Sendable {
    /// Common git dirs of repositories that have linked worktrees.
    public var commonDirs: Set<String> = []
    /// Worktree directories whose repository no longer exists.
    public var orphanWorktrees: [String] = []
    public var directoriesVisited = 0
}

/// Walks folders looking for repositories with worktrees.
///
/// A repo's worktree list includes worktrees anywhere on disk (e.g. /private/tmp),
/// so finding the repo is enough; `.git` files under the root also lead back to
/// repos that live outside it.
public enum RepoScanner {
    /// Folder names that never contain worktrees worth finding and are expensive to walk.
    public static let skippedNames: Set<String> = [
        "node_modules", ".Trash", ".cache", ".npm", ".pnpm-store", ".yarn", ".bun",
        ".cargo", ".rustup", ".gradle", ".m2", ".cocoapods", ".android", ".docker",
        ".colima", ".orbstack", ".vscode", ".cursor", ".nvm", ".pyenv", ".rbenv",
        "DerivedData", "Pods", ".build", ".venv", "venv", "__pycache__", ".tox",
        ".next", ".nuxt", ".turbo", ".svelte-kit", ".terraform", ".expo",
    ]

    /// System locations that matter only when scanning the whole Mac.
    public static let skippedPaths: Set<String> = [
        "/System", "/Library", "/Applications", "/dev", "/Volumes", "/Network",
        "/cores", "/bin", "/sbin", "/usr", "/private/var", "/opt/homebrew",
        "/.Spotlight-V100", "/.fseventsd", "/.DocumentRevisions-V100", "/.vol",
        Paths.home + "/Library", Paths.home + "/.local", Paths.home + "/go/pkg",
        Paths.home + "/.kube",
    ]

    public static func scan(
        roots: [String],
        progress: ((Int, String) -> Void)? = nil,
        isCancelled: () -> Bool = { false }
    ) -> ScanResult {
        var result = ScanResult()

        for root in roots {
            FileWalker.walk(root, options: FTS_PHYSICAL | FTS_NOCHDIR | FTS_COMFOLLOW, isCancelled: isCancelled) { entry in
                let info = FileWalker.info(entry)
                let level = entry.pointee.fts_level

                if info == FTS_D {
                    result.directoriesVisited += 1
                    if result.directoriesVisited % 2000 == 0 {
                        progress?(result.directoriesVisited, FileWalker.path(entry))
                    }
                    guard level > 0 else { return .descend }
                    let name = FileWalker.name(entry)
                    if name == ".git" {
                        let gitDir = FileWalker.path(entry)
                        if hasLinkedWorktrees(gitDir) { result.commonDirs.insert(Paths.real(gitDir)) }
                        return .skip
                    }
                    if skippedNames.contains(name) || name.hasSuffix(".app") || name.hasSuffix(".photoslibrary") {
                        return .skip
                    }
                    if skippedPaths.contains(FileWalker.path(entry)) || isOtherUsersLibrary(FileWalker.path(entry)) {
                        return .skip
                    }
                    return .descend
                }

                if info == FTS_F, FileWalker.name(entry) == ".git" {
                    let gitFile = FileWalker.path(entry)
                    switch resolveGitFile(gitFile) {
                    case .worktree(let commonDir):
                        result.commonDirs.insert(Paths.real(commonDir))
                    case .orphan:
                        result.orphanWorktrees.append((gitFile as NSString).deletingLastPathComponent)
                    case .other:
                        break
                    }
                }
                return .descend
            }
        }
        return result
    }

    enum GitFileTarget {
        case worktree(commonDir: String)
        case orphan
        /// Submodule or something we don't manage.
        case other
    }

    /// Reads a worktree's `.git` file (`gitdir: <common>/worktrees/<name>`).
    static func resolveGitFile(_ gitFile: String) -> GitFileTarget {
        guard let contents = try? String(contentsOfFile: gitFile, encoding: .utf8),
              let line = contents.split(separator: "\n").first,
              line.hasPrefix("gitdir:")
        else { return .other }

        let worktreeDir = (gitFile as NSString).deletingLastPathComponent
        let adminDir = Paths.resolve(line.dropFirst("gitdir:".count).trimmingCharacters(in: .whitespaces), relativeTo: worktreeDir)

        // Linked worktrees point into `<common>/worktrees/`; submodules point into `modules/`.
        guard ((adminDir as NSString).deletingLastPathComponent as NSString).lastPathComponent == "worktrees" else {
            return .other
        }
        guard Paths.isDirectory(adminDir) else { return .orphan }

        let commondirFile = (adminDir as NSString).appendingPathComponent("commondir")
        let commonDir: String
        if let relative = try? String(contentsOfFile: commondirFile, encoding: .utf8) {
            commonDir = Paths.resolve(relative.trimmingCharacters(in: .whitespacesAndNewlines), relativeTo: adminDir)
        } else {
            commonDir = ((adminDir as NSString).deletingLastPathComponent as NSString).deletingLastPathComponent
        }
        return Paths.isDirectory(commonDir) ? .worktree(commonDir: commonDir) : .orphan
    }

    static func hasLinkedWorktrees(_ gitDir: String) -> Bool {
        let worktrees = (gitDir as NSString).appendingPathComponent("worktrees")
        let entries = (try? FileManager.default.contentsOfDirectory(atPath: worktrees)) ?? []
        return !entries.isEmpty
    }

    private static func isOtherUsersLibrary(_ path: String) -> Bool {
        let parent = (path as NSString).deletingLastPathComponent
        return (path as NSString).lastPathComponent == "Library"
            && (parent as NSString).deletingLastPathComponent == "/Users"
    }
}
