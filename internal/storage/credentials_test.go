package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/w0rxbend/twi/internal/auth"
)

func TestCredentialRecordFromLoginResultAndRedaction(t *testing.T) {
	updatedAt := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	expiresAt := updatedAt.Add(time.Hour)
	record := CredentialRecordFromLoginResult(auth.LoginResult{
		Identity: auth.Identity{
			UserID:      "42",
			Login:       "viewer",
			DisplayName: "Viewer",
		},
		Tokens: auth.TokenSet{
			AccessToken:  auth.NewSecret("oauth:access-secret"),
			RefreshToken: auth.NewSecret("refresh-secret"),
			TokenType:    "bearer",
			Scopes:       auth.RequiredChatScopes(),
			ExpiresAt:    expiresAt,
		},
	}, "client-id", updatedAt)

	if record.UserID != "42" || record.Login != "viewer" || record.ClientID != "client-id" {
		t.Fatalf("record identity = %#v, want login result fields", record)
	}
	if record.AccessToken.Reveal() != "oauth:access-secret" || record.RefreshToken.Reveal() != "refresh-secret" {
		t.Fatalf("record token fields were not preserved")
	}
	if !reflect.DeepEqual(record.Scopes, auth.RequiredChatScopes()) {
		t.Fatalf("scopes = %#v, want required chat scopes", record.Scopes)
	}

	formatted := fmt.Sprintf("%+v %#v", record, record)
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal record returned error: %v", err)
	}
	for _, output := range []string{formatted, string(encoded)} {
		for _, raw := range []string{"oauth:access-secret", "access-secret", "refresh-secret"} {
			if strings.Contains(output, raw) {
				t.Fatalf("default credential output leaked %q: %s", raw, output)
			}
		}
		if !strings.Contains(output, "redacted") {
			t.Fatalf("default credential output missing redaction marker: %s", output)
		}
	}
}

func TestCredentialFileMarshalIsExplicitRevealPath(t *testing.T) {
	record := credentialFixture()

	data, err := MarshalCredentialFile(record)
	if err != nil {
		t.Fatalf("MarshalCredentialFile returned error: %v", err)
	}
	got := string(data)
	for _, want := range []struct {
		name string
		text string
	}{
		{name: "version", text: `"version": 1`},
		{name: "access token field", text: `"access_token": "oauth:access-secret"`},
		{name: "refresh token field", text: `"refresh_token": "refresh-secret"`},
		{name: "scope list", text: `"scopes": [`},
		{name: "chat read scope", text: `"chat:read"`},
		{name: "chat edit scope", text: `"chat:edit"`},
	} {
		if !strings.Contains(got, want.text) {
			t.Fatalf("credential file missing %s", want.name)
		}
	}
	if strings.Contains(got, "<redacted>") {
		t.Fatal("credential file used redacted token values")
	}

	parsed, err := ParseCredentialFile(data)
	if err != nil {
		t.Fatalf("ParseCredentialFile returned error: %v", err)
	}
	if parsed.AccessToken.Reveal() != record.AccessToken.Reveal() {
		t.Fatal("parsed access token did not round-trip")
	}
	if parsed.RefreshToken.Reveal() != record.RefreshToken.Reveal() {
		t.Fatal("parsed refresh token did not round-trip")
	}
	if !reflect.DeepEqual(parsed.Scopes, record.Scopes) {
		t.Fatalf("parsed scopes = %#v, want %#v", parsed.Scopes, record.Scopes)
	}
}

