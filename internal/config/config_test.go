package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := strings.Join([]string{
		`twitch_username = "file_user"`,
		`twitch_oauth_token = "file_token"`,
		`default_channels = "fileone,filetwo"`,
		`animation_mode = "reduced"`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load([]string{
		"TWI_TWITCH_USERNAME=env_user",
		"TWI_DEFAULT_CHANNELS=envone,envtwo",
	}, Overrides{
		ConfigPath: path,
		Channels:   []string{"cli"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Twitch.Username != "env_user" {
		t.Fatalf("username = %q, want env_user", cfg.Twitch.Username)
	}
	if cfg.Twitch.OAuthToken != "file_token" {
		t.Fatalf("token = %q, want file_token", cfg.Twitch.OAuthToken)
	}
	if !reflect.DeepEqual(cfg.DefaultChannels, []string{"cli"}) {
		t.Fatalf("channels = %#v, want cli override", cfg.DefaultChannels)
	}
	if cfg.Features.AnimationMode != "reduced" {
		t.Fatalf("animation mode = %q, want reduced", cfg.Features.AnimationMode)
	}
}

func TestRedactedStringDoesNotLeakSecrets(t *testing.T) {
	cfg := Default()
	cfg.Twitch.OAuthToken = "oauth:secret"
	cfg.Twitch.ClientSecret = "client-secret"

	output := cfg.RedactedString()

	for _, secret := range []string{"oauth:secret", "client-secret"} {
		if strings.Contains(output, secret) {
			t.Fatalf("redacted output leaked %q: %s", secret, output)
		}
	}
	if strings.Count(output, redacted) != 2 {
		t.Fatalf("redacted output = %q, want two redactions", output)
	}
}

func TestLoadEnvOnlySkipsConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(path, []byte("not a key value line\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadEnvOnly([]string{"TWI_TWITCH_USERNAME=env_user"}, Overrides{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path != path {
		t.Fatalf("path = %q, want %q", cfg.Path, path)
	}
	if cfg.Twitch.Username != "env_user" {
		t.Fatalf("username = %q, want env_user", cfg.Twitch.Username)
	}
}
