//go:build unix

package wtkit

import (
	"io/fs"
	"syscall"
)

// newStatter reads allocated blocks and link counts from lstat(2).
func newStatter(string) func(string, fs.DirEntry) (fileStat, bool) {
	return func(_ string, d fs.DirEntry) (fileStat, bool) {
		info, err := d.Info()
		if err != nil {
			return fileStat{}, false
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fileStat{allocated: info.Size(), links: 1}, true
		}
		device := uint64(st.Dev)
		return fileStat{
			allocated: int64(st.Blocks) * 512,
			links:     uint64(st.Nlink),
			id:        fileID{device: device, inode: uint64(st.Ino)},
			device:    device,
		}, true
	}
}
