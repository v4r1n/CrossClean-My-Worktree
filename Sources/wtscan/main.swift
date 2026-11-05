import Foundation
import WorktreeKit

// Command-line companion: `wtscan [--no-size] [folder ...]` (default: home folder).

var arguments = Array(CommandLine.arguments.dropFirst())
if arguments.contains("-h") || arguments.contains("--help") {
    print("usage: wtscan [--no-size] [folder ...]\nLists git worktrees under the folders (default: ~), with status and disk usage.")
    exit(0)
}
let measureSizes = !arguments.contains("--no-size")
arguments.removeAll { $0 == "--no-size" }
let roots = arguments.isEmpty ? [Paths.home] : arguments.map { Paths.real(($0 as NSString).expandingTildeInPath) }

func bytes(_ value: Int64?) -> String {
    guard let value else { return "-" }
    return ByteCountFormatter.string(fromByteCount: value, countStyle: .file)
}

let started = Date()
let scan = RepoScanner.scan(roots: roots)
FileHandle.standardError.write("Scanned \(scan.directoriesVisited) folders in \(String(format: "%.1f", Date().timeIntervalSince(started)))s, \(scan.commonDirs.count) repos with worktrees\n".data(using: .utf8)!)

let cwds = WorktreeInspector.processWorkingDirectories()
var rows: [Worktree] = []
let lock = NSLock()

let repos = Array(scan.commonDirs).sorted()
DispatchQueue.concurrentPerform(iterations: repos.count) { i in
    guard let (repo, worktrees) = WorktreeInspector.load(commonDir: repos[i]) else { return }
    var loaded: [Worktree] = []
    for var wt in worktrees {
        if !wt.isPrunable {
            wt.status = WorktreeInspector.status(of: wt, in: repo)
            if measureSizes { wt.usage = DiskUsage.measure(wt.path) }
        }
        let real = Paths.real(wt.path)
        wt.inUse = cwds.contains { $0 == real || $0.hasPrefix(real + "/") }
        loaded.append(wt)
    }
    lock.lock(); rows += loaded; lock.unlock()
}
for path in scan.orphanWorktrees {
    var wt = WorktreeInspector.orphan(path: path)
    if measureSizes { wt.usage = DiskUsage.measure(path) }
    rows.append(wt)
}

rows.sort { ($0.repoName, $0.path) < ($1.repoName, $1.path) }
let verdictLabel: [Verdict: String] = [.safe: "SAFE", .review: "REVIEW", .risky: "RISKY", .pending: "?"]
for wt in rows {
    var notes: [String] = []
    if wt.isPrunable { notes.append("missing dir") }
    if wt.isOrphan { notes.append("orphan") }
    if wt.inUse { notes.append("in use") }
    if wt.isLocked { notes.append("locked") }
    if let s = wt.status {
        if s.uncommitted > 0 { notes.append("\(s.uncommitted) uncommitted") }
        if let u = s.unpushedCommits, u > 0 { notes.append("\(u) unpushed") }
        if s.mergedIntoBase == true { notes.append("merged") }
        if let e = s.error, !wt.isOrphan { notes.append("error: \(e.prefix(60))") }
    }
    let line = [
        verdictLabel[wt.verdict]!.padding(toLength: 7, withPad: " ", startingAt: 0),
        wt.repoName.padding(toLength: 24, withPad: " ", startingAt: 0),
        bytes(wt.usage?.allocated).padding(toLength: 9, withPad: " ", startingAt: 0),
        bytes(wt.usage?.reclaimable).padding(toLength: 9, withPad: " ", startingAt: 0),
        (wt.branch ?? (wt.isDetached ? "(detached)" : "-")).padding(toLength: 36, withPad: " ", startingAt: 0),
        notes.joined(separator: ", ").padding(toLength: 30, withPad: " ", startingAt: 0),
        Paths.abbreviate(wt.path),
    ].joined(separator: " ")
    print(line)
}

let total = rows.reduce(Int64(0)) { $0 + ($1.usage?.allocated ?? 0) }
let reclaimable = rows.reduce(Int64(0)) { $0 + $1.bytesFreed }
let safe = rows.filter { $0.verdict == .safe }
print("\n\(rows.count) worktrees · \(bytes(total)) on disk · \(bytes(reclaimable)) reclaimable · \(safe.count) safe to delete (\(bytes(safe.reduce(0) { $0 + $1.bytesFreed })))")