func TestParseCredentialFileRejectsUnsupportedFormatAndUnknownFields(t *testing.T) {
	if _, err := ParseCredentialFile([]byte(`{"version":2,"twitch":{}}`)); !errors.Is(err, ErrUnsupportedCredentialFileFormat) {
		t.Fatalf("ParseCredentialFile version error = %v, want ErrUnsupportedCredentialFileFormat", err)
	}
	if _, err := ParseCredentialFile([]byte(`{"version":1,"twitch":{},"access_token":"oauth:secret"}`)); err == nil {
		t.Fatal("ParseCredentialFile accepted unknown top-level field")
	} else if strings.Contains(err.Error(), "oauth:secret") {
		t.Fatalf("ParseCredentialFile error leaked unknown field value: %v", err)
	}

	if _, err := ParseCredentialFile([]byte(`{"version":1,"twitch":{"expires_at":"oauth:timestamp-secret"}}`)); err == nil {
		t.Fatal("ParseCredentialFile accepted malformed timestamp")
	} else if strings.Contains(err.Error(), "oauth:timestamp-secret") || strings.Contains(err.Error(), "timestamp-secret") {
		t.Fatal("ParseCredentialFile timestamp error leaked raw value")
	}
}

func TestCredentialFilePlanUsesRestrictiveDefaults(t *testing.T) {
	plan, err := NewCredentialFilePlan(filepath.Join(t.TempDir(), "credentials.json"))
	if err != nil {
		t.Fatalf("NewCredentialFilePlan returned error: %v", err)
	}
	if plan.DirectoryMode != CredentialDirectoryMode {
		t.Fatalf("directory mode = %s, want %s", plan.DirectoryMode, CredentialDirectoryMode)
	}
	if plan.DirectoryMode != fs.FileMode(0o700) {
		t.Fatalf("directory mode = %s, want 0700", plan.DirectoryMode)
	}
	if plan.FileMode != CredentialFileMode {
		t.Fatalf("file mode = %s, want %s", plan.FileMode, CredentialFileMode)
	}
	if plan.FileMode != fs.FileMode(0o600) {
		t.Fatalf("file mode = %s, want 0600", plan.FileMode)
	}
	if plan.FormatVersion != CredentialFileRecordVersion {
		t.Fatalf("format version = %d, want %d", plan.FormatVersion, CredentialFileRecordVersion)
	}
	if plan.Migration != CredentialMigrationExplicitOnly {
		t.Fatalf("migration = %q, want explicit-only", plan.Migration)
	}
}

func TestCredentialPermissionValidationRejectsGroupWorldAndExecutableModes(t *testing.T) {
	if err := ValidateCredentialFileMode(0o600); err != nil {
		t.Fatalf("ValidateCredentialFileMode(0600) returned error: %v", err)
	}
	for _, mode := range []uint32{0o700, 0o640, 0o604, 0o666, 0o777, 0o400, 0o200, 0o000} {
		if err := ValidateCredentialFileMode(fs.FileMode(mode)); !errors.Is(err, ErrInsecureCredentialPermissions) {
			t.Fatalf("ValidateCredentialFileMode(%#o) = %v, want ErrInsecureCredentialPermissions", mode, err)
		}
	}

	if err := ValidateCredentialDirectoryMode(0o700); err != nil {
		t.Fatalf("ValidateCredentialDirectoryMode(0700) returned error: %v", err)
	}
	for _, mode := range []uint32{0o750, 0o705, 0o777, 0o500, 0o300, 0o000} {
		if err := ValidateCredentialDirectoryMode(fs.FileMode(mode)); !errors.Is(err, ErrInsecureCredentialPermissions) {
			t.Fatalf("ValidateCredentialDirectoryMode(%#o) = %v, want ErrInsecureCredentialPermissions", mode, err)
		}
	}
}

func TestCredentialFilePlanValidationRejectsUnsafeModesAndMigration(t *testing.T) {
	plan := CredentialFilePlan{
		Path:          filepath.Join(t.TempDir(), "credentials.json"),
		DirectoryMode: 0o700,
		FileMode:      0o640,
		FormatVersion: CredentialFileRecordVersion,
		Migration:     CredentialMigrationExplicitOnly,
	}
	if err := plan.Validate(); !errors.Is(err, ErrInsecureCredentialPermissions) {
		t.Fatalf("Validate insecure file mode = %v, want ErrInsecureCredentialPermissions", err)
	}

	plan.FileMode = 0o600
	plan.Migration = CredentialMigration("automatic-copy")
	if err := plan.Validate(); err == nil {
		t.Fatal("Validate accepted unsupported migration policy")
	}
}

