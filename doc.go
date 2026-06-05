// Package webview provides Go bindings for the webview/webview library using
// purego, with no cgo. The native library is loaded at runtime via dlopen on
// Unix or LoadLibrary on Windows; import the embedded subpackage to ship and
// extract a prebuilt copy automatically.
//
// Create a window, configure it, optionally bind Go functions into JavaScript,
// then run the event loop:
//
//	w, err := webview.New(true)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer w.Destroy()
//	w.SetTitle("Hello")
//	w.SetSize(800, 600, webview.HintNone)
//	w.Navigate("https://example.com")
//	w.Run()
//
// Threading: the native UI toolkit is single-threaded. New must be called from
// the main goroutine (the package pins it to the main OS thread in init) and Run
// blocks that goroutine running the event loop. Bound callbacks are invoked on
// independent goroutines and may run concurrently, so callback code that shares
// state must synchronize it.
//
// Bind exposes a Go function to JavaScript. It may take JSON-decodable
// parameters and return a value, a value and an error, just an error, or
// nothing; the result is marshaled back to the JavaScript caller as JSON.
//
// To ship the native library, blank-import the embedded subpackage for its side
// effect: it extracts a prebuilt copy at startup and reports any failure via
// embedded.Err.
package webview
