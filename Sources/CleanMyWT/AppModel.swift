import AppKit
import Foundation
import WorktreeKit

enum ScanTarget: Hashable, Codable {
    case home
    case entireMac
    case folder(String)

    var roots: [String] {
        switch self {
        case .home: return [Paths.home]
        case .entireMac: return ["/"]
        case .folder(let path): return [path]
        }
    }

    var title: String {
        switch self {
        case .home: return "Home folder"
        case .entireMac: return "Entire Mac"
        case .folder(let path): return Paths.abbreviate(path)
        }
    }
}

struct RepoSummary: Identifiable {
    let id: String
    let name: String
    let count: Int
    let bytes: Int64
}

struct DeleteReport {
    var removed: [Worktree] = []
    var failures: [(path: String, message: String)] = []
    var branchesDeleted = 0
    var branchesKept: [String] = []
    var measuredFreed: Int64?

    var estimatedFreed: Int64 { removed.reduce(0) { $0 + $1.bytesFreed } }
}

/// Lets the UI cancel synchronous work running on background threads.
final class CancelToken: @unchecked Sendable {
    private let lock = NSLock()
    private var cancelled = false

    var isCancelled: Bool {
        lock.lock(); defer { lock.unlock() }
        return cancelled
    }

    func cancel() {
        lock.lock(); cancelled = true; lock.unlock()
    }
}

@MainActor
final class AppModel: ObservableObject {
    static let allRepos = "all"

    enum Phase: Equatable {
        case idle
        case scanning(String)
        case inspecting(done: Int, total: Int)
        case measuring(done: Int, total: Int)
    }

    @Published var target: ScanTarget {
        didSet { saveTarget() }
    }
    @Published private(set) var recentFolders: [String] = []
    @Published private(set) var repositories: [String: Repository] = [:]
    @Published private(set) var worktrees: [Worktree] = []
    @Published var checked: Set<Worktree.ID> = []
    @Published private(set) var phase: Phase = .idle
    @Published private(set) var isDeleting = false
    @Published private(set) var freeSpace: Int64?
    @Published private(set) var lastScan: Date?

    private var token = CancelToken()
    private var scanTask: Task<Void, Never>?

    init() {
        let defaults = UserDefaults.standard
        if let data = defaults.data(forKey: "scanTarget"),
           let saved = try? JSONDecoder().decode(ScanTarget.self, from: data) {
            target = saved
        } else {
            target = .home
        }
        recentFolders = defaults.stringArray(forKey: "recentFolders") ?? []
        refreshFreeSpace()
    }

    var isScanning: Bool { phase != .idle }
    var isBusy: Bool { isScanning || isDeleting }

    // MARK: - Derived data

    var totalBytes: Int64 { worktrees.reduce(0) { $0 + ($1.usage?.allocated ?? 0) } }

    var repoSummaries: [RepoSummary] {
        Dictionary(grouping: worktrees, by: \.repoID)
            .map { id, items in
                RepoSummary(id: id, name: items[0].repoName, count: items.count,
                            bytes: items.reduce(0) { $0 + ($1.usage?.allocated ?? 0) })
            }
            .sorted { $0.bytes != $1.bytes ? $0.bytes > $1.bytes : $0.name < $1.name }
    }

    var checkedWorktrees: [Worktree] { worktrees.filter { checked.contains($0.id) } }
    var checkedBytes: Int64 { checkedWorktrees.reduce(0) { $0 + $1.bytesFreed } }

    // MARK: - Scanning

    func chooseFolder() {
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.allowsMultipleSelection = false
        panel.prompt = "Scan"
        panel.message = "Choose a folder to search for git worktrees"
        guard panel.runModal() == .OK, let url = panel.url else { return }
        let path = url.path
        recentFolders.removeAll { $0 == path }
        recentFolders.insert(path, at: 0)
        recentFolders = Array(recentFolders.prefix(5))
        UserDefaults.standard.set(recentFolders, forKey: "recentFolders")
        target = .folder(path)
        scan()
    }

