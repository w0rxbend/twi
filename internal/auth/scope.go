package auth

import "strings"

// Scope is a Twitch OAuth scope requested or granted during login.
type Scope string

const (
	// ScopeChatRead allows Twitch IRC clients to receive chat messages.
	ScopeChatRead Scope = "chat:read"
	// ScopeChatEdit allows Twitch IRC clients to send chat messages.
	ScopeChatEdit Scope = "chat:edit"
)

var requiredChatScopes = []Scope{ScopeChatRead, ScopeChatEdit}

// ChatReadScopes returns the OAuth scopes required for read-only Twitch chat.
func ChatReadScopes() []Scope {
	return []Scope{ScopeChatRead}
}

// ChatSendScopes returns the OAuth scopes required to send Twitch chat messages.
func ChatSendScopes() []Scope {
	return []Scope{ScopeChatEdit}
}

// RequiredChatScopes returns the minimum OAuth scopes for twi's MVP chat read
// and send behavior.
func RequiredChatScopes() []Scope {
	return append([]Scope(nil), requiredChatScopes...)
}

// MissingScopes returns required scopes that are absent from granted.
func MissingScopes(granted, required []Scope) []Scope {
	have := make(map[Scope]bool, len(granted))
	for _, scope := range granted {
		have[scope] = true
	}

	missing := make([]Scope, 0, len(required))
	for _, scope := range required {
		if !have[scope] {
			missing = append(missing, scope)
		}
	}
	return missing
}

// Scopes converts non-empty string values into Scope values.
func Scopes(values ...string) []Scope {
	scopes := make([]Scope, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			scopes = append(scopes, Scope(value))
		}
	}
	return scopes
}

// ScopeValues converts scopes into their string OAuth values.
func ScopeValues(scopes []Scope) []string {
	values := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		values = append(values, string(scope))
	}
	return values
}
