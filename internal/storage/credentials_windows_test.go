//go:build windows

package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/w0rxbend/twi/internal/auth"
)

func TestNewDefaultCredentialStoreUsesWindowsCredentialManager(t *testing.T) {
	store, err := NewDefaultCredentialStore()
	if err != nil {
		t.Fatalf("NewDefaultCredentialStore returned error: %v", err)
	}
	winStore, ok := store.(*WindowsCredentialManagerStore)
	if !ok {
		t.Fatalf("default credential store = %T, want *WindowsCredentialManagerStore", store)
	}
	if winStore.TargetName() != WindowsCredentialTargetName {
		t.Fatalf("target name = %q, want %q", winStore.TargetName(), WindowsCredentialTargetName)
	}
	if _, ok := store.(interface{ Path() string }); ok {
		t.Fatal("Windows default store exposed a credential-file path")
	}
}

func TestWindowsCredentialManagerConstants(t *testing.T) {
	if WindowsCredentialTargetName != "w0rxbend/twi/twitch-oauth" {
		t.Fatalf("target name = %q", WindowsCredentialTargetName)
	}
	if WindowsCredentialTypeGeneric != 1 {
		t.Fatalf("CRED_TYPE_GENERIC = %d, want 1", WindowsCredentialTypeGeneric)
	}
	if WindowsCredentialPersistLocalMachine != 2 {
		t.Fatalf("CRED_PERSIST_LOCAL_MACHINE = %d, want 2", WindowsCredentialPersistLocalMachine)
	}
}

func TestWindowsCredentialManagerSaveLoadDelete(t *testing.T) {
	provider := newFakeWindowsCredentialProvider()
	store := newWindowsCredentialManagerStoreForTest(t, "w0rxbend/twi/test/save-load-delete", provider)
	record := credentialFixture()

	if err := store.SaveCredentials(context.Background(), record); err != nil {
		t.Fatalf("SaveCredentials returned error: %v", err)
	}
	if len(provider.writes) != 1 {
		t.Fatalf("provider writes = %d, want 1", len(provider.writes))
	}
	write := provider.writes[0]
	if write.TargetName != store.TargetName() {
		t.Fatalf("write target = %q, want %q", write.TargetName, store.TargetName())
	}
	if write.Type != WindowsCredentialTypeGeneric {
		t.Fatalf("write type = %d, want CRED_TYPE_GENERIC", write.Type)
	}
	if write.Persist != WindowsCredentialPersistLocalMachine {
		t.Fatalf("write persist = %d, want CRED_PERSIST_LOCAL_MACHINE", write.Persist)
	}

	loaded, ok, err := store.LoadCredentials(context.Background())
	if err != nil {
		t.Fatalf("LoadCredentials returned error: %v", err)
	}
	if !ok {
		t.Fatal("LoadCredentials ok = false, want true")
	}
	if loaded.AccessToken.Reveal() != record.AccessToken.Reveal() || loaded.RefreshToken.Reveal() != record.RefreshToken.Reveal() {
		t.Fatalf("loaded tokens = (%q, %q), want saved tokens", loaded.AccessToken.Reveal(), loaded.RefreshToken.Reveal())
	}
	if !reflect.DeepEqual(loaded.Scopes, record.Scopes) {
		t.Fatalf("loaded scopes = %#v, want %#v", loaded.Scopes, record.Scopes)
	}

	if err := store.DeleteCredentials(context.Background()); err != nil {
		t.Fatalf("DeleteCredentials returned error: %v", err)
	}
	if _, ok := provider.records[store.TargetName()]; ok {
		t.Fatal("provider retained credential after delete")
	}
	_, ok, err = store.LoadCredentials(context.Background())
	if err != nil {
		t.Fatalf("LoadCredentials after delete returned error: %v", err)
	}
	if ok {
		t.Fatal("LoadCredentials after delete ok = true, want missing")
	}
}

func TestWindowsCredentialManagerMissingCredential(t *testing.T) {
	store := newWindowsCredentialManagerStoreForTest(t, "w0rxbend/twi/test/missing", newFakeWindowsCredentialProvider())

	record, ok, err := store.LoadCredentials(context.Background())
	if err != nil {
		t.Fatalf("LoadCredentials returned error: %v", err)
	}
	if ok {
		t.Fatalf("LoadCredentials ok = true with record %#v, want missing", record)
	}
	if err := store.DeleteCredentials(context.Background()); err != nil {
		t.Fatalf("DeleteCredentials missing credential returned error: %v", err)
	}
}

