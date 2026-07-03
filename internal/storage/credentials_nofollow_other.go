//go:build !unix

package storage

import "os"

// Non-Unix platforms still get Lstat-based stable symlink rejection from the
// shared store path. They do not yet have an atomic no-follow open hook here;
// platform-specific ACL and reparse-point handling is tracked separately.
func openCredentialFileNoFollow(path string) (*os.File, error) {
	return os.Open(path)
}