    func select(_ newTarget: ScanTarget) {
        target = newTarget
        scan()
    }

    func scanIfNeeded() {
        if lastScan == nil && !isScanning { scan() }
    }

    func stopScan() {
        token.cancel()
        scanTask?.cancel()
        phase = .idle
    }

    func scan() {
        stopScan()
        let token = CancelToken()
        self.token = token
        worktrees = []
        repositories = [:]
        checked = []
        phase = .scanning("Looking for repositories…")
        let roots = target.roots
        scanTask = Task { await runScan(roots: roots, token: token) }
    }

    private func runScan(roots: [String], token: CancelToken) async {
        // 1. Find repositories that have worktrees.
        let found = await background {
            RepoScanner.scan(roots: roots, progress: { count, path in
                Task { @MainActor [weak self] in
                    guard !token.isCancelled else { return }
                    self?.phase = .scanning("\(count.formatted()) folders · \(Paths.abbreviate(path))")
                }
            }, isCancelled: { token.isCancelled })
        }
        guard !token.isCancelled else { return }

        // 2. List each repository's worktrees.
        phase = .scanning("Reading \(found.commonDirs.count) repositories…")
        async let cwdsTask = background { WorktreeInspector.processWorkingDirectories().map(Paths.real) }
        var repos: [String: Repository] = [:]
        var items: [Worktree] = []
        await withTaskGroup(of: (Repository, [Worktree])?.self) { group in
            for commonDir in found.commonDirs {
                group.addTask { WorktreeInspector.load(commonDir: commonDir) }
            }
            for await loaded in group {
                guard let (repo, worktrees) = loaded else { continue }
                repos[repo.id] = repo
                items += worktrees
            }
        }
        items += found.orphanWorktrees.map(WorktreeInspector.orphan(path:))
        let cwds = await cwdsTask
        guard !token.isCancelled else { return }

        var seen = Set<String>()
        items = items.filter { seen.insert(Paths.real($0.path)).inserted }
        for i in items.indices {
            let real = Paths.real(items[i].path)
            items[i].inUse = cwds.contains { $0 == real || $0.hasPrefix(real + "/") }
        }
        items.sort { ($0.repoName.lowercased(), $0.path) < ($1.repoName.lowercased(), $1.path) }
        repositories = repos
        worktrees = items

        // 3. Git status for each worktree.
        let needStatus = items.filter { $0.status == nil && !$0.isPrunable }
        phase = .inspecting(done: 0, total: needStatus.count)
        var done = 0
        let repoSnapshot = repos
        await forEach(needStatus, limit: 8, token: token) { wt -> WorktreeStatus? in
            guard let repo = repoSnapshot[wt.repoID] else { return nil }
            return WorktreeInspector.status(of: wt, in: repo)
        } apply: { [weak self] (id: Worktree.ID, status: WorktreeStatus) in
            self?.update(id) { $0.status = status }
            done += 1
            self?.phase = .inspecting(done: done, total: needStatus.count)
        }
        guard !token.isCancelled else { return }

        // 4. Disk usage, the slow part.
        let needSize = items.filter { !$0.isPrunable }
        phase = .measuring(done: 0, total: needSize.count)
        done = 0
        await forEach(needSize, limit: 4, token: token) { wt -> DiskUsage? in
            var usage = DiskUsage.measure(wt.path, isCancelled: { token.isCancelled })
            if let adminDir = wt.adminDir {
                let admin = DiskUsage.measure(adminDir)
                usage.allocated += admin.allocated
                usage.reclaimable += admin.reclaimable
            }
            return usage
        } apply: { [weak self] (id: Worktree.ID, usage: DiskUsage) in
            self?.update(id) { $0.usage = usage }
            done += 1
            self?.phase = .measuring(done: done, total: needSize.count)
        }
        guard !token.isCancelled else { return }

        phase = .idle
        lastScan = Date()
        refreshFreeSpace()
    }

