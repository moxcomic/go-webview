//go:build !((darwin || linux || windows) && (amd64 || arm64))

package embedded

import "runtime"

// On platforms without an embedded native library, fail loudly at startup with
// an actionable message instead of the cryptic "undefined: name/lib" compile
// error the per-platform embed files would otherwise produce.
func init() {
	panic("go-webview/embedded: unsupported platform " + runtime.GOOS + "/" + runtime.GOARCH +
		" (supported: darwin, linux, windows on amd64 or arm64)")
}
