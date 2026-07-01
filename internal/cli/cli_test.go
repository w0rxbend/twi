package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "twi chat") {
		t.Fatalf("help output missing chat command: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestMockChat(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run([]string{"chat", "--mock", "--channel", "example"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "#example") {
		t.Fatalf("mock chat output missing channel: %q", stdout.String())
	}
}

func TestConfigShowRedactsSecrets(t *testing.T) {
	t.Setenv("TWI_TWITCH_OAUTH_TOKEN", "oauth:secret")
	t.Setenv("TWI_TWITCH_CLIENT_SECRET", "client-secret")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"config", "show", "--config", t.TempDir() + "/missing.toml"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	for _, secret := range []string{"oauth:secret", "client-secret"} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("config output leaked %q: %s", secret, stdout.String())
		}
	}
}

func TestDoctorDoesNotPrintSecrets(t *testing.T) {
	t.Setenv("TWI_TWITCH_OAUTH_TOKEN", "oauth:secret")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"doctor", "--config", t.TempDir() + "/missing.toml"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("Run returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "oauth:secret") {
		t.Fatalf("doctor output leaked token: %s", stdout.String())
	}
}
