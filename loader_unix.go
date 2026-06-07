//go:build darwin || linux

package webview

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/ebitengine/purego"
)

func libraryPath() string {
	var name string
	switch runtime.GOOS {
	case "linux":
		name = "libwebview.so"
	case "darwin":
		name = "libwebview.dylib"
	}

	// Resolve against trusted directories only. An empty WEBVIEW_PATH is
	// ignored rather than collapsing to a relative name that would load from
	// the current working directory.
	var dirs []string
	if p := os.Getenv("WEBVIEW_PATH"); p != "" {
		dirs = append(dirs, p)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		dirs = append(dirs, dir)
		if runtime.GOOS == "darwin" {
			dirs = append(dirs, filepath.Join(dir, "..", "Frameworks"))
		}
	}

	for _, dir := range dirs {
		n := filepath.Join(dir, name)
		if _, err := os.Stat(n); err == nil { //nolint:gosec // trusted lookup dirs (WEBVIEW_PATH/exe dir), not user-tainted
			return n
		}
	}

	// Fall back to the bare name so dlopen can find a system-installed library
	// via the platform loader's standard search paths.
	return name
}

func loadLibrary(name string) (uintptr, error) {
	return purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
}

func loadSymbol(lib uintptr, name string) (uintptr, error) {
	return purego.Dlsym(lib, name)
}
