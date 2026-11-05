import SwiftUI
import WorktreeKit

struct ContentView: View {
    @EnvironmentObject private var model: AppModel
    @State private var sidebarSelection: String? = AppModel.allRepos
    @State private var tableSelection = Set<Worktree.ID>()
    @State private var sortOrder = [KeyPathComparator(\Worktree.sortSize, order: .reverse)]
    @State private var search = ""
    @State private var filter: VerdictFilter = .all
    @State private var deleteRequest: DeleteRequest?
    /// Last checkbox clicked; shift-click marks everything between it and the new click.
    @State private var checkAnchor: Worktree.ID?

    private var rows: [Worktree] {
        model.worktrees
            .filter { wt in
                (sidebarSelection == nil || sidebarSelection == AppModel.allRepos || wt.repoID == sidebarSelection)
                    && filter.matches(wt)
                    && (search.isEmpty
                        || wt.path.localizedCaseInsensitiveContains(search)
                        || wt.repoName.localizedCaseInsensitiveContains(search)
                        || (wt.branch ?? "").localizedCaseInsensitiveContains(search))
            }
            .sorted(using: sortOrder)
    }

    var body: some View {
        NavigationSplitView {
            sidebar
                .navigationSplitViewColumnWidth(min: 200, ideal: 240)
        } detail: {
            VStack(spacing: 0) {
                if model.worktrees.isEmpty {
                    emptyState
                } else {
                    table
                }
                Divider()
                bottomBar
            }
        }
        .navigationSubtitle("Scanning: \(model.target.title)")
        .searchable(text: $search, placement: .toolbar, prompt: "Filter by name, repo or branch")
        .toolbar { toolbar }
        .sheet(item: $deleteRequest) { request in
            DeleteSheet(items: request.items)
        }
        .task { model.scanIfNeeded() }
    }

    // MARK: - Sidebar

    private var sidebar: some View {
        List(selection: $sidebarSelection) {
            Section("Scope") {
                SidebarRow(title: "All worktrees", systemImage: "square.stack.3d.up",
                           count: model.worktrees.count, bytes: model.totalBytes)
                    .tag(Optional(AppModel.allRepos))
            }
            Section("Repositories") {
                ForEach(model.repoSummaries) { repo in
                    SidebarRow(title: repo.name,
                               systemImage: repo.id == Worktree.orphanRepoID ? "exclamationmark.triangle" : "shippingbox",
                               count: repo.count, bytes: repo.bytes)
                        .tag(Optional(repo.id))
                        .help(repo.id == Worktree.orphanRepoID ? "Worktrees whose repository was deleted" : Paths.abbreviate(repo.id))
                }
            }
        }
    }

    // MARK: - Table

