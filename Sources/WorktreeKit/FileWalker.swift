import Foundation

/// Thin wrapper over fts(3); much faster than FileManager enumeration for big trees.
enum FileWalker {
    enum Action { case descend, skip }

    static func walk(
        _ root: String,
        options: Int32 = FTS_PHYSICAL | FTS_NOCHDIR,
        isCancelled: () -> Bool = { false },
        visit: (UnsafeMutablePointer<FTSENT>) -> Action
    ) {
        guard let cRoot = strdup(root) else { return }
        defer { free(cRoot) }
        var argv: [UnsafeMutablePointer<CChar>?] = [cRoot, nil]
        guard let fts = fts_open(&argv, options, nil) else { return }
        defer { fts_close(fts) }

        var count = 0
        while let entry = fts_read(fts) {
            count += 1
            if count & 0xFF == 0, isCancelled() { break }
            if visit(entry) == .skip {
                fts_set(fts, entry, FTS_SKIP)
            }
        }
    }

    @inline(__always)
    static func info(_ entry: UnsafeMutablePointer<FTSENT>) -> Int32 {
        Int32(entry.pointee.fts_info)
    }

    static func path(_ entry: UnsafeMutablePointer<FTSENT>) -> String {
        String(cString: entry.pointee.fts_path)
    }

    /// The entry's file name, read from the tail of fts_path.
    static func name(_ entry: UnsafeMutablePointer<FTSENT>) -> String {
        let nameLength = Int(entry.pointee.fts_namelen)
        let pathLength = Int(entry.pointee.fts_pathlen)
        let start = UnsafeRawPointer(entry.pointee.fts_path)
            .advanced(by: pathLength - nameLength)
            .assumingMemoryBound(to: UInt8.self)
        return String(decoding: UnsafeBufferPointer(start: start, count: nameLength), as: UTF8.self)
    }
}

extension DiskUsage {
    /// Measures a directory tree. Hard-linked files count once, and only count as
    /// reclaimable when every link to them lives inside the tree.
    public static func measure(_ path: String, isCancelled: () -> Bool = { false }) -> DiskUsage {
        struct LinkKey: Hashable { let dev: Int32; let ino: UInt64 }
        var usage = DiskUsage()
        var links: [LinkKey: (seen: Int, nlink: Int, bytes: Int64)] = [:]

        FileWalker.walk(path, options: FTS_PHYSICAL | FTS_NOCHDIR | FTS_XDEV, isCancelled: isCancelled) { entry in
            switch FileWalker.info(entry) {
            case FTS_D, FTS_F, FTS_SL, FTS_SLNONE, FTS_DEFAULT:
                guard let st = entry.pointee.fts_statp?.pointee else { return .descend }
                let bytes = Int64(st.st_blocks) * 512
                let isDir = FileWalker.info(entry) == FTS_D
                if !isDir { usage.fileCount += 1 }
                if !isDir && st.st_nlink > 1 {
                    let key = LinkKey(dev: st.st_dev, ino: st.st_ino)
                    if var existing = links[key] {
                        existing.seen += 1
                        links[key] = existing
                    } else {
                        links[key] = (1, Int(st.st_nlink), bytes)
                    }
                } else {
                    usage.allocated += bytes
                    usage.reclaimable += bytes
                }
            default:
                break
            }
            return .descend
        }

        for link in links.values {
            usage.allocated += link.bytes
            if link.seen >= link.nlink { usage.reclaimable += link.bytes }
        }
        return usage
    }
}
