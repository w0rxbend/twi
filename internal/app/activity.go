package app

import (
	"strings"
	"time"

	"github.com/worxbend/twi/internal/twitch"
)

const maxActivityEntries = 200

// activityKind coarsely categorizes an activity log entry for potential
// future filtering/styling. Twitch's IRC system events (raids, subs, gift
// subs, etc.) map to activityIRCEvent; new followers map to activityFollow
// (detected by polling and diffing Get Channel Followers, since Twitch only
// pushes follow events through EventSub - a separate WebSocket push API -
// not IRC or any plain polling endpoint with real-time semantics).
type activityKind string

const (
	activityFollow   activityKind = "follow"
	activityIRCEvent activityKind = "irc_event"
)

// activityEntry is one row in the activity log column.
type activityEntry struct {
	Kind    activityKind
	Channel string
	Text    string
	At      time.Time
}

// recordActivityFromMessage appends an activity log entry when message
// represents a Twitch system event twi already classifies for desktop
// notifications (raid, sub, resub, gift sub, etc. - see
// systemNotificationFromMessage/systemEventLabel in system_notification.go).
// Plain chat messages produce no activity entry.
func (m *mockShellModel) recordActivityFromMessage(message twitch.ChatMessage) {
	notification, ok := systemNotificationFromMessage(message)
	if !ok {
		return
	}
	// "system" is twi's own catch-all for MessageTypeSystem placeholders
	// (e.g. mock-mode banners); it isn't a real Twitch event worth logging.
	if strings.EqualFold(notification.EventID, "system") {
		return
	}
	at := message.Timestamp
	if at.IsZero() {
		at = time.Now()
	}
	m.appendActivity(activityEntry{
		Kind:    activityIRCEvent,
		Channel: notification.Channel,
		Text:    systemNotificationSummary(notification),
		At:      at,
	})
}

func (m *mockShellModel) appendActivity(entry activityEntry) {
	if entry.At.IsZero() {
		entry.At = time.Now()
	}
	m.activityLog = append(m.activityLog, entry)
	if len(m.activityLog) > maxActivityEntries {
		m.activityLog = m.activityLog[len(m.activityLog)-maxActivityEntries:]
	}
}

// applyNewFollowerActivity diffs the latest Get Channel Followers page
// against the previously seen follower IDs and appends one activity entry
// per newly seen follower, oldest first. The very first poll only
// establishes the seen-set as a baseline; it never floods the log by
// treating every existing follower as "new".
func (m *mockShellModel) applyNewFollowerActivity(page []twitch.Follower) {
	hadBaseline := m.seenFollowerIDs != nil
	if m.seenFollowerIDs == nil {
		m.seenFollowerIDs = make(map[string]bool, len(page))
	}
	for i := len(page) - 1; i >= 0; i-- {
		follower := page[i]
		if follower.UserID == "" || m.seenFollowerIDs[follower.UserID] {
			continue
		}
		m.seenFollowerIDs[follower.UserID] = true
		if !hadBaseline {
			continue
		}
		name := follower.UserName
		if name == "" {
			name = follower.UserLogin
		}
		m.appendActivity(activityEntry{
			Kind: activityFollow,
			Text: name + " followed",
			At:   follower.FollowedAt,
		})
	}
}
