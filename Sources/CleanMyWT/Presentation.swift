import SwiftUI
import WorktreeKit

enum Format {
    static func bytes(_ value: Int64) -> String {
        ByteCountFormatter.string(fromByteCount: value, countStyle: .file)
    }

    static func relative(_ date: Date?) -> String {
        guard let date else { return "—" }
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .abbreviated
        return formatter.localizedString(for: date, relativeTo: Date())
    }
}

struct Badge: Hashable {
    enum Tone { case good, info, warn, bad, neutral }
    let text: String
    let tone: Tone

    var color: Color {
        switch tone {
        case .good: return .green
        case .info: return .blue
        case .warn: return .orange
        case .bad: return .red
        case .neutral: return .secondary
        }
    }
}

enum VerdictFilter: String, CaseIterable, Identifiable {
    case all = "All"
    case safe = "Safe to delete"
    case review = "Needs review"

    var id: String { rawValue }

    func matches(_ wt: Worktree) -> Bool {
        switch self {
        case .all: return true
        case .safe: return wt.verdict == .safe
        case .review: return wt.verdict == .review || wt.verdict == .risky
        }
    }
}

extension Worktree {
    var badges: [Badge] {
        if isPrunable { return [Badge(text: "Folder already gone", tone: .info)] }
        var result: [Badge] = []
        if inUse { result.append(Badge(text: "In use", tone: .bad)) }
        if isOrphan { result.append(Badge(text: "Repo deleted", tone: .warn)) }
        if isLocked { result.append(Badge(text: "Locked", tone: .warn)) }
        guard let status else {
            return result + [Badge(text: "Checking…", tone: .neutral)]
        }
        if status.error != nil && !isOrphan { result.append(Badge(text: "Git error", tone: .warn)) }
        if status.uncommitted > 0 { result.append(Badge(text: "\(status.uncommitted) uncommitted", tone: .bad)) }
        if let unpushed = status.unpushedCommits, unpushed > 0, status.mergedIntoBase != true {
            result.append(Badge(text: "\(unpushed) unpushed", tone: .bad))
        }
        if status.mergedIntoBase == true {
            result.append(Badge(text: "Merged", tone: .good))
        } else if status.unpushedCommits == 0 {
            result.append(Badge(text: "Pushed", tone: .good))
        }
        if result.isEmpty { result.append(Badge(text: "Clean", tone: .neutral)) }
        return result
    }

    var statusDetail: String {
        var lines = [path]
        if let branch { lines.append("Branch: \(branch)") } else if isDetached { lines.append("Detached HEAD at \(head?.prefix(8) ?? "?")") }
        if isPrunable { lines.append("The folder no longer exists; deleting only cleans up git's record of it.") }
        if let lockReason { lines.append("Locked: \(lockReason)") }
        if inUse { lines.append("A running process (terminal, editor, agent) is working inside this folder.") }
        if let status {
            if let upstream = status.upstream {
                lines.append("Upstream: \(upstream) (ahead \(status.ahead ?? 0), behind \(status.behind ?? 0))")
            }
            if status.changedFiles > 0 { lines.append("\(status.changedFiles) modified tracked files") }
            if status.untrackedFiles > 0 { lines.append("\(status.untrackedFiles) untracked files") }
            if let unpushed = status.unpushedCommits { lines.append("\(unpushed) commits not on any remote") }
            if let merged = status.mergedIntoBase { lines.append(merged ? "Fully merged into the default branch" : "Not merged into the default branch") }
            if let error = status.error { lines.append(error) }
        }
        if let usage, usage.reclaimable < usage.allocated {
            lines.append("\(Format.bytes(usage.allocated - usage.reclaimable)) is hard-linked from elsewhere (e.g. a pnpm store) and won't be freed.")
        }
        return lines.joined(separator: "\n")
    }

    var branchLabel: String { branch ?? (isDetached ? "detached @ \(head?.prefix(7) ?? "?")" : "—") }

    // Sort keys for Table columns.
    var sortSize: Int64 { usage?.allocated ?? -1 }
    var sortFreed: Int64 { usage?.reclaimable ?? -1 }
    var sortActivity: Double { lastActivity?.timeIntervalSince1970 ?? 0 }
    var sortVerdict: Int { verdict.rawValue }
    var sortBranch: String { branchLabel }
}

struct BadgeView: View {
    let badge: Badge

    var body: some View {
        Text(badge.text)
            .font(.caption.weight(.medium))
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .foregroundStyle(badge.color)
            .background(badge.color.opacity(0.14), in: Capsule())
            .lineLimit(1)
            .fixedSize()
    }
}

struct BadgeRow: View {
    let worktree: Worktree

    var body: some View {
        HStack(spacing: 4) {
            ForEach(worktree.badges, id: \.self) { BadgeView(badge: $0) }
        }
        .help(worktree.statusDetail)
    }
}

struct VerdictIcon: View {
    let verdict: Verdict

    var body: some View {
        switch verdict {
        case .safe: Image(systemName: "checkmark.circle.fill").foregroundStyle(.green).help("Safe to delete: no uncommitted or unpushed work")
        case .review: Image(systemName: "questionmark.circle.fill").foregroundStyle(.orange).help("Check before deleting")
        case .risky: Image(systemName: "exclamationmark.triangle.fill").foregroundStyle(.red).help("Deleting loses work or disrupts something running")
        case .pending: ProgressView().controlSize(.mini)
        }
    }
}
