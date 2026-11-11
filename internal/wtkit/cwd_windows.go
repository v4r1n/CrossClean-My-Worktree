package wtkit

import (
	"strings"
	"syscall"
	"unsafe"
)

// Offsets into 64-bit PEB and RTL_USER_PROCESS_PARAMETERS structures.
const (
	pebProcessParametersOffset = 0x20
	paramsCurrentDirOffset     = 0x38
	processQueryInformation    = 0x0400
	processVMRead              = 0x0010
	maxCwdBytes                = 32*1024*2 - 2
)

var (
	ntdll                         = syscall.NewLazyDLL("ntdll.dll")
	procNtQueryInformationProcess = ntdll.NewProc("NtQueryInformationProcess")
	procReadProcessMemory         = kernel32.NewProc("ReadProcessMemory")
)

type processBasicInformation struct {
	ExitStatus                   uintptr
	PebBaseAddress               uintptr
	AffinityMask                 uintptr
	BasePriority                 uintptr
	UniqueProcessID              uintptr
	InheritedFromUniqueProcessID uintptr
}

// ProcessWorkingDirectories returns the working directories of the processes
// this user can read. Windows has no public API for this, so it reads each
// process's PEB, which is how Process Explorer does it. Processes it can't
// open (other users, protected processes) are skipped.
func ProcessWorkingDirectories() []string {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return nil
	}
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(snapshot)

	var dirs []string
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		if entry.ProcessID <= 4 {
			continue
		}
		if dir, ok := processCwd(entry.ProcessID); ok {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

func processCwd(pid uint32) (string, bool) {
	handle, err := syscall.OpenProcess(processQueryInformation|processVMRead, false, pid)
	if err != nil {
		return "", false
	}
	defer syscall.CloseHandle(handle)

	var info processBasicInformation
	status, _, _ := procNtQueryInformationProcess.Call(uintptr(handle), 0,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), 0)
	if status != 0 || info.PebBaseAddress == 0 {
		return "", false
	}

	var params uintptr
	if !readMemory(handle, info.PebBaseAddress+pebProcessParametersOffset, unsafe.Pointer(&params), 8) || params == 0 {
		return "", false
	}
	// UNICODE_STRING: Length uint16, MaximumLength uint16, padding, Buffer pointer.
	var dirString struct {
		Length, MaximumLength uint16
		_                     uint32
		Buffer                uintptr
	}
	if !readMemory(handle, params+paramsCurrentDirOffset, unsafe.Pointer(&dirString), unsafe.Sizeof(dirString)) {
		return "", false
	}
	if dirString.Length == 0 || dirString.Length > maxCwdBytes || dirString.Buffer == 0 {
		return "", false
	}
	buf := make([]uint16, dirString.Length/2)
	if !readMemory(handle, dirString.Buffer, unsafe.Pointer(&buf[0]), uintptr(dirString.Length)) {
		return "", false
	}
	dir := syscall.UTF16ToString(buf)
	if len(dir) > 3 {
		dir = strings.TrimRight(dir, `\`)
	}
	return dir, dir != ""
}

func readMemory(process syscall.Handle, address uintptr, dest unsafe.Pointer, size uintptr) bool {
	var read uintptr
	ok, _, _ := procReadProcessMemory.Call(uintptr(process), address, uintptr(dest), size, uintptr(unsafe.Pointer(&read)))
	return ok != 0 && read == size
}
