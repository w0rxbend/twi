package twitch

import (
	"errors"
	"strings"
	"testing"
)

func TestNewIRCClientValidatesRequiredConfigWithoutLeakingToken(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  IRCConfig
		want string
	}{
		{
			name: "username",
			cfg: IRCConfig{
				OAuthToken: "oauth:secret-token",
				Channels:   []string{"example"},
			},
			want: "username",
		},
		{
			name: "token",
			cfg: IRCConfig{
				Username: "viewer",
				Channels: []string{"example"},
			},
			want: "OAuth token",
		},
		{
			name: "channel",
			cfg: IRCConfig{
				Username:   "viewer",
				OAuthToken: "oauth:secret-token",
			},
			want: "channel",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewIRCClient(tt.cfg)
			if err == nil {
				t.Fatal("NewIRCClient returned nil error, want validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to mention %q", err.Error(), tt.want)
			}
			if strings.Contains(err.Error(), "oauth:secret-token") {
				t.Fatalf("error leaked token: %q", err.Error())
			}
		})
	}
}

func TestNewIRCClientNormalizesChannelsAndCapabilities(t *testing.T) {
	client, err := NewIRCClient(IRCConfig{
		Username:   "viewer",
		OAuthToken: "oauth:secret-token",
		Channels:   []string{"#Example", " second "},
	})
	if err != nil {
		t.Fatalf("NewIRCClient returned error: %v", err)
	}
	if got, want := client.channels, []string{"example", "second"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("channels = %#v, want %#v", got, want)
	}
}

func TestCredentialSafeIRCErrorRedactsOAuthPattern(t *testing.T) {
	err := credentialSafeIRCError(errors.New("server rejected oauth:secret-token"))
	if err == nil {
		t.Fatal("credentialSafeIRCError returned nil, want error")
	}
	if strings.Contains(err.Error(), "oauth:secret-token") {
		t.Fatalf("error leaked token: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "oauth:<redacted>") {
		t.Fatalf("error = %q, want redacted token marker", err.Error())
	}
}
