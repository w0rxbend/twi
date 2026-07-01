package twitch

import "context"

type ChatClient interface {
	Connect(ctx context.Context) (<-chan Event, error)
	Send(ctx context.Context, channel, text string) error
	Reply(ctx context.Context, channel, parentMessageID, text string) error
	Close() error
}

type Event struct {
	Kind    EventKind
	Message ChatMessage
	Notice  Notice
	Err     error
}

type EventKind string

const (
	EventConnected    EventKind = "connected"
	EventDisconnected EventKind = "disconnected"
	EventMessage      EventKind = "message"
	EventNotice       EventKind = "notice"
	EventRoomState    EventKind = "room_state"
	EventClear        EventKind = "clear"
	EventError        EventKind = "error"
)

type Notice struct {
	Channel string
	Text    string
}
