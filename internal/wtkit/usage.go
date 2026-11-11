package wtkit

import (
	"context"
	"io/fs"
	"path/filepath"
)

// fileID identifies a file across hard links.
type fileID struct {
	device uint64
	inode  uint64
}

// fileStat is what Measure needs to know about one entry.
type fileStat struct {
	allocated int64
	links     uint64
	id        fileID
	// device lets Measure stay on the root's file system.
	device uint64
}

type linkInfo struct {
	seen, links uint64
	bytes       int64
}

// Measure measures a directory tree. Hard-linked files count once, and count as
// reclaimable only when every link to them lives inside the tree. Symlinks and
// junctions are counted but never followed, and other file systems mounted
// inside the tree are skipped.
func Measure(ctx context.Context, root string) Usage {
	var usage Usage
	links := map[fileID]*linkInfo{}
	stat := newStatter(root)
	var rootDevice uint64
	haveRoot := false

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		st, ok := stat(path, d)
		if !ok {
			return nil
		}
		if d.IsDir() {
			if !haveRoot {
				rootDevice, haveRoot = st.device, true
			} else if st.device != rootDevice {
				return filepath.SkipDir
			}
			usage.Allocated += st.allocated
			usage.Reclaimable += st.allocated
			return nil
		}

		usage.Files++
		if st.links > 1 {
			if info, found := links[st.id]; found {
				info.seen++
			} else {
				links[st.id] = &linkInfo{seen: 1, links: st.links, bytes: st.allocated}
			}
			return nil
		}
		usage.Allocated += st.allocated
		usage.Reclaimable += st.allocated
		return nil
	})

	for _, link := range links {
		usage.Allocated += link.bytes
		if link.seen >= link.links {
			usage.Reclaimable += link.bytes
		}
	}
	return usage
}