    /// Runs `work` on background threads (at most `limit` at once) and applies results on the main actor.
    private func forEach<Result: Sendable>(
        _ items: [Worktree],
        limit: Int,
        token: CancelToken,
        work: @escaping @Sendable (Worktree) -> Result?,
        apply: @escaping (Worktree.ID, Result) -> Void
    ) async {
        await withTaskGroup(of: (Worktree.ID, Result?).self) { group in
            var iterator = items.makeIterator()
            func addNext() {
                guard !token.isCancelled, let wt = iterator.next() else { return }
                group.addTask { (wt.id, work(wt)) }
            }
            for _ in 0..<limit { addNext() }
            for await (id, result) in group {
                if let result, !token.isCancelled { apply(id, result) }
                addNext()
            }
        }
    }

    private func update(_ id: Worktree.ID, _ change: (inout Worktree) -> Void) {
        guard let index = worktrees.firstIndex(where: { $0.id == id }) else { return }
        change(&worktrees[index])
    }

    // MARK: - Selection

    func isChecked(_ id: Worktree.ID) -> Bool { checked.contains(id) }

    func setChecked(_ id: Worktree.ID, _ value: Bool) {
        if value { checked.insert(id) } else { checked.remove(id) }
    }

    func check(_ items: [Worktree]) { checked.formUnion(items.map(\.id)) }

    func uncheck(_ items: [Worktree]) { checked.subtract(items.map(\.id)) }

    // MARK: - Deleting

    func delete(_ items: [Worktree], deleteBranches: Bool, progress: @escaping (Int, Int) -> Void) async -> DeleteReport {
        isDeleting = true
        defer { isDeleting = false }
        var report = DeleteReport()
        let before = Volume.rawAvailableBytes()
        var done = 0

        await withTaskGroup(of: (Worktree, String?).self) { group in
            var iterator = items.makeIterator()
            func addNext() {
                guard let wt = iterator.next() else { return }
                group.addTask {
                    do {
                        try WorktreeRemover.remove(wt)
                        return (wt, nil)
                    } catch {
                        return (wt, error.localizedDescription)
                    }
                }
            }
            for _ in 0..<4 { addNext() }
            for await (wt, error) in group {
                if let error {
                    report.failures.append((wt.path, error))
                } else {
                    report.removed.append(wt)
                    worktrees.removeAll { $0.id == wt.id }
                    checked.remove(wt.id)
                }
                done += 1
                progress(done, items.count)
                addNext()
            }
        }

        if deleteBranches {
            let branches = report.removed.compactMap { wt -> (String, String)? in
                guard let branch = wt.branch, !wt.isOrphan else { return nil }
                return (branch, wt.repoID)
            }
            let results = await background {
                branches.map { branch, repoID in (branch, WorktreeRemover.deleteBranch(branch, repoID: repoID).ok) }
            }
            for (branch, ok) in results {
                if ok { report.branchesDeleted += 1 } else { report.branchesKept.append(branch) }
            }
        }

        if let before, let after = Volume.rawAvailableBytes() {
            report.measuredFreed = max(0, after - before)
        }
        refreshFreeSpace()
        return report
    }

    // MARK: - Helpers

    func refreshFreeSpace() {
        freeSpace = Volume.availableBytes()
    }

    private func saveTarget() {
        if let data = try? JSONEncoder().encode(target) {
            UserDefaults.standard.set(data, forKey: "scanTarget")
        }
    }

    private func background<T: Sendable>(_ work: @escaping @Sendable () -> T) async -> T {
        await Task.detached(priority: .userInitiated) { work() }.value
    }
}

extension Worktree {
    func reveal() {
        let path = Paths.exists(self.path) ? self.path : (self.path as NSString).deletingLastPathComponent
        NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: path)])
    }

    func openInTerminal() {
        guard Paths.exists(path) else { return }
        NSWorkspace.shared.open(
            [URL(fileURLWithPath: path)],
            withApplicationAt: URL(fileURLWithPath: "/System/Applications/Utilities/Terminal.app"),
            configuration: NSWorkspace.OpenConfiguration()
        )
    }
}
