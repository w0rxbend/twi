package twitch

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestTokenValidationResultRepresentsCredentialStates(t *testing.T) {
	expiresAt := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		result TokenValidationResult
		valid  bool
	}{
		{
			name: "valid",
			result: TokenValidationResult{
				Status:           TokenValidationValid,
				Identity:         TokenIdentity{UserID: "42", Login: "viewer", DisplayName: "Viewer"},
				Scopes:           RequiredIRCScopes(),
				ExpiresAt:        expiresAt,
				RefreshAvailable: true,
			},
			valid: true,
		},
		{
			name: "malformed",
			result: TokenValidationResult{
				Status: TokenValidationMalformed,
				Detail: "token response could not be parsed",
			},
		},
		{
			name: "expired",
			result: TokenValidationResult{
				Status:           TokenValidationExpired,
				ExpiresAt:        expiresAt.Add(-time.Hour),
				RefreshAvailable: true,
			},
		},
		{
			name: "wrong user",
			result: TokenValidationResult{
				Status:   TokenValidationWrongUser,
				Identity: TokenIdentity{Login: "other"},
			},
		},
		{
			name: "missing scope",
			result: TokenValidationResult{
				Status:        TokenValidationMissingScope,
				Scopes:        []TokenScope{ScopeChatRead},
				MissingScopes: []TokenScope{ScopeChatEdit},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.Valid(); got != tt.valid {
				t.Fatalf("Valid() = %t, want %t", got, tt.valid)
			}
			if tt.result.Status == "" {
				t.Fatal("status is empty")
			}
		})
	}
}

func TestRequiredIRCScopesAndMissingScopes(t *testing.T) {
	required := RequiredIRCScopes()
	if !reflect.DeepEqual(required, []TokenScope{ScopeChatRead, ScopeChatEdit}) {
		t.Fatalf("required IRC scopes = %#v, want chat read/edit", required)
	}

	required[0] = "mutated"
	if got := RequiredIRCScopes()[0]; got != ScopeChatRead {
		t.Fatalf("RequiredIRCScopes allowed mutation, first scope = %q", got)
	}

	missing := MissingRequiredIRCScopes([]TokenScope{ScopeChatEdit})
	if !reflect.DeepEqual(missing, []TokenScope{ScopeChatRead}) {
		t.Fatalf("missing = %#v, want chat:read", missing)
	}
}

func TestTokenCredentialsRefreshAvailable(t *testing.T) {
	if (TokenCredentials{RefreshToken: "refresh", ClientID: "client", ClientSecret: "secret"}).RefreshAvailable() != true {
		t.Fatal("RefreshAvailable() = false, want true")
	}
	if (TokenCredentials{RefreshToken: "refresh", ClientID: "client"}).RefreshAvailable() {
		t.Fatal("RefreshAvailable() = true without client secret")
	}
}

func TestFakeTokenValidatorQueuesOutcomesAndRecordsRequests(t *testing.T) {
	wantErr := errors.New("validator failed")
	fake := NewFakeTokenValidator(
		FakeTokenValidationOutcome{
			Result: TokenValidationResult{Status: TokenValidationMissingScope, MissingScopes: []TokenScope{ScopeChatEdit}},
		},
		FakeTokenValidationOutcome{Err: wantErr},
	)

	credentials := TokenCredentials{Username: "viewer", OAuthToken: "oauth:secret"}
	result, err := fake.ValidateToken(context.Background(), credentials)
	if err != nil {
		t.Fatalf("ValidateToken first error = %v", err)
	}
	if result.Status != TokenValidationMissingScope {
		t.Fatalf("first status = %q, want missing_scope", result.Status)
	}

	_, err = fake.ValidateToken(context.Background(), credentials)
	if !errors.Is(err, wantErr) {
		t.Fatalf("second error = %v, want %v", err, wantErr)
	}

	requests := fake.Requests()
	if len(requests) != 2 {
		t.Fatalf("recorded requests = %d, want 2", len(requests))
	}
	if requests[0].Username != "viewer" || requests[0].OAuthToken != "oauth:secret" {
		t.Fatalf("first request = %#v, want original credentials", requests[0])
	}
}

func TestFakeTokenValidatorHonorsContextCancellation(t *testing.T) {
	fake := NewFakeTokenValidator(FakeTokenValidationOutcome{
		Result: TokenValidationResult{Status: TokenValidationValid},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fake.ValidateToken(ctx, TokenCredentials{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ValidateToken error = %v, want context.Canceled", err)
	}
	if got := len(fake.Requests()); got != 0 {
		t.Fatalf("recorded requests = %d, want 0", got)
	}
}