func TestWindowsCredentialManagerOverwriteAndRefreshTokenPersistence(t *testing.T) {
	provider := newFakeWindowsCredentialProvider()
	store := newWindowsCredentialManagerStoreForTest(t, "w0rxbend/twi/test/overwrite", provider)

	oldRecord := credentialFixture()
	oldRecord.AccessToken = auth.NewSecret("oauth:old-access-secret")
	oldRecord.RefreshToken = auth.NewSecret("old-refresh-secret")
	if err := store.SaveCredentials(context.Background(), oldRecord); err != nil {
		t.Fatalf("initial SaveCredentials returned error: %v", err)
	}

	newRecord := credentialFixture()
	newRecord.AccessToken = auth.NewSecret("oauth:new-access-secret")
	newRecord.RefreshToken = auth.NewSecret("new-refresh-secret")
	newRecord.UpdatedAt = newRecord.UpdatedAt.Add(time.Minute)
	if err := store.SaveCredentials(context.Background(), newRecord); err != nil {
		t.Fatalf("overwrite SaveCredentials returned error: %v", err)
	}

	if len(provider.writes) != 2 {
		t.Fatalf("provider writes = %d, want 2", len(provider.writes))
	}
	blob := string(provider.records[store.TargetName()].Blob)
	if strings.Contains(blob, "oauth:old-access-secret") || strings.Contains(blob, "old-refresh-secret") {
		t.Fatalf("overwritten credential blob retained old tokens: %s", blob)
	}
	if !strings.Contains(blob, "oauth:new-access-secret") || !strings.Contains(blob, "new-refresh-secret") {
		t.Fatalf("overwritten credential blob missing new tokens: %s", blob)
	}

	loaded, ok, err := store.LoadCredentials(context.Background())
	if err != nil {
		t.Fatalf("LoadCredentials returned error: %v", err)
	}
	if !ok {
		t.Fatal("LoadCredentials ok = false, want true")
	}
	if loaded.RefreshToken.Reveal() != "new-refresh-secret" {
		t.Fatalf("loaded refresh token = %q, want new-refresh-secret", loaded.RefreshToken.Reveal())
	}
}

func TestWindowsCredentialManagerProviderFailuresAreRedacted(t *testing.T) {
	record := credentialFixture()
	record.AccessToken = auth.NewSecret("oauth:access-secret")
	record.RefreshToken = auth.NewSecret("refresh-secret")

	for _, tc := range []struct {
		name      string
		configure func(*fakeWindowsCredentialProvider)
		run       func(*WindowsCredentialManagerStore) error
		want      string
	}{
		{
			name: "write",
			configure: func(provider *fakeWindowsCredentialProvider) {
				provider.writeErr = errors.New("CredWriteW failed with oauth:access-secret refresh_token=refresh-secret")
			},
			run: func(store *WindowsCredentialManagerStore) error {
				return store.SaveCredentials(context.Background(), record)
			},
			want: "save Windows credential",
		},
		{
			name: "read",
			configure: func(provider *fakeWindowsCredentialProvider) {
				provider.readErr = errors.New("CredReadW failed with oauth:access-secret refresh_token=refresh-secret")
			},
			run: func(store *WindowsCredentialManagerStore) error {
				_, _, err := store.LoadCredentials(context.Background())
				return err
			},
			want: "load Windows credential",
		},
		{
			name: "delete",
			configure: func(provider *fakeWindowsCredentialProvider) {
				provider.deleteErr = errors.New("CredDeleteW failed with oauth:access-secret refresh_token=refresh-secret")
			},
			run: func(store *WindowsCredentialManagerStore) error {
				return store.DeleteCredentials(context.Background())
			},
			want: "delete Windows credential",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := newFakeWindowsCredentialProvider()
			tc.configure(provider)
			store := newWindowsCredentialManagerStoreForTest(t, "w0rxbend/twi/test/failure/"+tc.name, provider)

			err := tc.run(store)
			if err == nil {
				t.Fatal("operation returned nil error, want provider failure")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want %q", err.Error(), tc.want)
			}
			assertCredentialErrorDoesNotLeak(t, err, "oauth:access-secret", "access-secret", "refresh-secret")
		})
	}
}

func TestWindowsCredentialManagerMalformedPayloadsAreRedacted(t *testing.T) {
	provider := newFakeWindowsCredentialProvider()
	target := "w0rxbend/twi/test/malformed"
	provider.records[target] = windowsCredentialWriteRequest{
		TargetName: target,
		Type:       WindowsCredentialTypeGeneric,
		Persist:    WindowsCredentialPersistLocalMachine,
		Blob:       []byte(`{"version":1,"twitch":{"access_token":"oauth:access-secret","refresh_token":"refresh-secret","expires_at":"oauth:bad-time"}}`),
	}
	store := newWindowsCredentialManagerStoreForTest(t, target, provider)

	_, _, err := store.LoadCredentials(context.Background())
	if !errors.Is(err, ErrMalformedCredentialFile) {
		t.Fatalf("LoadCredentials malformed error = %v, want ErrMalformedCredentialFile", err)
	}
	assertCredentialErrorDoesNotLeak(t, err, "oauth:access-secret", "access-secret", "refresh-secret", "oauth:bad-time", "bad-time")
}

