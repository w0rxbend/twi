//go:build !unix

package cli

import (
	"strings"
	"testing"
)

func TestDebugLogOpenOtherPlatformDocumentsLimitedGuarantees(t *testing.T) {
	if debugLogOpenUsesNoFollow {
		t.Fatal("non-Unix debug log opener must not advertise Unix no-follow guarantees")
	}
	for _, want := range []string{"does not provide", "O_NOFOLLOW", "ACL"} {
		if !strings.Contains(debugLogOpenPlatformNote, want) {
			t.Fatalf("non-Unix platform note missing %q: %q", want, debugLogOpenPlatformNote)
		}
	}
}
