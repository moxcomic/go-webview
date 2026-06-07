package webview

import (
	"errors"
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

// Loaded once, the first time a webview is created. errLoad records any failure
// so NewWindow can return it instead of panicking.
var (
	loadOnce sync.Once
	errLoad  error
)

// Function pointers for native library functions.
var (
	pCreate    uintptr
	pDestroy   uintptr
	pRun       uintptr
	pTerminate uintptr
	pDispatch  uintptr
	pGetWindow uintptr
	pSetTitle  uintptr
	pSetSize   uintptr
	pNavigate  uintptr
	pSetHtml   uintptr
	pInit      uintptr
	pEval      uintptr
	pBind      uintptr
	pUnbind    uintptr
	pReturn    uintptr
)

// Callback function pointers, created once when the library is loaded.
var (
	dispatchCallback uintptr
	bindingCallback  uintptr
)

// loadLibraryAndSymbols resolves the native library and all required symbols.
// It returns an error instead of panicking so callers of New/NewWindow can
// handle a missing or broken native library gracefully. Platform-specific
// libraryPath, loadLibrary and loadSymbol live in loader_unix.go and
// loader_windows.go.
func loadLibraryAndSymbols() error {
	libHandle, err := loadLibrary(libraryPath())
	if err != nil {
		return fmt.Errorf("webview: failed to load native library: %w", err)
	}
	if libHandle == 0 {
		return errors.New("webview: native library not loaded")
	}

	symbols := []struct {
		ptr  *uintptr
		name string
	}{
		{&pCreate, "webview_create"},
		{&pDestroy, "webview_destroy"},
		{&pRun, "webview_run"},
		{&pTerminate, "webview_terminate"},
		{&pDispatch, "webview_dispatch"},
		{&pGetWindow, "webview_get_window"},
		{&pSetTitle, "webview_set_title"},
		{&pSetSize, "webview_set_size"},
		{&pNavigate, "webview_navigate"},
		{&pSetHtml, "webview_set_html"},
		{&pInit, "webview_init"},
		{&pEval, "webview_eval"},
		{&pBind, "webview_bind"},
		{&pUnbind, "webview_unbind"},
		{&pReturn, "webview_return"},
	}
	for _, s := range symbols {
		ptr, err := loadSymbol(libHandle, s.name)
		if err != nil {
			return fmt.Errorf("webview: failed to load symbol %s: %w", s.name, err)
		}
		*s.ptr = ptr
	}

	dispatchCallback = purego.NewCallback(dispatchCallbackFn)
	bindingCallback = purego.NewCallback(bindingCallbackFn)
	return nil
}
