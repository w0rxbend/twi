//go:build windows

package storage

// NewDefaultCredentialStore returns the supported saved-credential backend for
// Windows builds.
func NewDefaultCredentialStore() (CredentialStore, error) {
	return NewWindowsCredentialManagerStore()
}
