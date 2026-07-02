package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w0rxbend/twi/internal/config"
	"github.com/w0rxbend/twi/internal/twitch"
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

	validator := twitch.NewFakeTokenValidator(twitch.FakeTokenValidationOutcome{
		Result: twitch.TokenValidationResult{
			Status:        twitch.TokenValidationMissingScope,
			Identity:      twitch.TokenIdentity{UserID: "42", Login: "viewer", DisplayName: "Viewer"},
			Scopes:        []twitch.TokenScope{twitch.ScopeChatRead},
			MissingScopes: []twitch.TokenScope{twitch.ScopeChatEdit},
		},
	})

	report := DoctorWithOptions(context.Background(), cfg, DoctorOptions{
		Environ:  []string{"TERM=xterm-256color", "COLORTERM=truecolor", "KITTY_WINDOW_ID=1"},
		CacheDir: filepath.Join(t.TempDir(), "cache"),
		ReachabilityProbe: func(context.Context) error {
			return nil
		},
		TokenValidator: validator,
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
	requests := validator.Requests()
	if len(requests) != 1 {
		t.Fatalf("validator requests = %d, want 1", len(requests))
	}
	if requests[0].Username != "viewer" ||
		requests[0].OAuthToken != "oauth:secret-token" ||
		requests[0].RefreshToken != "refresh-secret" ||
		requests[0].ClientID != "client-id" ||
		requests[0].ClientSecret != "client-secret" {
		t.Fatalf("validator request = %#v, want config Twitch credentials", requests[0])
	}
	assertDoctorDoesNotLeak(t, report, "oauth:secret-token", "refresh-secret", "client-secret")
}

func TestDoctorReportsTokenValidationStates(t *testing.T) {
	for _, tt := range []struct {
		name       string
		result     twitch.TokenValidationResult
		wantDetail string
	}{
		{
			name:       "malformed",
			result:     twitch.TokenValidationResult{Status: twitch.TokenValidationMalformed},
			wantDetail: "malformed",
		},
		{
			name: "expired with refresh",
			result: twitch.TokenValidationResult{
				Status:           twitch.TokenValidationExpired,
				RefreshAvailable: true,
			},
			wantDetail: "refresh credentials are available",
		},
		{
			name: "wrong user",
			result: twitch.TokenValidationResult{
				Status:   twitch.TokenValidationWrongUser,
				Identity: twitch.TokenIdentity{Login: "other_viewer"},
			},
			wantDetail: "other_viewer",
		},
		{
			name: "valid wrong user fallback",
			result: twitch.TokenValidationResult{
				Status:   twitch.TokenValidationValid,
				Identity: twitch.TokenIdentity{Login: "other_viewer"},
				Scopes:   twitch.RequiredIRCScopes(),
			},
			wantDetail: "configured username",
		},
		{
			name: "valid",
			result: twitch.TokenValidationResult{
				Status:   twitch.TokenValidationValid,
				Identity: twitch.TokenIdentity{Login: "viewer"},
				Scopes:   twitch.RequiredIRCScopes(),
			},
			wantDetail: "valid with required scopes chat:read, chat:edit",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Default()
			cfg.Path = filepath.Join(t.TempDir(), "missing.toml")
			cfg.Twitch.Username = "viewer"
			cfg.Twitch.OAuthToken = "oauth:secret-token"

			report := DoctorWithOptions(context.Background(), cfg, DoctorOptions{
				Environ:           []string{"TERM=xterm-256color"},
				CacheDir:          filepath.Join(t.TempDir(), "cache"),
				ReachabilityProbe: func(context.Context) error { return nil },
				TokenValidator: twitch.NewFakeTokenValidator(twitch.FakeTokenValidationOutcome{
					Result: tt.result,
				}),
			})

			validation := doctorCheck(t, report, "token validation")
			if !strings.Contains(validation.Detail, tt.wantDetail) {
				t.Fatalf("token validation detail = %q, want it to contain %q", validation.Detail, tt.wantDetail)
			}
		})
	}
}

func TestDoctorRedactsValidatorErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Path = filepath.Join(t.TempDir(), "missing.toml")
	cfg.Twitch.OAuthToken = "oauth:secret-token"
	cfg.Twitch.RefreshToken = "refresh-secret"
	cfg.Twitch.ClientSecret = "client-secret"

	validator := twitch.NewFakeTokenValidator(twitch.FakeTokenValidationOutcome{
		Err: errors.New("Bearer secret-token rejected with client-secret"),
	})

	report := DoctorWithOptions(context.Background(), cfg, DoctorOptions{
		Environ:  []string{"TERM=xterm-256color"},
		CacheDir: filepath.Join(t.TempDir(), "cache"),
		ReachabilityProbe: func(context.Context) error {
			return nil
		},
		TokenValidator: validator,
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
