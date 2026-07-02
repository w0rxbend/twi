package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w0rxbend/twi/internal/config"
)

func TestDoctorRunsWithoutCredentialsAndUsesWarnings(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	cfg := config.Default()
	cfg.Path = filepath.Join(t.TempDir(), "missing.toml")

	report := DoctorWithOptions(context.Background(), cfg, DoctorOptions{
		Environ:  []string{"TERM=dumb"},
		CacheDir: cacheDir,
		ReachabilityProbe: func(context.Context) error {
			return errors.New("network unavailable")
		},
	})

	for _, name := range []string{"config file", "twitch username", "oauth token", "token validation", "twitch reachability", "terminal", "kitty graphics"} {
		check := doctorCheck(t, report, name)
		if check.Status != DoctorStatusWarn {
			t.Fatalf("%s status = %q, want warn; detail=%q", name, check.Status, check.Detail)
		}
	}
	if check := doctorCheck(t, report, "cache directory"); check.Status != DoctorStatusOK {
		t.Fatalf("cache status = %q, want ok; detail=%q", check.Status, check.Detail)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cache probe left files behind: %#v", entries)
	}
}

func TestDoctorReportsCredentialPresenceAndValidationWithoutSecrets(t *testing.T) {
	cfg := config.Default()
	cfg.Path = filepath.Join(t.TempDir(), "missing.toml")
	cfg.Twitch.Username = "viewer"
	cfg.Twitch.OAuthToken = "oauth:secret-token"
	cfg.Twitch.RefreshToken = "refresh-secret"
	cfg.Twitch.ClientID = "client-id"
	cfg.Twitch.ClientSecret = "client-secret"

	report := DoctorWithOptions(context.Background(), cfg, DoctorOptions{
		Environ:  []string{"TERM=xterm-256color", "COLORTERM=truecolor", "KITTY_WINDOW_ID=1"},
		CacheDir: filepath.Join(t.TempDir(), "cache"),
		ReachabilityProbe: func(context.Context) error {
			return nil
		},
		TokenValidator: func(context.Context, config.TwitchConfig) (TokenValidation, error) {
			return TokenValidation{Valid: true, Scopes: []string{"chat:read"}}, nil
		},
	})

	for _, name := range []string{"twitch username", "oauth token", "refresh token", "client id", "client secret"} {
		check := doctorCheck(t, report, name)
		if check.Status != DoctorStatusOK || check.Detail != "present" {
			t.Fatalf("%s = (%q, %q), want ok present", name, check.Status, check.Detail)
		}
	}
	validation := doctorCheck(t, report, "token validation")
	if validation.Status != DoctorStatusWarn || !strings.Contains(validation.Detail, "chat:edit") {
		t.Fatalf("token validation = (%q, %q), want missing chat:edit warning", validation.Status, validation.Detail)
	}
	assertDoctorDoesNotLeak(t, report, "oauth:secret-token", "refresh-secret", "client-secret")
}

func TestDoctorRedactsValidatorErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Path = filepath.Join(t.TempDir(), "missing.toml")
	cfg.Twitch.OAuthToken = "oauth:secret-token"
	cfg.Twitch.RefreshToken = "refresh-secret"
	cfg.Twitch.ClientSecret = "client-secret"

	report := DoctorWithOptions(context.Background(), cfg, DoctorOptions{
		Environ:  []string{"TERM=xterm-256color"},
		CacheDir: filepath.Join(t.TempDir(), "cache"),
		ReachabilityProbe: func(context.Context) error {
			return nil
		},
		TokenValidator: func(context.Context, config.TwitchConfig) (TokenValidation, error) {
			return TokenValidation{}, errors.New("Bearer secret-token rejected with client-secret")
		},
	})

	validation := doctorCheck(t, report, "token validation")
	if validation.Status != DoctorStatusWarn {
		t.Fatalf("token validation status = %q, want warn", validation.Status)
	}
	if !strings.Contains(validation.Detail, "[redacted]") {
		t.Fatalf("token validation detail = %q, want redaction marker", validation.Detail)
	}
	assertDoctorDoesNotLeak(t, report, "oauth:secret-token", "refresh-secret", "client-secret")
	assertDoctorDoesNotLeak(t, report, "secret-token")
}

func doctorCheck(t *testing.T, report DoctorReport, name string) DoctorCheck {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("doctor report missing check %q: %#v", name, report.Checks)
	return DoctorCheck{}
}

func assertDoctorDoesNotLeak(t *testing.T, report DoctorReport, secrets ...string) {
	t.Helper()
	for _, check := range report.Checks {
		for _, secret := range secrets {
			if strings.Contains(check.Detail, secret) {
				t.Fatalf("%s leaked %q in detail %q", check.Name, secret, check.Detail)
			}
		}
	}
}