func TestWindowsCredentialManagerRejectsOversizedPayloadsBeforeProvider(t *testing.T) {
	provider := newFakeWindowsCredentialProvider()
	target := "w0rxbend/twi/test/oversized"
	store := newWindowsCredentialManagerStoreForTest(t, target, provider)
	record := credentialFixture()
	for len(mustMarshalCredentialFileForWindowsTest(t, record)) <= maxWindowsCredentialBlobBytes {
		record.Scopes = append(record.Scopes, auth.Scope("fixture:scope:"+strings.Repeat("x", 128)))
	}

	err := store.SaveCredentials(context.Background(), record)
	if !errors.Is(err, ErrMalformedCredentialFile) {
		t.Fatalf("SaveCredentials oversized error = %v, want ErrMalformedCredentialFile", err)
	}
	if len(provider.writes) != 0 {
		t.Fatalf("provider writes = %d, want no native write attempt", len(provider.writes))
	}

	provider.records[target] = windowsCredentialWriteRequest{
		TargetName: target,
		Type:       WindowsCredentialTypeGeneric,
		Persist:    WindowsCredentialPersistLocalMachine,
		Blob:       make([]byte, maxWindowsCredentialBlobBytes+1),
	}
	_, _, err = store.LoadCredentials(context.Background())
	if !errors.Is(err, ErrMalformedCredentialFile) {
		t.Fatalf("LoadCredentials oversized error = %v, want ErrMalformedCredentialFile", err)
	}
}

func TestWindowsCredentialManagerNativeSmoke(t *testing.T) {
	if os.Getenv("TWI_WINDOWS_CREDENTIAL_MANAGER_SMOKE") != "1" {
		t.Skip("set TWI_WINDOWS_CREDENTIAL_MANAGER_SMOKE=1 on a Windows host to run the native WinCred smoke")
	}
	target := fmt.Sprintf("w0rxbend/twi/test/smoke/%d", time.Now().UnixNano())
	store := newWindowsCredentialManagerStoreForTest(t, target, nativeWindowsCredentialProvider{})
	t.Cleanup(func() {
		_ = store.DeleteCredentials(context.Background())
	})

	record := credentialFixture()
	record.Login = "wincred_smoke_viewer"
	record.AccessToken = auth.NewSecret("oauth:fake-wincred-smoke-access")
	record.RefreshToken = auth.NewSecret("fake-wincred-smoke-refresh")

	if err := store.SaveCredentials(context.Background(), record); err != nil {
		t.Fatalf("native smoke SaveCredentials returned error: %v", err)
	}
	loaded, ok, err := store.LoadCredentials(context.Background())
	if err != nil {
		t.Fatalf("native smoke LoadCredentials returned error: %v", err)
	}
	if !ok {
		t.Fatal("native smoke LoadCredentials ok = false, want true")
	}
	if loaded.AccessToken.Reveal() != record.AccessToken.Reveal() || loaded.RefreshToken.Reveal() != record.RefreshToken.Reveal() {
		t.Fatal("native smoke loaded credentials did not round-trip")
	}
	if err := store.DeleteCredentials(context.Background()); err != nil {
		t.Fatalf("native smoke DeleteCredentials returned error: %v", err)
	}
	if _, ok, err := store.LoadCredentials(context.Background()); err != nil || ok {
		t.Fatalf("native smoke after delete = ok %v err %v, want missing without error", ok, err)
	}
}

func newWindowsCredentialManagerStoreForTest(t *testing.T, target string, provider windowsCredentialProvider) *WindowsCredentialManagerStore {
	t.Helper()
	store, err := newWindowsCredentialManagerStoreWithProvider(target, provider)
	if err != nil {
		t.Fatalf("newWindowsCredentialManagerStoreWithProvider returned error: %v", err)
	}
	return store
}

func mustMarshalCredentialFileForWindowsTest(t *testing.T, record CredentialRecord) []byte {
	t.Helper()
	data, err := MarshalCredentialFile(record)
	if err != nil {
		t.Fatalf("MarshalCredentialFile returned error: %v", err)
	}
	return data
}

type fakeWindowsCredentialProvider struct {
	records   map[string]windowsCredentialWriteRequest
	writes    []windowsCredentialWriteRequest
	deletes   []string
	writeErr  error
	readErr   error
	deleteErr error
}

func newFakeWindowsCredentialProvider() *fakeWindowsCredentialProvider {
	return &fakeWindowsCredentialProvider{records: map[string]windowsCredentialWriteRequest{}}
}

func (p *fakeWindowsCredentialProvider) Write(req windowsCredentialWriteRequest) error {
	if p.writeErr != nil {
		return p.writeErr
	}
	clone := req
	clone.Blob = append([]byte(nil), req.Blob...)
	p.records[req.TargetName] = clone
	p.writes = append(p.writes, clone)
	return nil
}

func (p *fakeWindowsCredentialProvider) Read(targetName string, typ uint32) ([]byte, bool, error) {
	if p.readErr != nil {
		return nil, false, p.readErr
	}
	req, ok := p.records[targetName]
	if !ok || req.Type != typ {
		return nil, false, nil
	}
	return append([]byte(nil), req.Blob...), true, nil
}

func (p *fakeWindowsCredentialProvider) Delete(targetName string, typ uint32) (bool, error) {
	if p.deleteErr != nil {
		return false, p.deleteErr
	}
	req, ok := p.records[targetName]
	if !ok || req.Type != typ {
		return false, nil
	}
	delete(p.records, targetName)
	p.deletes = append(p.deletes, targetName)
	return true, nil
}