    private var table: some View {
        Table(rows, selection: $tableSelection, sortOrder: $sortOrder) {
            TableColumn("") { wt in
                Toggle("", isOn: Binding(get: { model.isChecked(wt.id) }, set: { setChecked(wt.id, $0) }))
                    .toggleStyle(.checkbox)
                    .labelsHidden()
                    .help("Shift-click to mark a range")
            }
            .width(20)

            TableColumn("", value: \.sortVerdict) { wt in
                VerdictIcon(verdict: wt.verdict)
            }
            .width(22)

            TableColumn("Worktree", value: \.name) { wt in
                VStack(alignment: .leading, spacing: 1) {
                    Text(wt.name).fontWeight(.medium)
                    Text(Paths.abbreviate((wt.path as NSString).deletingLastPathComponent))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .truncationMode(.middle)
                }
                .lineLimit(1)
                .help(wt.path)
            }
            .width(min: 160, ideal: 240)

            TableColumn("Repository", value: \.repoName) { wt in
                Text(wt.repoName).lineLimit(1)
            }
            .width(min: 90, ideal: 140)

            TableColumn("Branch", value: \.sortBranch) { wt in
                Text(wt.branchLabel)
                    .font(.system(.callout, design: .monospaced))
                    .lineLimit(1)
                    .truncationMode(.middle)
                    .help(wt.branchLabel)
            }
            .width(min: 120, ideal: 220)

            TableColumn("Status") { wt in
                BadgeRow(worktree: wt)
            }
            .width(min: 140, ideal: 200)

            TableColumn("Last active", value: \.sortActivity) { wt in
                Text(Format.relative(wt.lastActivity))
                    .foregroundStyle(.secondary)
                    .help(wt.lastActivity?.formatted(date: .abbreviated, time: .shortened) ?? "")
            }
            .width(min: 70, ideal: 90)

            TableColumn("Size", value: \.sortSize) { wt in
                sizeText(wt.usage?.allocated, pending: !wt.isPrunable)
            }
            .width(min: 64, ideal: 80)

            TableColumn("Frees", value: \.sortFreed) { wt in
                sizeText(wt.usage?.reclaimable, pending: !wt.isPrunable)
                    .fontWeight(.medium)
            }
            .width(min: 64, ideal: 80)
        }
        .contextMenu(forSelectionType: Worktree.ID.self) { ids in
            let items = model.worktrees.filter { ids.contains($0.id) }
            Button("Mark for Deletion") { model.check(items) }
            Button("Unmark") { model.uncheck(items) }
            Divider()
            Button("Delete…") { deleteRequest = DeleteRequest(items: items) }
            Divider()
            if items.count == 1, let wt = items.first {
                Button("Reveal in Finder") { wt.reveal() }
                Button("Open in Terminal") { wt.openInTerminal() }
                    .disabled(!Paths.exists(wt.path))
            }
            Button("Copy Path") {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(items.map(\.path).joined(separator: "\n"), forType: .string)
            }
        } primaryAction: { ids in
            model.worktrees.first { ids.contains($0.id) }?.reveal()
        }
        .onKeyPress(.space) { toggleHighlightedRows() }
    }

    /// Applies a checkbox click; with Shift held, applies the same value to every
    /// visible row between the previous click and this one.
    private func setChecked(_ id: Worktree.ID, _ value: Bool) {
        let visible = rows.map(\.id)
        if NSEvent.modifierFlags.contains(.shift),
           let anchor = checkAnchor,
           let from = visible.firstIndex(of: anchor),
           let to = visible.firstIndex(of: id) {
            for rowID in visible[min(from, to)...max(from, to)] {
                model.setChecked(rowID, value)
            }
        } else {
            model.setChecked(id, value)
        }
        checkAnchor = id
    }

    /// Space marks the highlighted rows, or unmarks them if they're all marked already.
    private func toggleHighlightedRows() -> KeyPress.Result {
        let items = model.worktrees.filter { tableSelection.contains($0.id) }
        guard !items.isEmpty else { return .ignored }
        if items.allSatisfy({ model.isChecked($0.id) }) {
            model.uncheck(items)
        } else {
            model.check(items)
        }
        return .handled
    }

    @ViewBuilder
    private func sizeText(_ bytes: Int64?, pending: Bool) -> some View {
        if let bytes {
            Text(Format.bytes(bytes)).monospacedDigit()
        } else if pending {
            Text("…").foregroundStyle(.tertiary)
        } else {
            Text("—").foregroundStyle(.tertiary)
        }
    }

