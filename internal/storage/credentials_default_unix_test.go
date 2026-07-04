//go:build unix

package storage

import "testing"

func TestNewDefaultCredentialStoreUsesUnixFileFallback(t *testing.T) {
	store, err := NewDefaultCredentialStore()
	if err != nil {
		t.Fatalf("NewDefaultCredentialStore returned error: %v", err)
	}
	fileStore, ok := store.(*CredentialFileStore)
	if !ok {
		t.Fatalf("default credential store = %T, want *CredentialFileStore", store)
	}
	if fileStore.Path() == "" {
		t.Fatal("default credential file store path is empty")
	}
}
