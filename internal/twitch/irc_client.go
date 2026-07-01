package twitch

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	irc "github.com/gempir/go-twitch-irc/v4"
)

const defaultIRCEventBuffer = 128

var ircOAuthTokenPattern = regexp.MustCompile(`(?i)oauth:[^\s]+`)

// IRCConfig contains the credentials and channels needed to open a Twitch IRC
// session. OAuthToken must be an IRC token with the oauth: prefix.
type IRCConfig struct {
	Username   string
	OAuthToken string
	Channels   []string
	Buffer     int
	Now        func() time.Time
}

// IRCClient adapts go-twitch-irc callbacks into twi's normalized event stream.
type IRCClient struct {
	client   *irc.Client
	channels []string
	buffer   int
	now      func() time.Time

	done      chan struct{}
	closeOnce sync.Once
}

var _ ChatClient = (*IRCClient)(nil)

// NewIRCClient creates a Twitch IRC client without opening the network
// connection. Call Connect to start the read loop.
func NewIRCClient(cfg IRCConfig) (*IRCClient, error) {
	username := strings.TrimSpace(cfg.Username)
	if username == "" {
		return nil, errors.New("missing Twitch username")
	}
	token := strings.TrimSpace(cfg.OAuthToken)
	if token == "" {
		return nil, errors.New("missing Twitch OAuth token")
	}

	channels := normalizeIRCChannels(cfg.Channels)
	if len(channels) == 0 {
		return nil, errors.New("missing Twitch channel")
	}

	buffer := cfg.Buffer
	if buffer <= 0 {
		buffer = defaultIRCEventBuffer
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	client := irc.NewClient(username, token)
	client.Capabilities = []string{irc.TagsCapability, irc.CommandsCapability}
	client.Join(channels...)

	return &IRCClient{
		client:   client,
		channels: channels,
		buffer:   buffer,
		now:      now,
		done:     make(chan struct{}),
	}, nil
}

func (c *IRCClient) Connect(ctx context.Context) (<-chan Event, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	events := make(chan Event, c.buffer)
	emit := func(event Event) {
		select {
		case events <- event:
		case <-ctx.Done():
		case <-c.done:
		}
	}

	c.client.OnConnect(func() {
		emit(NormalizeIRCConnect(c.now()))
	})
	c.client.OnPrivateMessage(func(message irc.PrivateMessage) {
		emit(NormalizeIRCPrivateMessage(message))
	})
	c.client.OnNoticeMessage(func(message irc.NoticeMessage) {
		emit(NormalizeIRCNoticeMessage(message))
	})
	c.client.OnUserNoticeMessage(func(message irc.UserNoticeMessage) {
		emit(NormalizeIRCUserNoticeMessage(message))
	})
	c.client.OnRoomStateMessage(func(message irc.RoomStateMessage) {
		emit(NormalizeIRCRoomStateMessage(message))
	})
	c.client.OnClearChatMessage(func(message irc.ClearChatMessage) {
		emit(NormalizeIRCClearChatMessage(message))
	})
	c.client.OnClearMessage(func(message irc.ClearMessage) {
		emit(NormalizeIRCClearMessage(message))
	})
	c.client.OnUserStateMessage(func(message irc.UserStateMessage) {
		emit(NormalizeIRCUserStateMessage(message))
	})
	c.client.OnReconnectMessage(func(message irc.ReconnectMessage) {
		emit(NormalizeIRCReconnectMessage(message, c.now()))
	})
	c.client.OnUnsetMessage(func(message irc.RawMessage) {
		emit(NormalizeIRCRawMessage(message))
	})

	go func() {
		defer close(events)

		err := c.client.Connect()
		if err == nil || errors.Is(err, irc.ErrClientDisconnected) {
			emit(NormalizeIRCDisconnect(nil, c.now()))
			return
		}
		emit(NormalizeIRCDisconnect(credentialSafeIRCError(err), c.now()))
	}()

	return events, nil
}

func (c *IRCClient) Send(ctx context.Context, channel, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	channel = normalizeIRCChannel(channel)
	if channel == "" {
		return errors.New("missing Twitch channel")
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("message text cannot be empty")
	}
	c.client.Say(channel, text)
	return nil
}

func (c *IRCClient) Reply(ctx context.Context, channel, parentMessageID, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	channel = normalizeIRCChannel(channel)
	if channel == "" {
		return errors.New("missing Twitch channel")
	}
	if strings.TrimSpace(parentMessageID) == "" {
		return errors.New("missing parent message ID")
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("message text cannot be empty")
	}
	c.client.Reply(channel, parentMessageID, text)
	return nil
}

func (c *IRCClient) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.done)
		err = c.client.Disconnect()
		if errors.Is(err, irc.ErrConnectionIsNotOpen) {
			err = nil
		}
	})
	return err
}

func credentialSafeIRCError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, irc.ErrLoginAuthenticationFailed) {
		return errors.New("twitch IRC authentication failed; verify username, OAuth token, and chat:read scope")
	}
	return fmt.Errorf("twitch IRC connection failed: %s", redactIRCError(err.Error()))
}

func redactIRCError(value string) string {
	return ircOAuthTokenPattern.ReplaceAllString(value, "oauth:<redacted>")
}

func normalizeIRCChannels(values []string) []string {
	channels := make([]string, 0, len(values))
	for _, value := range values {
		channel := normalizeIRCChannel(value)
		if channel != "" {
			channels = append(channels, channel)
		}
	}
	return channels
}

func normalizeIRCChannel(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "#")))
}
