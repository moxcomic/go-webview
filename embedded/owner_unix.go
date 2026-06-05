//go:build !windows

package embedded

import (
	"os"
	"syscall"
)

// ownedByCurrentUser reports whether fi is owned by the effective user. A file
// owned by someone else in a shared location must not be trusted as our library.
func ownedByCurrentUser(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return int(st.Uid) == os.Geteuid()
}
