import Foundation

public struct CommandResult: Sendable {
    public let status: Int32
    public let stdout: String
    public let stderr: String

    public var ok: Bool { status == 0 }
    public var output: String { stdout.trimmingCharacters(in: .whitespacesAndNewlines) }
    public var errorOutput: String { stderr.trimmingCharacters(in: .whitespacesAndNewlines) }
}

private final class DataBox: @unchecked Sendable {
    var data = Data()
}

public enum Shell {
    /// Runs a process synchronously and captures its output. Call from a background thread.
    public static func run(_ executable: String, _ args: [String], cwd: String? = nil, env: [String: String] = [:]) -> CommandResult {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: executable)
        process.arguments = args
        if let cwd { process.currentDirectoryURL = URL(fileURLWithPath: cwd) }
        var environment = ProcessInfo.processInfo.environment
        for (key, value) in env { environment[key] = value }
        process.environment = environment

        let out = Pipe()
        let err = Pipe()
        process.standardOutput = out
        process.standardError = err
        process.standardInput = FileHandle.nullDevice

        do {
            try process.run()
        } catch {
            return CommandResult(status: -1, stdout: "", stderr: error.localizedDescription)
        }

        // Drain stderr concurrently so a chatty command can't block on a full pipe.
        // A dedicated thread, not GCD: when callers already occupy every core with
        // blocking work, GCD won't start a worker and the read never runs.
        let errBox = DataBox()
        let done = DispatchSemaphore(value: 0)
        Thread {
            errBox.data = err.fileHandleForReading.readDataToEndOfFile()
            done.signal()
        }.start()
        let outData = out.fileHandleForReading.readDataToEndOfFile()
        done.wait()
        process.waitUntilExit()

        return CommandResult(
            status: process.terminationStatus,
            stdout: String(decoding: outData, as: UTF8.self),
            stderr: String(decoding: errBox.data, as: UTF8.self)
        )
    }
}

public enum Git {
    /// GUI apps get a minimal PATH, so look for git in the usual places.
    public static let executable: String = {
        for candidate in ["/opt/homebrew/bin/git", "/usr/local/bin/git", "/usr/bin/git"]
        where FileManager.default.isExecutableFile(atPath: candidate) {
            return candidate
        }
        return "/usr/bin/git"
    }()

    /// GIT_OPTIONAL_LOCKS=0 keeps `git status` from taking index.lock, so scanning
    /// never interferes with a session that is actively working in a worktree.
    private static let environment = [
        "GIT_OPTIONAL_LOCKS": "0",
        "GIT_TERMINAL_PROMPT": "0",
        "LC_ALL": "C",
    ]

    public static func run(_ args: [String], cwd: String? = nil) -> CommandResult {
        Shell.run(executable, args, cwd: cwd, env: environment)
    }

    /// Arguments that point git at a repository, given its common git dir.
    public static func repoArgs(_ commonDir: String) -> [String] {
        if (commonDir as NSString).lastPathComponent == ".git" {
            return ["-C", (commonDir as NSString).deletingLastPathComponent]
        }
        return ["--git-dir", commonDir]
    }
}

public enum Paths {
    public static let home = NSHomeDirectory()

    /// realpath(3); unlike URL.resolvingSymlinksInPath it keeps /private/tmp intact.
    public static func real(_ path: String) -> String {
        guard let resolved = realpath(path, nil) else {
            return (path as NSString).standardizingPath
        }
        defer { free(resolved) }
        return String(cString: resolved)
    }

    public static func abbreviate(_ path: String) -> String {
        if path == home { return "~" }
        if path.hasPrefix(home + "/") { return "~" + path.dropFirst(home.count) }
        return path
    }

    public static func exists(_ path: String) -> Bool {
        FileManager.default.fileExists(atPath: path)
    }

    public static func isDirectory(_ path: String) -> Bool {
        var isDir: ObjCBool = false
        return FileManager.default.fileExists(atPath: path, isDirectory: &isDir) && isDir.boolValue
    }

    public static func isRegularFile(_ path: String) -> Bool {
        var isDir: ObjCBool = false
        return FileManager.default.fileExists(atPath: path, isDirectory: &isDir) && !isDir.boolValue
    }

    public static func modificationDate(_ path: String) -> Date? {
        var st = stat()
        guard lstat(path, &st) == 0 else { return nil }
        return Date(timeIntervalSince1970: TimeInterval(st.st_mtimespec.tv_sec))
    }

    /// Resolves `path` relative to `base` when it isn't absolute.
    public static func resolve(_ path: String, relativeTo base: String) -> String {
        let joined = path.hasPrefix("/") ? path : (base as NSString).appendingPathComponent(path)
        return (joined as NSString).standardizingPath
    }
}

public enum Volume {
    /// Free space as Finder reports it (includes purgeable space).
    public static func availableBytes(at path: String = Paths.home) -> Int64? {
        let values = try? URL(fileURLWithPath: path).resourceValues(forKeys: [.volumeAvailableCapacityForImportantUsageKey])
        return values?.volumeAvailableCapacityForImportantUsage
    }

    /// Raw free space; better for measuring what a deletion actually returned.
    public static func rawAvailableBytes(at path: String = Paths.home) -> Int64? {
        let values = try? URL(fileURLWithPath: path).resourceValues(forKeys: [.volumeAvailableCapacityKey])
        return values?.volumeAvailableCapacity.map(Int64.init)
    }
}
