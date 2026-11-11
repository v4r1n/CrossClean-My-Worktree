//go:build !darwin && !linux && !windows

package wtkit

// AvailableBytes is not implemented on this platform.
func AvailableBytes(string) (int64, bool) { return 0, false }
