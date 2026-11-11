package wtkit

import (
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	fileReadAttributes       = 0x80
	fileFlagOpenReparsePoint = 0x00200000
	defaultClusterSize       = 4096
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceW  = kernel32.NewProc("GetDiskFreeSpaceW")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// clusterSize is the allocation unit of the volume holding path.
func clusterSize(path string) int64 {
	root, err := syscall.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
	if err != nil {
		return defaultClusterSize
	}
	var sectorsPerCluster, bytesPerSector, freeClusters, totalClusters uint32
	ok, _, _ := procGetDiskFreeSpaceW.Call(uintptr(unsafe.Pointer(root)),
		uintptr(unsafe.Pointer(&sectorsPerCluster)), uintptr(unsafe.Pointer(&bytesPerSector)),
		uintptr(unsafe.Pointer(&freeClusters)), uintptr(unsafe.Pointer(&totalClusters)))
	size := int64(sectorsPerCluster) * int64(bytesPerSector)
	if ok == 0 || size <= 0 {
		return defaultClusterSize
	}
	return size
}

// longPath adds the `\\?\` prefix that lifts the 260-character MAX_PATH limit
// (os.Open does this itself; raw syscalls don't). Deep node_modules trees need it.
func longPath(path string) string {
	switch {
	case strings.HasPrefix(path, `\\?\`) || !filepath.IsAbs(path):
		return path
	case strings.HasPrefix(path, `\\`):
		return `\\?\UNC\` + path[2:]
	}
	return `\\?\` + filepath.Clean(path)
}

// newStatter opens each file's metadata (never its contents, and never through
// a reparse point) to read its link count and file ID. Allocated size is the
// file size rounded up to the volume's cluster size, an estimate: NTFS stores
// tiny files inside the MFT and compressed files use less.
func newStatter(root string) func(string, fs.DirEntry) (fileStat, bool) {
	cluster := clusterSize(root)
	return func(path string, d fs.DirEntry) (fileStat, bool) {
		name, err := syscall.UTF16PtrFromString(longPath(path))
		if err != nil {
			return fileStat{}, false
		}
		handle, err := syscall.CreateFile(name, fileReadAttributes,
			syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil,
			syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|fileFlagOpenReparsePoint, 0)
		if err != nil {
			return fileStat{}, false
		}
		defer syscall.CloseHandle(handle)

		var info syscall.ByHandleFileInformation
		if err := syscall.GetFileInformationByHandle(handle, &info); err != nil {
			return fileStat{}, false
		}
		device := uint64(info.VolumeSerialNumber)
		st := fileStat{
			links:  uint64(info.NumberOfLinks),
			id:     fileID{device: device, inode: uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)},
			device: device,
		}
		if !d.IsDir() {
			size := int64(info.FileSizeHigh)<<32 | int64(info.FileSizeLow)
			st.allocated = (size + cluster - 1) / cluster * cluster
		}
		return st, true
	}
}
