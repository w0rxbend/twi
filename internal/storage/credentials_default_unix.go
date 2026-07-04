//go:build unix

package storage

// NewDefaultCredentialStore returns the supported saved-credential backend for
// Unix builds.
func NewDefaultCredentialStore() (CredentialStore, error) {
	return NewDefaultCredentialFileStore()
}