    @ViewBuilder
    private var emptyState: some View {
        if model.isScanning {
            VStack(spacing: 12) {
                ProgressView()
                Text(phaseText).foregroundStyle(.secondary).lineLimit(1).truncationMode(.middle)
            }
            .padding()
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if model.lastScan == nil {
            ContentUnavailableView {
                Label("Scan for worktrees", systemImage: "magnifyingglass")
            } description: {
                Text("Pick where to look from the toolbar.")
            } actions: {
                Button("Scan \(model.target.title)") { model.scan() }
            }
        } else {
            ContentUnavailableView("No worktrees in \(model.target.title)", systemImage: "checkmark.seal",
                                   description: Text("Nothing to clean up."))
        }
    }

    // MARK: - Bottom bar

    private var bottomBar: some View {
        let visible = rows
        let checkedCount = model.checked.count
        return HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                Text("\(model.worktrees.count) worktrees · \(Format.bytes(model.totalBytes)) on disk")
                    .font(.callout.weight(.medium))
                Group {
                    if model.isScanning {
                        Text(phaseText)
                    } else if let free = model.freeSpace {
                        Text("\(Format.bytes(free)) available on this Mac")
                    }
                }
                .font(.caption)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .truncationMode(.middle)
            }

            Spacer()

            Menu("Mark") {
                Button("Safe to Delete (\(visible.filter { $0.verdict == .safe }.count))") {
                    model.check(visible.filter { $0.verdict == .safe })
                }
                Button("All Shown (\(visible.count))") { model.check(visible) }
                Divider()
                Button("Clear Marks") { model.checked = [] }
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
            .disabled(visible.isEmpty)

            if checkedCount > 0 {
                Text("\(checkedCount) marked · frees \(Format.bytes(model.checkedBytes))")
                    .monospacedDigit()
            }

            Button(role: .destructive) {
                deleteRequest = DeleteRequest(items: model.checkedWorktrees)
            } label: {
                Label("Delete Marked…", systemImage: "trash")
            }
            .buttonStyle(.borderedProminent)
            .tint(.red)
            .disabled(checkedCount == 0 || model.isDeleting)
            .keyboardShortcut(.delete, modifiers: .command)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .background(.bar)
    }

    private var phaseText: String {
        switch model.phase {
        case .idle: return ""
        case .scanning(let message): return message
        case .inspecting(let done, let total): return "Checking git status \(done)/\(total)…"
        case .measuring(let done, let total): return "Measuring disk usage \(done)/\(total)…"
        }
    }

    // MARK: - Toolbar

    @ToolbarContentBuilder
    private var toolbar: some ToolbarContent {
        ToolbarItem(placement: .navigation) {
            Menu {
                Button("Home Folder") { model.select(.home) }
                Button("Entire Mac") { model.select(.entireMac) }
                if !model.recentFolders.isEmpty {
                    Divider()
                    ForEach(model.recentFolders, id: \.self) { path in
                        Button(Paths.abbreviate(path)) { model.select(.folder(path)) }
                    }
                }
                Divider()
                Button("Choose Folder…") { model.chooseFolder() }
            } label: {
                Label(model.target.title, systemImage: "folder")
                    .labelStyle(.titleAndIcon)
            }
            .help("Where to look for worktrees")
        }

        ToolbarItem {
            Picker("Show", selection: $filter) {
                ForEach(VerdictFilter.allCases) { Text($0.rawValue).tag($0) }
            }
            .pickerStyle(.segmented)
        }

        ToolbarItem {
            if model.isScanning {
                Button { model.stopScan() } label: { Label("Stop", systemImage: "stop.fill") }
                    .help("Stop scanning")
            } else {
                Button { model.scan() } label: { Label("Rescan", systemImage: "arrow.clockwise") }
                    .help("Scan again (⌘R)")
                    .keyboardShortcut("r")
                    .disabled(model.isDeleting)
            }
        }
    }
}

private struct DeleteRequest: Identifiable {
    let id = UUID()
    let items: [Worktree]
}

struct SidebarRow: View {
    let title: String
    let systemImage: String
    let count: Int
    let bytes: Int64

    var body: some View {
        HStack {
            Label {
                VStack(alignment: .leading, spacing: 1) {
                    Text(title).lineLimit(1)
                    if bytes > 0 {
                        Text(Format.bytes(bytes)).font(.caption).foregroundStyle(.secondary)
                    }
                }
            } icon: {
                Image(systemName: systemImage)
            }
            Spacer()
            Text("\(count)")
                .font(.caption.monospacedDigit())
                .foregroundStyle(.secondary)
        }
    }
}
