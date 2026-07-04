//go:build windows

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/w0rxbend/twi/internal/app"
	"github.com/w0rxbend/twi/internal/auth"
	"github.com/w0rxbend/twi/internal/config"
	"github.com/w0rxbend/twi/internal/debuglog"
	"github.com/w0rxbend/twi/internal/storage"
	"github.com/w0rxbend/twi/internal/twitch"
)

func init() {
	newCredentialStore = func() (storage.CredentialStore, error) {
		return nil, fmt.Errorf("%w: saved credential persistence is disabled in Windows CLI tests; use environment variables or a private flat config file", storage.ErrUnsupportedCredentialFilePlatform)
	}
}

func TestWindowsConfigShowLoadsCredentialManagerCredentialsAndRedactsTokens(t *testing.T) {
	clearTwitchCredentialEnv(t)
	store := newWindowsCLIStore()
	store.SetCredentials(windowsCLICredentialFixture())

	oldNewCredentialStore := newCredentialStore
	t.Cleanup(func() {
		newCredentialStore = oldNewCredentialStore
	})
	newCredentialStore = func() (storage.CredentialStore, error) {
		return store, nil
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"config", "show", "--config", t.TempDir() + "/missing.toml"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	for _, want := range []string{
		`twitch_username = "win_viewer"`,
		`twitch_oauth_token = "[redacted]"`,
		`twitch_refresh_token = "[redacted]"`,
		`twitch_client_id = "win-client-id"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("config output missing %q:\n%s", want, stdout.String())
		}
	}
	assertOutputDoesNotContain(t, stdout.String()+stderr.String(), "win-access-secret", "win-refresh-secret")
}

func TestWindowsDoctorReportsCredentialManagerStore(t *testing.T) {
	clearTwitchCredentialEnv(t)
	store := newWindowsCLIStore()
	store.SetCredentials(windowsCLICredentialFixture())

	oldNewCredentialStore := newCredentialStore
	oldNewDoctorTokenValidator := newDoctorTokenValidator
	oldDoctorReachabilityProbe := doctorReachabilityProbe
	oldDoctorCacheDir := doctorCacheDir
	t.Cleanup(func() {
		newCredentialStore = oldNewCredentialStore
		newDoctorTokenValidator = oldNewDoctorTokenValidator
		doctorReachabilityProbe = oldDoctorReachabilityProbe
		doctorCacheDir = oldDoctorCacheDir
	})
	newCredentialStore = func() (storage.CredentialStore, error) {
		return store, nil
	}
	fake := twitch.NewFakeTokenValidator(twitch.FakeTokenValidationOutcome{
		Result: twitch.TokenValidationResult{
			Status:   twitch.TokenValidationValid,
			Identity: twitch.TokenIdentity{UserID: "42", Login: "win_viewer"},
			Scopes:   twitch.RequiredIRCScopes(),
		},
	})
	newDoctorTokenValidator = func() twitch.TokenValidator {
		return fake
	}
	doctorReachabilityProbe = func(context.Context) error {
		return nil
	}
	doctorCacheDir = func() string {
		return t.TempDir()
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor", "--config", t.TempDir() + "/missing.toml"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	for _, want := range []string{
		"[ok] Windows Credential Manager:",
		"target " + storage.WindowsCredentialTargetName + " loaded",
		"[ok] twitch username: present",
		"[ok] oauth token: present",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, stdout.String())
		}
	}
	assertOutputDoesNotContain(t, stdout.String()+stderr.String(), "win-access-secret", "win-refresh-secret")
}

func TestWindowsLiveChatUsesCredentialManagerCredentials(t *testing.T) {
	clearTwitchCredentialEnv(t)
	store := newWindowsCLIStore()
	store.SetCredentials(windowsCLICredentialFixture())

	oldNewCredentialStore := newCredentialStore
	oldNewLiveChatClient := newLiveChatClient
	oldRunLiveChat := runLiveChat
	t.Cleanup(func() {
		newCredentialStore = oldNewCredentialStore
		newLiveChatClient = oldNewLiveChatClient
		runLiveChat = oldRunLiveChat
	})
	newCredentialStore = func() (storage.CredentialStore, error) {
		return store, nil
	}

	fake := app.NewFakeChatClient(1)
	newLiveChatClient = func(_ context.Context, cfg config.Config, _ debuglog.Logger, _ credentialLoadStatus) (app.ChatClient, error) {
		if cfg.Twitch.Username != "win_viewer" || cfg.Twitch.OAuthToken != "oauth:win-access-secret" || cfg.Twitch.RefreshToken != "win-refresh-secret" {
			t.Fatalf("live chat credentials = %#v, want Windows stored credentials", cfg.Twitch)
		}
		return fake, nil
	}
	runLiveChat = func(stdout io.Writer, _ config.Config, client app.ChatClient, _ app.ClientOptions) error {
		if client != fake {
			t.Fatalf("runLiveChat client = %#v, want fake", client)
		}
		_, err := stdout.Write([]byte("live shell started\n"))
		return err
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"chat", "--config", t.TempDir() + "/missing.toml", "--channel", "example"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "live shell started") {
		t.Fatalf("stdout missing live shell output: %q", stdout.String())
	}
	assertOutputDoesNotContain(t, stdout.String()+stderr.String(), "win-access-secret", "win-refresh-secret")
}

func TestWindowsLoginSavesToCredentialManagerStore(t *testing.T) {
	t.Setenv("TWI_TWITCH_CLIENT_ID", "win-client-id")
	t.Setenv("TWI_TWITCH_CLIENT_SECRET", "win-client-secret")
	store := newWindowsCLIStore()

	resetLoginTestHooks(t)
	newCredentialStore = func() (storage.CredentialStore, error) {
		return store, nil
	}
	fakeFlow := auth.NewFakeLoginFlow()
	fakeFlow.QueueBegin(auth.LoginChallenge{
		AuthorizationURL: auth.NewSecret("https://auth.example/authorize?client_id=win-client-id&state=state-secret"),
		State:            auth.NewSecret("state-secret"),
		Scopes:           auth.RequiredChatScopes(),
		ExpiresAt:        time.Now().Add(time.Minute),
	}, nil)
	fakeFlow.QueueComplete(auth.LoginResult{
		Identity: auth.Identity{UserID: "42", Login: "win_viewer"},
		Tokens: auth.TokenSet{
			AccessToken:  auth.NewSecret("oauth:win-access-secret"),
			RefreshToken: auth.NewSecret("win-refresh-secret"),
			Scopes:       auth.RequiredChatScopes(),
		},
		Scopes: auth.RequiredChatScopes(),
	}, nil)
	newLoginFlow = func() auth.LoginFlow {
		return fakeFlow
	}
	newLoginCallbackWaiter = func(string) (loginCallbackWaiter, error) {
		return &fakeLoginCallbackWaiter{callback: auth.LoginCallback{
			Code:  auth.NewSecret("callback-code"),
			State: auth.NewSecret("state-secret"),
		}}, nil
	}
	openLoginBrowser = func(context.Context, string) error {
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"login", "--config", t.TempDir() + "/missing.toml"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	saves := store.SavedRecords()
	if len(saves) != 1 {
		t.Fatalf("saved records = %d, want 1", len(saves))
	}
	if saves[0].AccessToken.Reveal() != "oauth:win-access-secret" || saves[0].RefreshToken.Reveal() != "win-refresh-secret" {
		t.Fatal("saved Windows credential tokens did not match login result")
	}
	for _, want := range []string{"Login succeeded for Twitch user: win_viewer", "Credentials saved"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("login output missing %q:\n%s", want, stdout.String())
		}
	}
	assertOutputDoesNotContain(t, stdout.String()+stderr.String(), "win-client-secret", "state-secret", "callback-code", "win-access-secret", "win-refresh-secret", "https://auth.example")
}

type windowsCLICredentialStore struct {
	*storage.MemoryCredentialStore
}

func newWindowsCLIStore() *windowsCLICredentialStore {
	return &windowsCLICredentialStore{MemoryCredentialStore: storage.NewMemoryCredentialStore()}
}

func (s *windowsCLICredentialStore) StoreLabel() string {
	return "Windows Credential Manager"
}

func (s *windowsCLICredentialStore) StoreLocation() string {
	return "target " + storage.WindowsCredentialTargetName
}

func windowsCLICredentialFixture() storage.CredentialRecord {
	return storage.CredentialRecord{
		UserID:       "42",
		Login:        "win_viewer",
		DisplayName:  "Windows Viewer",
		ClientID:     "win-client-id",
		AccessToken:  auth.NewSecret("oauth:win-access-secret"),
		RefreshToken: auth.NewSecret("win-refresh-secret"),
		TokenType:    "bearer",
		Scopes:       auth.RequiredChatScopes(),
		UpdatedAt:    time.Now(),
	}
}
