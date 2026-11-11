package wtkit

import (
	"syscall"
	"unsafe"
)

// AvailableBytes is the free space available to the current user on the
// volume holding path.
func AvailableBytes(path string) (int64, bool) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	var freeToCaller, total, totalFree uint64
	ok, _, _ := procGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(&freeToCaller)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree)))
	if ok == 0 {
		return 0, false
	}
	return int64(freeToCaller), true
}
