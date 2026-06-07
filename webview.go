package webview

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

func init() {
	// Ensure that main.main is called from the main thread.
	runtime.LockOSThread()
}

// Hint is used to configure window sizing and resizing behaviour.
type Hint int

const (
	// HintNone sets width and height to the default size.
	HintNone Hint = iota

	// HintMin sets the minimum bounds.
	HintMin

	// HintMax sets the maximum bounds.
	HintMax

	// HintFixed prevents the window size from being changed by the user.
	HintFixed
)

// WebView is a webview instance and its native window.
type WebView interface {
	// Run runs the main loop until it's terminated. After this function exits -
	// you must destroy the webview.
	Run()

	// Terminate stops the main loop. It is safe to call this function from
	// a background thread.
	Terminate()

	// Dispatch posts a function to be executed on the main thread. You normally
	// do not need to call this function, unless you want to tweak the native
	// window.
	Dispatch(f func())

	// Destroy destroys a webview and closes the native window. It blocks until
	// any in-flight bound-callback goroutines have finished so none of them call
	// into the native handle after it is freed. Calling Destroy more than once
	// is a no-op.
	Destroy()

	// Window returns a native window handle pointer. When using GTK backend the
	// pointer is GtkWindow pointer, when using Cocoa backend the pointer is
	// NSWindow pointer, when using Win32 backend the pointer is HWND pointer.
	Window() unsafe.Pointer

	// SetTitle updates the title of the native window. Must be called from the UI
	// thread.
	SetTitle(title string)

	// SetSize updates the native window size. See Hint constants.
	SetSize(w, h int, hint Hint)

	// Navigate navigates webview to the given URL. URL may be a properly encoded
	// data URI. Examples:
	//	w.Navigate("https://github.com/webview/webview")
	//	w.Navigate("data:text/html,%3Ch1%3EHello%3C%2Fh1%3E")
	//	w.Navigate("data:text/html;base64,PGgxPkhlbGxvPC9oMT4=")
	Navigate(url string)

	// SetHtml sets the webview HTML directly.
	// Example: w.SetHtml("<h1>Hello</h1>")
	SetHtml(html string)

	// Init injects JavaScript code at the initialization of the new page. Every
	// time the webview will open a new page this initialization code will be
	// executed. It is guaranteed that the code is executed before window.onload.
	Init(js string)

	// Eval evaluates arbitrary JavaScript code. Evaluation happens
	// asynchronously and the result of the expression is ignored. Use RPC
	// bindings if you want to receive notifications about the results of the
	// evaluation.
	Eval(js string)

	// Bind binds a callback function so that it will appear under the given name
	// as a global JavaScript function. Internally it uses webview_init(). The
	// request string is a JSON array of all the arguments passed to the
	// JavaScript function.
	//
	// f must be a function and must return either a value, a value and an error,
	// just an error, or nothing.
	//
	// Bound callbacks are invoked on independent goroutines and may run
	// concurrently with each other. User code that touches shared state from a
	// bound callback must synchronize that access itself.
	Bind(name string, f any) error

	// Unbind removes a callback that was previously set by Bind.
	Unbind(name string) error
}

// New calls NewWindow to create a new window and a new webview instance. If
// debug is true developer tools will be enabled (if the platform supports them).
func New(debug bool) (WebView, error) { return NewWindow(debug, nil) }

// NewWindow creates a new webview instance. If debug is true developer tools
// will be enabled (if the platform supports them). The window parameter can be a
// pointer to the native window handle. If it's non-nil then the child WebView is
// embedded into the given parent window, otherwise a new window is created.
// Depending on the platform, a GtkWindow, NSWindow or HWND pointer can be passed
// here. It returns an error if the native library cannot be loaded or the window
// cannot be created.
func NewWindow(debug bool, window unsafe.Pointer) (WebView, error) {
	loadOnce.Do(func() { errLoad = loadLibraryAndSymbols() })
	if errLoad != nil {
		return nil, errLoad
	}

	r1, _, _ := purego.SyscallN(pCreate, boolToInt(debug), uintptr(window))
	if r1 == 0 {
		return nil, errors.New("webview: failed to create window")
	}
	return &webview{handle: r1, alive: true, mu: sync.Mutex{}, wg: sync.WaitGroup{}}, nil
}

// webview is the concrete implementation of WebView using native library calls.
type webview struct {
	handle uintptr

	mu    sync.Mutex     // guards alive and serializes Destroy against pReturn
	alive bool           // false once Destroy has begun
	wg    sync.WaitGroup // tracks in-flight bound-callback goroutines
}

func (w *webview) Run() {
	purego.SyscallN(pRun, w.handle)
}

func (w *webview) Terminate() {
	// On Windows, we need to dispatch the terminate call to the main thread.
	// Remove once this is merged: https://github.com/webview/webview/pull/1240
	if runtime.GOOS == "windows" {
		w.Dispatch(func() { purego.SyscallN(pTerminate, w.handle) })
		return
	}
	purego.SyscallN(pTerminate, w.handle)
}

func (w *webview) Destroy() {
	w.mu.Lock()
	if !w.alive {
		w.mu.Unlock()
		return
	}
	w.alive = false
	w.mu.Unlock()

	// Drain in-flight bound-callback goroutines before freeing the handle so
	// none of them calls webview_return on a destroyed webview (use-after-free).
	w.wg.Wait()

	purego.SyscallN(pDestroy, w.handle)

	// Drop this instance's bookkeeping so a destroyed window doesn't leak its
	// binding/dispatch entries.
	bindMu.Lock()
	for _, key := range boundNames[w.handle] {
		delete(bindingMap, key)
	}
	delete(boundNames, w.handle)
	bindMu.Unlock()

	dispatchMu.Lock()
	for k, de := range dispatchMap {
		if de.w == w {
			delete(dispatchMap, k)
		}
	}
	dispatchMu.Unlock()
}

func (w *webview) Window() unsafe.Pointer {
	r1, _, _ := purego.SyscallN(pGetWindow, w.handle)
	// r1 is a native window handle, not a Go pointer. Taking the address and
	// dereferencing keeps go vet from flagging a uintptr->unsafe.Pointer misuse.
	return *(*unsafe.Pointer)(unsafe.Pointer(&r1))
}

func (w *webview) SetTitle(title string) {
	cs, ptr := cString(title)
	purego.SyscallN(pSetTitle, w.handle, ptr)
	runtime.KeepAlive(cs)
}

func (w *webview) SetSize(width, height int, hint Hint) {
	purego.SyscallN(pSetSize, w.handle, uintptr(width), uintptr(height), uintptr(hint))
}

func (w *webview) Navigate(url string) {
	cs, ptr := cString(url)
	purego.SyscallN(pNavigate, w.handle, ptr)
	runtime.KeepAlive(cs)
}

func (w *webview) SetHtml(html string) {
	cs, ptr := cString(html)
	purego.SyscallN(pSetHtml, w.handle, ptr)
	runtime.KeepAlive(cs)
}

func (w *webview) Init(js string) {
	cs, ptr := cString(js)
	purego.SyscallN(pInit, w.handle, ptr)
	runtime.KeepAlive(cs)
}

func (w *webview) Eval(js string) {
	cs, ptr := cString(js)
	purego.SyscallN(pEval, w.handle, ptr)
	runtime.KeepAlive(cs)
}
