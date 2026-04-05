//go:build windows

package updater

import (
	"syscall"
	"unsafe"
)

var rtlGetVersion = func() (uint32, uint32, error) {
	type osVersionInfoEx struct {
		dwOSVersionInfoSize uint32
		dwMajorVersion      uint32
		dwMinorVersion      uint32
		dwBuildNumber       uint32
		dwPlatformId        uint32
		szCSDVersion        [128]uint16
		wServicePackMajor   uint16
		wServicePackMinor   uint16
		wSuiteMask          uint16
		wProductType        byte
		wReserved           byte
	}

	info := osVersionInfoEx{}
	info.dwOSVersionInfoSize = uint32(unsafe.Sizeof(info))
	dll := syscall.NewLazyDLL("ntdll.dll")
	proc := dll.NewProc("RtlGetVersion")
	status, _, _ := proc.Call(uintptr(unsafe.Pointer(&info)))
	if status != 0 {
		return 0, 0, syscall.Errno(status)
	}
	return info.dwMajorVersion, info.dwMinorVersion, nil
}

func isWindows7OrEarlier() bool {
	major, minor, err := rtlGetVersion()
	if err != nil {
		return false
	}
	return major < 6 || (major == 6 && minor <= 1)
}
