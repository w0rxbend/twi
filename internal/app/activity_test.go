package app

import (
	"testing"
	"time"

	"github.com/worxbend/twi/internal/config"
	"github.com/worxbend/twi/internal/twitch"
)

func TestRecordActivityFromMessageClassifiesRaidsAndSubs(t *testing.T) {
	model := newMockShellModel("example", config.Default())

	model.recordActivityFromMessage(twitch.ChatMessage{
		Channel:       "example",
		Type:          twitch.MessageTypeNotice,
		SystemEventID: "raid",
		Text:          "RaiderName is raiding with 42 viewers!",
		Timestamp:     time.Date(2026, 7, 14, 20, 0, 0, 0, time.UTC),
	})
	model.recordActivityFromMessage(twitch.ChatMessage{
		Channel:       "example",
		Type:          twitch.MessageTypeNotice,
		SystemEventID: "resub",
		Text:          "ViewerName subscribed for 6 months!",
	})
	// Plain chat and twi's own generic "system" banners are not activity.
	model.recordActivityFromMessage(twitch.ChatMessage{Channel: "example", Type: twitch.MessageTypeChat, Text: "hello"})
	model.recordActivityFromMessage(twitch.ChatMessage{Channel: "example", Type: twitch.MessageTypeSystem, Text: "Mock chat is ready."})

	if len(model.activityLog) != 2 {
		t.Fatalf("activityLog = %#v, want 2 entries (raid, resub)", model.activityLog)
	}
	if model.activityLog[0].Kind != activityIRCEvent || model.activityLog[0].Channel != "example" {
		t.Fatalf("entry[0] = %#v, want irc_event in #example", model.activityLog[0])
	}
	if model.activityLog[0].At != time.Date(2026, 7, 14, 20, 0, 0, 0, time.UTC) {
		t.Fatalf("entry[0].At = %v, want message timestamp preserved", model.activityLog[0].At)
	}
}

func TestApplyNewFollowerActivityEstablishesBaselineSilently(t *testing.T) {
	model := newMockShellModel("example", config.Default())
	model.applyNewFollowerActivity([]twitch.Follower{
		{UserID: "1", UserName: "First"},
		{UserID: "2", UserName: "Second"},
	})
	if len(model.activityLog) != 0 {
		t.Fatalf("activityLog after first poll = %#v, want empty (baseline only)", model.activityLog)
	}
	if len(model.seenFollowerIDs) != 2 {
		t.Fatalf("seenFollowerIDs = %#v, want 2 entries", model.seenFollowerIDs)
	}
}

func TestApplyNewFollowerActivityDetectsNewFollowersAfterBaseline(t *testing.T) {
	model := newMockShellModel("example", config.Default())
	model.applyNewFollowerActivity([]twitch.Follower{{UserID: "1", UserName: "First"}})

	model.applyNewFollowerActivity([]twitch.Follower{
		{UserID: "2", UserName: "Second", FollowedAt: time.Date(2026, 7, 14, 21, 0, 0, 0, time.UTC)},
		{UserID: "1", UserName: "First"},
	})
	if len(model.activityLog) != 1 {
		t.Fatalf("activityLog = %#v, want 1 new-follower entry", model.activityLog)
	}
	if model.activityLog[0].Kind != activityFollow || model.activityLog[0].Text != "Second followed" {
		t.Fatalf("entry = %#v, want Kind=follow Text=\"Second followed\"", model.activityLog[0])
	}

	// Polling again with the same data must not re-report the same follower.
	model.applyNewFollowerActivity([]twitch.Follower{
		{UserID: "2", UserName: "Second"},
		{UserID: "1", UserName: "First"},
	})
	if len(model.activityLog) != 1 {
		t.Fatalf("activityLog after repeat poll = %#v, want still 1 entry", model.activityLog)
	}
}

func TestAppendActivityBoundsLogSize(t *testing.T) {
	model := newMockShellModel("example", config.Default())
	for i := 0; i < maxActivityEntries+10; i++ {
		model.appendActivity(activityEntry{Kind: activityIRCEvent, Text: "entry"})
	}
	if len(model.activityLog) != maxActivityEntries {
		t.Fatalf("activityLog length = %d, want bounded to %d", len(model.activityLog), maxActivityEntries)
	}
}
