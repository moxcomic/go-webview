package webview

import (
	"encoding/json"
	"unsafe"
)

func boolToInt(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// cString returns a NUL-terminated copy of s together with a pointer to its
// first byte. The returned slice must be kept alive (e.g. via runtime.KeepAlive)
// for as long as native code uses the pointer.
func cString(s string) ([]byte, uintptr) {
	b := append([]byte(s), 0)
	return b, uintptr(unsafe.Pointer(&b[0]))
}

// goString copies the NUL-terminated C string at address c into a Go string. It
// returns "" for a nil pointer.
func goString(c uintptr) string {
	// Take the address and dereference it so go vet doesn't flag a possible
	// misuse of unsafe.Pointer on the uintptr->pointer conversion.
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&c))
	if ptr == nil {
		return ""
	}
	var length int
	for *(*byte)(unsafe.Add(ptr, uintptr(length))) != '\x00' {
		length++
	}
	return string(unsafe.Slice((*byte)(ptr), length))
}

// jsonString JSON-encodes s and always returns valid JSON. json.Marshal of a
// string effectively never fails, but fall back to a JSON null rather than
// emitting malformed JSON if it somehow does.
func jsonString(s string) string {
	if data, err := json.Marshal(s); err == nil {
		return string(data)
	}
	return "null"
}
