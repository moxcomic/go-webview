//go:build (darwin || linux || windows) && (amd64 || arm64)

package embedded

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed VERSION.txt
var version string

// Err holds any error encountered while extracting the embedded native library
// during package initialization. When non-nil, the library was not made
// available and webview.New will subsequently fail to load it; callers that want
// to surface the root cause can inspect embedded.Err.
var Err error

func init() {
	Err = extract()
}

// extract writes the embedded native library into a per-user cache directory
// (not a world-shared temp dir) with restrictive permissions, verifies any
// pre-existing copy is trustworthy, and points the loader at it via WEBVIEW_PATH.
func extract() error {
	base, err := os.UserCacheDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(base, "go-webview", version)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	file := filepath.Join(dir, name)
	ok, err := trusted(file)
	if err != nil {
		return err
	}
	if !ok {
		if err := writeAtomic(dir, file, lib); err != nil {
			return err
		}
	}

	return os.Setenv("WEBVIEW_PATH", dir)
}

// trusted reports whether file already exists as a regular, current-user-owned,
// non-symlink file of the expected size. Anything else means we must re-extract
// rather than load a file that another user or process may have planted.
func trusted(file string) (bool, error) {
	fi, err := os.Lstat(file)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	if fi.Size() != int64(len(lib)) {
		return false, nil
	}
	if !ownedByCurrentUser(fi) {
		return false, nil
	}
	return true, nil
}

// writeAtomic writes data to a temp file in dir with 0600 permissions and
// renames it into place, so concurrent processes never observe (or load) a
// partially-written library and there is no create-if-absent TOCTOU window.
func writeAtomic(dir, file string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, file)
}
