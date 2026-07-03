package auth

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSecretFormattingRedactsByDefault(t *testing.T) {
	secret := NewSecret("oauth:secret-token")
	tokenSet := TokenSet{
		AccessToken:  secret,
		RefreshToken: NewSecret("refresh-secret"),
	}

	for _, formatted := range []string{
		fmt.Sprint(secret),
		fmt.Sprintf("%#v", secret),
		fmt.Sprintf("%v", tokenSet),
		fmt.Sprintf("%+v", tokenSet),
		fmt.Sprintf("%#v", tokenSet),
		fmt.Sprintf("%q", tokenSet),
	} {
		for _, raw := range []string{"oauth:secret-token", "secret-token", "refresh-secret"} {
			if strings.Contains(formatted, raw) {
				t.Fatalf("formatted value leaked %q: %s", raw, formatted)
			}
		}
		if !strings.Contains(formatted, RedactedSecret) {
			t.Fatalf("formatted value = %q, want redaction marker", formatted)
		}
	}

	if got := secret.Reveal(); got != "oauth:secret-token" {
		t.Fatalf("Reveal = %q, want raw secret for HTTP adapters and tests", got)
	}
}

func TestSecretStructuredEncodingRedactsByDefault(t *testing.T) {
	tokenSet := TokenSet{
		AccessToken:  NewSecret("oauth:access-secret"),
		RefreshToken: NewSecret("refresh-secret"),
	}

	encoded, err := json.Marshal(tokenSet)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	got := string(encoded)
	for _, raw := range []string{"oauth:access-secret", "access-secret", "refresh-secret"} {
		if strings.Contains(got, raw) {
			t.Fatalf("encoded token set leaked %q: %s", raw, got)
		}
	}
	if strings.Count(got, "redacted") != 2 {
		t.Fatalf("encoded token set = %s, want two redacted secrets", got)
	}
}

func TestRedactorRedactsPatternsAndExplicitSecrets(t *testing.T) {
	redactor := NewRedactor(
		NewSecret("oauth:explicit-access"),
		NewSecret("refresh-explicit"),
		NewSecret("client-secret"),
		NewSecret("state-secret"),
	)
	text := strings.Join([]string{
		"oauth:explicit-access",
		"explicit-access",
		"Bearer bearer-secret",
		"access_token=query-access",
		"refresh_token=refresh-explicit",
		"client_secret=client-secret",
		"state=state-secret",
		"code=callback-code",
		"code_verifier=verifier-secret",
	}, " ")

	got := redactor.Redact(text)
	for _, raw := range []string{
		"oauth:explicit-access",
		"explicit-access",
		"bearer-secret",
		"query-access",
		"refresh-explicit",
		"client-secret",
		"state-secret",
		"callback-code",
		"verifier-secret",
	} {
		if strings.Contains(got, raw) {
			t.Fatalf("redacted text leaked %q: %s", raw, got)
		}
	}
	if strings.Count(got, RedactedSecret) < 8 {
		t.Fatalf("redacted text = %q, want redaction markers", got)
	}
}
