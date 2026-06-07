package webview

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	// kernel32 is a Windows "Known DLL", always mapped from System32 by the OS,
	// so NewLazyDLL resolves it safely without depending on golang.org/x/sys.
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procSetDefaultDllDirs = kernel32.NewProc("SetDefaultDllDirectories")
	procLoadLibraryExW    = kernel32.NewProc("LoadLibraryExW")
)

const (
	// https://learn.microsoft.com/windows/win32/api/libloaderapi/nf-libloaderapi-setdefaultdlldirectories
	loadLibrarySearchDefaultDirs = 0x00001000
	// https://learn.microsoft.com/windows/win32/api/libloaderapi/nf-libloaderapi-loadlibraryexw
	loadWithAlteredSearchPath = 0x00000008
)

func init() {
	// Opt into the safe DLL search order: System32 and explicitly-added
	// directories only, never the current working directory. Older Windows
	// (pre-8 without KB2533623) lack this API; Find guards against
	// LazyProc.Call panicking on a missing symbol, so we skip the hardening
	// there instead of crashing at package init.
	if procSetDefaultDllDirs.Find() == nil {
		_, _, _ = procSetDefaultDllDirs.Call(uintptr(loadLibrarySearchDefaultDirs))
	}
}

func libraryPath() string {
	const name = "webview.dll"

	var dirs []string
	if p := os.Getenv("WEBVIEW_PATH"); p != "" {
		dirs = append(dirs, p)
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}

	for _, dir := range dirs {
		n := filepath.Join(dir, name)
		if _, err := os.Stat(n); err == nil { //nolint:gosec // trusted lookup dirs (WEBVIEW_PATH/exe dir), not user-tainted
			return n
		}
	}

	// Fall back to the bare name; the safe search order set in init() keeps the
	// CWD out of the lookup.
	return name
}

func loadLibrary(name string) (uintptr, error) {
	// For an absolute path, load with an altered search path so the library's
	// own dependencies resolve from its directory rather than the CWD. Guard
	// with Find so a missing LoadLibraryExW (ancient Windows) falls back to
	// LoadLibrary rather than panicking in LazyProc.Call.
	if filepath.IsAbs(name) && procLoadLibraryExW.Find() == nil {
		p, err := syscall.UTF16PtrFromString(name)
		if err != nil {
			return 0, err
		}
		h, _, callErr := procLoadLibraryExW.Call(
			uintptr(unsafe.Pointer(p)),
			0,
			uintptr(loadWithAlteredSearchPath),
		)
		if h == 0 {
			return 0, callErr
		}
		return h, nil
	}

	handle, err := syscall.LoadLibrary(name)
	return uintptr(handle), err
}

func loadSymbol(lib uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(lib), name)
}
