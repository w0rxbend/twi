package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	irc "github.com/gempir/go-twitch-irc/v4"
)

const (
	defaultIRCEventBuffer = 128
	defaultOAuthTokenURL  = "https://id.twitch.tv/oauth2/token"
)

var ircOAuthTokenPattern = regexp.MustCompile(`(?i)oauth:[^\s]+`)

// IRCConfig contains the credentials and channels needed to open a Twitch IRC
// session. OAuthToken must be an IRC token with the oauth: prefix.
type IRCConfig struct {
	Username     string
	OAuthToken   string
	RefreshToken string
	ClientID     string
	ClientSecret string
	TokenURL     string
	HTTPClient   *http.Client
	Channels     []string
	Buffer       int
	Now          func() time.Time
}

// IRCClient adapts go-twitch-irc callbacks into twi's normalized event stream.
type IRCClient struct {
	client   *irc.Client
	username string
	token    string
	channels []string
	buffer   int
	now      func() time.Time
	refresh  oauthRefreshConfig
	mu       sync.RWMutex

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

	client := newIRCClient(username, token, channels)

	return &IRCClient{
		client:   client,
		username: username,
		token:    token,
		channels: channels,
		buffer:   buffer,
		now:      now,
		refresh: oauthRefreshConfig{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RefreshToken: cfg.RefreshToken,
			TokenURL:     cfg.TokenURL,
			HTTPClient:   cfg.HTTPClient,
		},
		done: make(chan struct{}),
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

	client := c.currentClient()

	registerIRCHandlers(client, emit, c.now)

	go func() {
		defer close(events)

		err := c.connectWithAuthRefresh(ctx, emit, client)
		if err == nil || errors.Is(err, irc.ErrClientDisconnected) {
			emit(NormalizeIRCDisconnect(nil, c.now()))
			return
		}
		emit(NormalizeIRCDisconnect(credentialSafeIRCError(err), c.now()))
	}()

	return events, nil
}

func registerIRCHandlers(client *irc.Client, emit func(Event), now func() time.Time) {
	client.OnConnect(func() {
		emit(NormalizeIRCConnect(now()))
	})
	client.OnPrivateMessage(func(message irc.PrivateMessage) {
		emit(NormalizeIRCPrivateMessage(message))
	})
	client.OnNoticeMessage(func(message irc.NoticeMessage) {
		emit(NormalizeIRCNoticeMessage(message))
	})
	client.OnUserNoticeMessage(func(message irc.UserNoticeMessage) {
		emit(NormalizeIRCUserNoticeMessage(message))
	})
	client.OnRoomStateMessage(func(message irc.RoomStateMessage) {
		emit(NormalizeIRCRoomStateMessage(message))
	})
	client.OnClearChatMessage(func(message irc.ClearChatMessage) {
		emit(NormalizeIRCClearChatMessage(message))
	})
	client.OnClearMessage(func(message irc.ClearMessage) {
		emit(NormalizeIRCClearMessage(message))
	})
	client.OnUserStateMessage(func(message irc.UserStateMessage) {
		emit(NormalizeIRCUserStateMessage(message))
	})
	client.OnReconnectMessage(func(message irc.ReconnectMessage) {
		emit(NormalizeIRCReconnectMessage(message, now()))
	})
	client.OnUnsetMessage(func(message irc.RawMessage) {
		emit(NormalizeIRCRawMessage(message))
	})
}

func (c *IRCClient) connectWithAuthRefresh(ctx context.Context, emit func(Event), client *irc.Client) error {
	err := client.Connect()
	if !errors.Is(err, irc.ErrLoginAuthenticationFailed) || !c.refresh.available() {
		return err
	}

	emit(Event{Kind: EventConnection, Connection: ConnectionEvent{
		Type:   ConnectionEventReconnect,
		At:     c.now(),
		Reason: "Twitch IRC authentication failed; refreshing access token",
	}})

	token, refreshed, refreshErr := c.refresh.refresh(ctx)
	if refreshErr != nil {
		return fmt.Errorf("refresh Twitch OAuth token after IRC auth failure: %w", refreshErr)
	}

	c.mu.Lock()
	c.token = token
	c.refresh.RefreshToken = refreshed
	next := newIRCClient(c.username, token, c.channels)
	c.client = next
	c.mu.Unlock()

	registerIRCHandlers(next, emit, c.now)
	return next.Connect()
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
	c.currentClient().Say(channel, text)
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
	c.currentClient().Reply(channel, parentMessageID, text)
	return nil
}

func (c *IRCClient) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.done)
		err = c.currentClient().Disconnect()
		if errors.Is(err, irc.ErrConnectionIsNotOpen) {
			err = nil
		}
	})
	return err
}

func (c *IRCClient) currentClient() *irc.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

func newIRCClient(username, token string, channels []string) *irc.Client {
	client := irc.NewClient(username, token)
	client.Capabilities = []string{irc.TagsCapability, irc.CommandsCapability}
	client.Join(channels...)
	return client
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

type oauthRefreshConfig struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	TokenURL     string
	HTTPClient   *http.Client
}

type oauthRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

func (c oauthRefreshConfig) available() bool {
	return strings.TrimSpace(c.ClientID) != "" &&
		strings.TrimSpace(c.ClientSecret) != "" &&
		strings.TrimSpace(c.RefreshToken) != ""
}

func (c oauthRefreshConfig) refresh(ctx context.Context) (string, string, error) {
	endpoint := strings.TrimSpace(c.TokenURL)
	if endpoint == "" {
		endpoint = defaultOAuthTokenURL
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(c.RefreshToken))
	form.Set("client_id", strings.TrimSpace(c.ClientID))
	form.Set("client_secret", strings.TrimSpace(c.ClientSecret))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("twitch OAuth refresh returned HTTP %d", resp.StatusCode)
	}

	var decoded oauthRefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", "", err
	}

	accessToken := normalizeOAuthToken(decoded.AccessToken)
	if accessToken == "" {
		return "", "", errors.New("twitch OAuth refresh response did not include an access token")
	}

	refreshToken := strings.TrimSpace(decoded.RefreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(c.RefreshToken)
	}
	return accessToken, refreshToken, nil
}

func normalizeOAuthToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(strings.ToLower(value), "oauth:") {
		return value
	}
	return "oauth:" + value
}