func TestMemoryCredentialStoreClonesRecordsAndTracksOperations(t *testing.T) {
	store := NewMemoryCredentialStore()
	record := credentialFixture()
	if err := store.SaveCredentials(context.Background(), record); err != nil {
		t.Fatalf("SaveCredentials returned error: %v", err)
	}
	record.Scopes[0] = auth.ScopeChatEdit

	got, ok, err := store.LoadCredentials(context.Background())
	if err != nil {
		t.Fatalf("LoadCredentials returned error: %v", err)
	}
	if !ok {
		t.Fatal("LoadCredentials ok = false, want true")
	}
	if !reflect.DeepEqual(got.Scopes, []auth.Scope{auth.ScopeChatRead, auth.ScopeChatEdit}) {
		t.Fatalf("loaded scopes = %#v, want saved snapshot", got.Scopes)
	}
	got.Scopes[0] = auth.ScopeChatEdit
	again, _, err := store.LoadCredentials(context.Background())
	if err != nil {
		t.Fatalf("LoadCredentials second call returned error: %v", err)
	}
	if again.Scopes[0] != auth.ScopeChatRead {
		t.Fatalf("LoadCredentials returned mutable backing slice: %#v", again.Scopes)
	}

	saves := store.SavedRecords()
	saves[0].Scopes[0] = auth.ScopeChatEdit
	if store.SavedRecords()[0].Scopes[0] != auth.ScopeChatRead {
		t.Fatalf("SavedRecords returned mutable backing slice")
	}

	if err := store.DeleteCredentials(context.Background()); err != nil {
		t.Fatalf("DeleteCredentials returned error: %v", err)
	}
	if store.DeleteCount() != 1 {
		t.Fatalf("DeleteCount = %d, want 1", store.DeleteCount())
	}
	if _, ok, err := store.LoadCredentials(context.Background()); err != nil || ok {
		t.Fatalf("LoadCredentials after delete = ok %v err %v, want miss", ok, err)
	}
}

func TestCredentialStoreFakesHonorCancellationAndConfiguredErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := NewMemoryCredentialStore().LoadCredentials(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("MemoryCredentialStore LoadCredentials canceled error = %v, want context.Canceled", err)
	}
	if err := (FailingCredentialStore{}).SaveCredentials(ctx, CredentialRecord{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("FailingCredentialStore SaveCredentials canceled error = %v, want context.Canceled", err)
	}

	wantErr := errors.New("fixture store failure")
	store := NewMemoryCredentialStore()
	store.SetErrors(wantErr, wantErr, wantErr)
	if _, _, err := store.LoadCredentials(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("LoadCredentials configured error = %v, want %v", err, wantErr)
	}
	if err := store.SaveCredentials(context.Background(), CredentialRecord{}); !errors.Is(err, wantErr) {
		t.Fatalf("SaveCredentials configured error = %v, want %v", err, wantErr)
	}
	if err := store.DeleteCredentials(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("DeleteCredentials configured error = %v, want %v", err, wantErr)
	}

	failing := FailingCredentialStore{Err: wantErr}
	if _, _, err := failing.LoadCredentials(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("FailingCredentialStore LoadCredentials error = %v, want %v", err, wantErr)
	}
}

func credentialFixture() CredentialRecord {
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	return CredentialRecord{
		UserID:       "42",
		Login:        "viewer",
		DisplayName:  "Viewer",
		ClientID:     "client-id",
		AccessToken:  auth.NewSecret("oauth:access-secret"),
		RefreshToken: auth.NewSecret("refresh-secret"),
		TokenType:    "bearer",
		Scopes:       []auth.Scope{auth.ScopeChatRead, auth.ScopeChatEdit},
		ExpiresAt:    now.Add(time.Hour),
		UpdatedAt:    now,
	}
}
