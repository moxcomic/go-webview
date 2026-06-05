# go-webview

> Pure-Go bindings for [webview/webview](https://github.com/webview/webview) — no cgo. The native shared library is loaded at runtime via [purego](https://github.com/ebitengine/purego).

[![Go Reference](https://pkg.go.dev/badge/github.com/moxcomic/go-webview.svg)](https://pkg.go.dev/github.com/moxcomic/go-webview)
[![Test](https://github.com/moxcomic/go-webview/actions/workflows/test.yml/badge.svg)](https://github.com/moxcomic/go-webview/actions/workflows/test.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/moxcomic/go-webview)](https://goreportcard.com/report/github.com/moxcomic/go-webview)

`go-webview` lets you build cross-platform desktop GUIs in Go using the system
webview (WebView2 on Windows, WebKitGTK on Linux, WKWebView on macOS). It wraps
the C [webview/webview](https://github.com/webview/webview) library (v0.12.0)
**without cgo**: every native call goes through
[`github.com/ebitengine/purego`](https://github.com/ebitengine/purego), and the
shared library is resolved and loaded at runtime (`dlopen` on Unix,
`LoadLibrary` on Windows). That means a plain `go build` with `CGO_ENABLED=0`,
no C toolchain, and trivial cross-compilation.

---

## Table of contents

- [Features](#features)
- [Platform support](#platform-support)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Binding Go functions to JavaScript](#binding-go-functions-to-javascript)
- [Going the other way: Go calling JS](#going-the-other-way-go-calling-js)
- [API overview](#api-overview)
- [Threading model](#threading-model)
- [Embedded libraries](#embedded-libraries)
- [Building for Windows](#building-for-windows)
- [FAQ / troubleshooting](#faq--troubleshooting)
- [Acknowledgements](#acknowledgements)
- [License](#license)

---

## Features

- **No cgo.** Builds with `CGO_ENABLED=0`; no C compiler required.
- **Runtime loading.** The native library is `dlopen`/`LoadLibrary`-loaded the
  first time you create a webview, so the binary stays portable.
- **Embeddable native libraries.** A single blank import extracts a prebuilt
  copy of the correct `.so`/`.dll`/`.dylib` for the host platform at startup —
  no separate distribution step.
- **Cross-platform.** macOS, Linux and Windows, on both `amd64` and `arm64`.
- **Simple, idiomatic API.** A small `WebView` interface mirroring the upstream
  C API.
- **Two-way JS bridge.** `Bind` exposes Go functions as global JavaScript
  functions; arguments and return values are marshalled as JSON.
- **Safe lifecycle.** `New`/`NewWindow` return an error instead of panicking,
  and `Destroy` drains in-flight bound-callback goroutines before freeing the
  native handle.

## Platform support

Prebuilt native libraries (webview/webview **v0.12.0**) are committed under
[`embedded/<os>_<arch>/`](./embedded) and embedded into your binary when you
import the [`embedded`](#embedded-libraries) subpackage.

| OS        | amd64 | arm64 | Native backend         | Embedded artifact     |
| --------- | :---: | :---: | ---------------------- | --------------------- |
| macOS     |  ✅   |  ✅   | WKWebView (Cocoa)      | `libwebview.dylib`    |
| Linux     |  ✅   |  ✅   | WebKitGTK (GTK)        | `libwebview.so`       |
| Windows   |  ✅   |  ✅   | WebView2 (Win32)       | `webview.dll`         |

**Linux runtime requirements:** GTK 3 and WebKitGTK 4.1 must be present on the
target machine. On Debian/Ubuntu:

```sh
sudo apt-get install -y libgtk-3-0 libwebkit2gtk-4.1-0
```

**Windows runtime requirements:** the
[Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/),
which ships with current versions of Windows 10/11.

**macOS** uses the system WebKit and needs no extra runtime dependency.

## Installation

```sh
go get github.com/moxcomic/go-webview
```

Requires Go 1.21 or newer.

## Quick start

The native library has to be available at runtime. The simplest way is to blank
import the `embedded` subpackage, which carries a prebuilt copy for the host
platform and extracts it on startup (see [Embedded libraries](#embedded-libraries)).

```go
package main

import (
	"github.com/moxcomic/go-webview"
	_ "github.com/moxcomic/go-webview/embedded"
)

func main() {
	w, err := webview.New(true) // debug = true enables dev tools where supported
	if err != nil {
		panic(err)
	}
	w.SetTitle("Basic Example")
	w.SetSize(480, 320, webview.HintNone)
	w.SetHtml("Thanks for using webview!")
	w.Run()     // blocks the main goroutine until the window is closed
	w.Destroy() // free the native window
}
```

> **Note:** `New` and `NewWindow` return an `error` — always check it. A nil
> error means the native library loaded and the window was created.

A few things worth knowing about the flow above:

- **`Run` blocks.** It drives the native event loop on the current (main)
  goroutine and only returns once the window closes (or you call `Terminate`).
- **Always `Destroy`.** It frees the native window. It also blocks until any
  in-flight bound callbacks have finished, so it's safe to free the handle.
- **Threading matters.** The native UI toolkit is single-threaded, so `New`
  must run on the main goroutine — the package pins it for you with
  `runtime.LockOSThread` in `init`.

You can load a remote URL or a data URI instead of inline HTML:

```go
w.Navigate("https://github.com/webview/webview")
w.Navigate("data:text/html,%3Ch1%3EHello%3C%2Fh1%3E")
```

The minimal window above lives in [`./examples/simple`](./examples/simple).

## Binding Go functions to JavaScript

`Bind` exposes a Go function as a global JavaScript function. The JS call
returns a `Promise` that resolves with the JSON-encoded result. Arguments from
JavaScript are decoded into the Go function's parameters.

```go
package main

import (
	"sync/atomic"
	"time"

	"github.com/moxcomic/go-webview"
	_ "github.com/moxcomic/go-webview/embedded"
)

const html = `
<button id="inc">+</button>
<span id="out">0</span>
<script type="module">
  document.getElementById("inc").addEventListener("click", async () => {
    document.getElementById("out").textContent = await window.count(1);
  });
</script>
`

func main() {
	// Bound callbacks run on independent goroutines and may execute
	// concurrently, so shared state must be synchronized — here via atomic.
	var count atomic.Int64

	w, err := webview.New(true)
	if err != nil {
		panic(err)
	}
	defer w.Destroy()
	w.SetTitle("Bind Example")
	w.SetSize(480, 320, webview.HintNone)
	w.SetHtml(html)

	// Returns immediately with the new counter value.
	if err := w.Bind("count", func(delta int64) int64 {
		return count.Add(delta)
	}); err != nil {
		panic(err)
	}

	// A slow callback does not block the UI thread.
	if err := w.Bind("compute", func(a, b int) int {
		time.Sleep(time.Second)
		return a * b
	}); err != nil {
		panic(err)
	}

	w.Run()
}
```

From the page, `await window.count(1)` resolves to the Go return value, and
`await window.compute(6, 7)` resolves to `42` a second later — without ever
blocking the UI thread. See the full runnable version (increment/decrement plus
an async compute button) in [`./examples/bind`](./examples/bind).

### Accepted callback signatures

The function passed to `Bind` must be a function and must return one of:

| Return signature        | Sent to JavaScript                                            |
| ----------------------- | ------------------------------------------------------------ |
| `func(...) T`           | `T` marshalled as JSON (resolves the promise)                |
| `func(...) (T, error)`  | `T` as JSON, or the error message rejects the promise        |
| `func(...) error`       | resolves with `null`, or rejects with the error message      |
| `func(...)`             | resolves with `null`                                          |

If the second return value is present it **must** implement `error`. A function
that returns more than two values, or a non-`error` second value, is rejected by
`Bind` with an error. A bound function that panics is recovered and surfaced to
JavaScript as a rejected promise rather than crashing the process. Use
`Unbind(name)` to remove a binding (and note that binding the same name twice on
one window returns an error).

## Going the other way: Go calling JS

`Eval` runs arbitrary JavaScript asynchronously (the result is ignored — use
`Bind` if you need a value back), and `Init` injects code that runs on every
page load, before `window.onload`:

```go
w.Init(`console.log("runs on every navigation")`)
w.Eval(`document.body.style.background = "rebeccapurple"`)
```

To touch the UI from another goroutine, post work back onto the UI thread with
`Dispatch`:

```go
go func() {
	w.Dispatch(func() {
		w.Eval(`alert("hello from a background goroutine")`)
	})
}()
```

## API overview

```go
func New(debug bool) (WebView, error)
func NewWindow(debug bool, window unsafe.Pointer) (WebView, error)

type Hint int

const (
	HintNone  Hint = iota // default size
	HintMin               // minimum bounds
	HintMax               // maximum bounds
	HintFixed             // window size cannot be changed by the user
)

type WebView interface {
	Run()                          // run the event loop (blocks)
	Terminate()                    // stop the event loop (safe from any goroutine)
	Dispatch(f func())             // run f on the main/UI thread
	Destroy()                      // close the window and free the handle
	Window() unsafe.Pointer        // native window handle (NSWindow/GtkWindow/HWND)
	SetTitle(title string)
	SetSize(w, h int, hint Hint)
	Navigate(url string)
	SetHtml(html string)
	Init(js string)                // JS run before window.onload on every page
	Eval(js string)                // evaluate JS asynchronously, result ignored
	Bind(name string, f any) error
	Unbind(name string) error
}
```

`NewWindow` accepts an optional native window handle; pass `nil` to create a new
top-level window (which is exactly what `New` does), or a
`GtkWindow`/`NSWindow`/`HWND` pointer to embed the webview into an existing
window. Full docs are on
[pkg.go.dev](https://pkg.go.dev/github.com/moxcomic/go-webview).

## Threading model

The native UI toolkit (Cocoa, GTK, Win32) is **single-threaded**, and that
constraint shapes how you use this library.

- **`New`/`NewWindow` must run on the main goroutine.** The package calls
  `runtime.LockOSThread()` in its `init`, pinning `main` to the main OS thread
  so the native toolkit is created on the thread it requires. In practice:
  create the webview from `main`, and don't move that work onto another
  goroutine.
- **`Run` blocks.** It runs the event loop on the calling (main) goroutine and
  returns only when the loop is terminated. After it returns, call `Destroy`.
- **UI mutators run on the UI thread.** Methods such as `SetTitle`, `SetSize`,
  `Navigate`, `SetHtml`, `Init` and `Eval` are intended to be called from the
  UI thread. To touch the window from another goroutine, post work with
  `Dispatch(func(){ ... })`, which runs `f` on the main thread.
- **`Terminate` is goroutine-safe.** You can call it from any goroutine to stop
  the loop.
- **Bound callbacks are concurrent.** Functions registered with `Bind` are
  invoked on **independent goroutines** and may run **concurrently** with one
  another, so a slow callback never blocks the UI. Any shared state your
  callbacks touch must be synchronized by your code (the example above uses
  `sync/atomic`; a `sync.Mutex` works too).
- **`Destroy` is safe.** It marks the instance dead, waits for in-flight
  bound-callback goroutines to finish (so none calls into a freed handle), then
  destroys the native window. Calling `Destroy` more than once is a no-op.

## Embedded libraries

The native library must be resolvable at runtime. There are two supported ways
to ship it.

### Option 1 — embed and auto-extract (recommended)

Blank-import the `embedded` subpackage. It bundles a prebuilt copy of the
correct native library for the host `GOOS`/`GOARCH` and, during package `init`,
extracts it into a **per-user cache directory** (from `os.UserCacheDir()`, under
`go-webview/<version>/`) using restrictive `0600` permissions, then points the
loader at that directory via the `WEBVIEW_PATH` environment variable.

```go
import (
	"github.com/moxcomic/go-webview"
	_ "github.com/moxcomic/go-webview/embedded"
)
```

Extraction is atomic and idempotent: a previously extracted, current-user-owned
copy of the right size is reused; otherwise it is rewritten via a temp file +
rename so concurrent processes never observe a partially written library.

If extraction fails, the error is recorded in `embedded.Err` rather than
panicking. `webview.New` will then fail to load the library, so you can inspect
the root cause:

```go
import (
	"log"

	"github.com/moxcomic/go-webview"
	embedded "github.com/moxcomic/go-webview/embedded"
)

func main() {
	if embedded.Err != nil {
		log.Fatalf("embedded native library not available: %v", embedded.Err)
	}

	w, err := webview.New(true)
	if err != nil {
		log.Fatal(err)
	}
	defer w.Destroy()
	// ...
}
```

### Option 2 — ship the library next to your executable

If you'd rather not embed the binary, place the appropriate native library in
the same directory as your executable:

| Platform | File               |
| -------- | ------------------ |
| Linux    | `libwebview.so`    |
| macOS    | `libwebview.dylib` |
| Windows  | `webview.dll`      |

You can copy these from the [`embedded/<os>_<arch>/`](./embedded) directories of
this repository, or build them from upstream
[webview/webview](https://github.com/webview/webview).

**macOS `.app` bundles:** place `libwebview.dylib` in the bundle's
`Contents/Frameworks` folder (e.g. `YourApp.app/Contents/Frameworks/`) — the
loader looks in `../Frameworks` relative to the executable in addition to the
executable's own directory.

### How the loader finds the library

The first time you create a webview, the loader searches, in order:

1. the directory named by the `WEBVIEW_PATH` environment variable (if set and
   non-empty — the `embedded` package sets this for you);
2. the directory containing the running executable (plus `../Frameworks` on
   macOS);
3. the platform's standard library search path (so a system-installed `webview`
   library is found via `dlopen`/`LoadLibrary`).

The current working directory is deliberately **not** part of the search path.
On Windows the loader opts into the safe DLL search order and, when given an
absolute path, loads with an altered search path so the library's own
dependencies resolve from its directory.

## Building for Windows

By default a Windows Go binary opens a console window. For a GUI application,
build with the `windowsgui` subsystem flag:

```sh
go build -ldflags="-H windowsgui" .
```

Because there is no cgo, you can cross-compile from any host:

```sh
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" .
```

The end-user machine needs the
[WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)
installed (bundled with current Windows). Ship `webview.dll` alongside your
`.exe`, or use the `embedded` subpackage.

## FAQ / troubleshooting

**`webview: failed to load native library` on startup.**
The loader could not find or open the native library. Either blank-import
`github.com/moxcomic/go-webview/embedded`, or place the right
`.so`/`.dll`/`.dylib` next to your executable. See
[Embedded libraries](#embedded-libraries). If you embed and still see this,
check `embedded.Err` for the underlying extraction failure.

**What does `WEBVIEW_PATH` do?**
It names the directory the loader checks **first** for the native library. The
`embedded` package sets it automatically to the per-user cache directory it
extracts into. You can also set it yourself to point at a directory that
contains `libwebview.so` / `libwebview.dylib` / `webview.dll` — for example to
test a locally built native library. An empty `WEBVIEW_PATH` is ignored (it does
**not** fall back to the current working directory).

**Linux: error loading shared libraries / `webkit2gtk` not found.**
The GTK 3 and WebKitGTK 4.1 runtime libraries are missing. Install them:
`sudo apt-get install -y libgtk-3-0 libwebkit2gtk-4.1-0`.

**Windows: the window never appears / errors mentioning WebView2.**
Install the [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/).

**The program panics or the UI hangs when I call from a goroutine.**
Create the webview from `main` and don't move UI calls onto other goroutines —
use `Dispatch` to run code on the UI thread, and remember bound callbacks run
concurrently (synchronize shared state). See [Threading model](#threading-model).

**My `Bind` call returns an error.**
The bound value must be a function returning a value, value+error, just an
error, or nothing; a second return value must implement `error`; and binding the
same name twice on one window fails. See
[Accepted callback signatures](#accepted-callback-signatures).

**A console window pops up on Windows.**
Build with `-ldflags="-H windowsgui"`. See [Building for Windows](#building-for-windows).

## Acknowledgements

This project stands on the work of others:

- [**webview/webview**](https://github.com/webview/webview) — the C library these
  bindings wrap (v0.12.0).
- [**ebitengine/purego**](https://github.com/ebitengine/purego) — calling C from
  Go without cgo, which makes the no-cgo runtime loading possible.
- [**abemedia/go-webview**](https://github.com/abemedia/go-webview) — the
  upstream Go bindings (MIT) that `go-webview` is forked from.

## License

Released under the [MIT License](./LICENSE).

Copyright (c) 2025 Adam Bouqdib (upstream author of
[abemedia/go-webview](https://github.com/abemedia/go-webview)). This fork retains
the original copyright and license.

