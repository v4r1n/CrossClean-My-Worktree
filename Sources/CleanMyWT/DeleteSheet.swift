import SwiftUI
import WorktreeKit

struct DeleteSheet: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss

    let items: [Worktree]

    @State private var deleteBranches = false
    @State private var acknowledgedRisk = false
    @State private var stage: Stage = .confirm

    enum Stage {
        case confirm
        case running(done: Int, total: Int)
        case finished(DeleteReport)
    }

    private var risky: [Worktree] { items.filter { $0.verdict == .risky || $0.verdict == .review || $0.verdict == .pending } }
    private var estimatedBytes: Int64 { items.reduce(0) { $0 + $1.bytesFreed } }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            switch stage {
            case .confirm: confirmView
            case .running(let done, let total): runningView(done: done, total: total)
            case .finished(let report): finishedView(report)
            }
        }
        .padding(20)
        .frame(width: 620)
        .interactiveDismissDisabled(isRunning)
    }

    private var isRunning: Bool {
        if case .running = stage { return true }
        return false
    }

    // MARK: - Confirm

    private var confirmView: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(alignment: .top, spacing: 12) {
                Image(systemName: "trash.circle.fill")
                    .font(.system(size: 36))
                    .foregroundStyle(.red)
                VStack(alignment: .leading, spacing: 4) {
                    Text("Delete \(items.count) worktree\(items.count == 1 ? "" : "s")?")
                        .font(.title2.weight(.semibold))
                    Text("Frees about \(Format.bytes(estimatedBytes)). Folders are deleted permanently (not moved to the Trash) and git's worktree records are cleaned up. Branches and commits stay in the repository.")
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }

            List(items) { wt in
                HStack(spacing: 8) {
                    VerdictIcon(verdict: wt.verdict)
                    VStack(alignment: .leading, spacing: 1) {
                        Text(wt.name).fontWeight(.medium)
                        Text("\(wt.repoName) · \(wt.branchLabel)")
                            .font(.caption).foregroundStyle(.secondary)
                    }
                    .lineLimit(1)
                    Spacer()
                    BadgeRow(worktree: wt)
                    Text(Format.bytes(wt.bytesFreed))
                        .monospacedDigit()
                        .frame(width: 70, alignment: .trailing)
                }
            }
            .listStyle(.bordered(alternatesRowBackgrounds: true))
            .frame(height: min(CGFloat(items.count) * 40 + 10, 280))

            if !risky.isEmpty {
                VStack(alignment: .leading, spacing: 6) {
                    Label("\(risky.count) of these have uncommitted changes, unpushed commits, are in use, or couldn't be checked.",
                          systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(.orange)
                        .fixedSize(horizontal: false, vertical: true)
                    Toggle("I understand that work in these worktrees will be lost", isOn: $acknowledgedRisk)
                }
                .padding(10)
                .background(.orange.opacity(0.08), in: RoundedRectangle(cornerRadius: 8))
            }

            Toggle(isOn: $deleteBranches) {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Also delete their local branches when fully merged")
                    Text("Uses `git branch -d`, which keeps any branch with unmerged commits.")
                        .font(.caption).foregroundStyle(.secondary)
                }
            }

            HStack {
                Spacer()
                Button("Cancel", role: .cancel) { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button("Delete \(items.count) Worktree\(items.count == 1 ? "" : "s")", role: .destructive) {
                    Task { await runDelete() }
                }
                .buttonStyle(.borderedProminent)
                .tint(.red)
                .disabled(!risky.isEmpty && !acknowledgedRisk)
            }
        }
    }

    private func runDelete() async {
        stage = .running(done: 0, total: items.count)
        let report = await model.delete(items, deleteBranches: deleteBranches) { done, total in
            stage = .running(done: done, total: total)
        }
        stage = .finished(report)
    }

    // MARK: - Progress & result

    private func runningView(done: Int, total: Int) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Deleting worktrees…").font(.title3.weight(.semibold))
            ProgressView(value: Double(done), total: Double(max(total, 1)))
            Text("\(done) of \(total) done").foregroundStyle(.secondary).monospacedDigit()
        }
    }

    private func finishedView(_ report: DeleteReport) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 12) {
                Image(systemName: report.failures.isEmpty ? "checkmark.circle.fill" : "exclamationmark.circle.fill")
                    .font(.system(size: 36))
                    .foregroundStyle(report.failures.isEmpty ? .green : .orange)
                VStack(alignment: .leading, spacing: 4) {
                    Text("Deleted \(report.removed.count) worktree\(report.removed.count == 1 ? "" : "s")")
                        .font(.title2.weight(.semibold))
                    Text(freedText(report)).foregroundStyle(.secondary)
                }
            }

            if report.branchesDeleted > 0 || !report.branchesKept.isEmpty {
                Text("Deleted \(report.branchesDeleted) merged branch\(report.branchesDeleted == 1 ? "" : "es")."
                     + (report.branchesKept.isEmpty ? "" : " Kept \(report.branchesKept.count) with unmerged commits: \(report.branchesKept.joined(separator: ", "))"))
                    .font(.callout)
                    .fixedSize(horizontal: false, vertical: true)
            }

            if !report.failures.isEmpty {
                Text("Couldn't delete \(report.failures.count):").font(.headline)
                ScrollView {
                    VStack(alignment: .leading, spacing: 8) {
                        ForEach(report.failures, id: \.path) { failure in
                            VStack(alignment: .leading, spacing: 2) {
                                Text(Paths.abbreviate(failure.path)).fontWeight(.medium)
                                Text(failure.message).font(.caption).foregroundStyle(.secondary).textSelection(.enabled)
                            }
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .frame(maxHeight: 160)
            }

            HStack {
                Spacer()
                Button("Done") { dismiss() }
                    .keyboardShortcut(.defaultAction)
            }
        }
    }

    private func freedText(_ report: DeleteReport) -> String {
        var text = "Estimated \(Format.bytes(report.estimatedFreed)) freed"
        if let measured = report.measuredFreed, measured > 0 {
            text += " · disk shows \(Format.bytes(measured)) more free space"
        }
        return text + "."
    }
}
