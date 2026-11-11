//go:build darwin || linux

package wtkit

import "syscall"

// AvailableBytes is the free space available to the current user on the
// volume holding path.
func AvailableBytes(path string) (int64, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, false
	}
	return int64(uint64(st.Bavail) * uint64(st.Bsize)), true
}
