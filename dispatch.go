package webview

import (
	"sync"

	"github.com/ebitengine/purego"
)

// Global state for functions posted with Dispatch. Entries are keyed by a
// globally-unique counter and carry the webview that posted them so a destroyed
// instance can drop its own entries.
var (
	dispatchMu      sync.Mutex
	dispatchMap     = make(map[uintptr]dispatchEntry)
	dispatchCounter uintptr
)

// dispatchEntry stores a dispatched function and the webview that posted it.
type dispatchEntry struct {
	fn func()
	w  *webview
}

func (w *webview) Dispatch(f func()) {
	dispatchMu.Lock()
	idx := dispatchCounter
	dispatchCounter++
	dispatchMap[idx] = dispatchEntry{fn: f, w: w}
	dispatchMu.Unlock()
	purego.SyscallN(pDispatch, w.handle, dispatchCallback, idx)
}

// dispatchCallbackFn executes a function posted with Dispatch on the main thread.
func dispatchCallbackFn(_, arg uintptr) uintptr {
	dispatchMu.Lock()
	de, ok := dispatchMap[arg]
	delete(dispatchMap, arg)
	dispatchMu.Unlock()
	if ok && de.fn != nil {
		de.fn()
	}
	return 0
}
