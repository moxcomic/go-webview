//go:build windows

package embedded

import "os"

// ownedByCurrentUser is a no-op on Windows: the cache lives under the per-user
// %LOCALAPPDATA% directory (from os.UserCacheDir), whose ACL already restricts
// access to the current user, so a separate ownership check adds nothing.
func ownedByCurrentUser(fi os.FileInfo) bool { return true }
