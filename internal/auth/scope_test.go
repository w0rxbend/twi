package auth

import (
	"reflect"
	"testing"
)

func TestRequiredChatScopesAndMissingScopes(t *testing.T) {
	required := RequiredChatScopes()
	if !reflect.DeepEqual(required, []Scope{ScopeChatRead, ScopeChatEdit}) {
		t.Fatalf("required chat scopes = %#v, want read/edit", required)
	}

	required[0] = "mutated"
	if got := RequiredChatScopes()[0]; got != ScopeChatRead {
		t.Fatalf("RequiredChatScopes allowed mutation, first scope = %q", got)
	}

	if missing := MissingScopes([]Scope{ScopeChatEdit}, RequiredChatScopes()); !reflect.DeepEqual(missing, []Scope{ScopeChatRead}) {
		t.Fatalf("missing = %#v, want chat:read", missing)
	}
}

func TestScopesConvertsNonEmptyStrings(t *testing.T) {
	got := Scopes(" chat:read ", "", "chat:edit")
	if !reflect.DeepEqual(got, []Scope{ScopeChatRead, ScopeChatEdit}) {
		t.Fatalf("Scopes = %#v, want read/edit", got)
	}
	if values := ScopeValues(got); !reflect.DeepEqual(values, []string{"chat:read", "chat:edit"}) {
		t.Fatalf("ScopeValues = %#v, want string values", values)
	}
}
