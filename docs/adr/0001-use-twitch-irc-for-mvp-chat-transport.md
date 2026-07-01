# 0001: Use Twitch IRC For MVP Chat Transport

## Status

Accepted for MVP.

## Context

The product needs low-latency live Twitch chat receive and send support from a terminal UI. `PLAN.md` identifies Twitch IRC as the simplest reliable path for an MVP because it supports reading and sending chat with the `chat:read` and `chat:edit` scopes, and because `github.com/gempir/go-twitch-irc/v4` already handles Twitch IRC parsing, callbacks, tags, keepalive behavior, and send helpers.

Twitch EventSub WebSockets and Helix chat APIs may provide richer metadata later, but they should not block the first runnable chat client. The Bubble Tea UI must not depend directly on any specific Twitch transport.

## Decision

Implement the first chat transport as an IRC adapter under the Twitch boundary, backed by `github.com/gempir/go-twitch-irc/v4`.

Expose the transport through an internal `ChatClient` contract rather than library-specific types. The contract should cover:

- Connecting and disconnecting.
- Joining one or more channels.
- Receiving typed chat, notice, room state, clear, user state, reconnect, and connection status events.
- Sending normal messages.
- Sending replies when a selected parent message ID is available.

The IRC adapter will:

- Connect to Twitch IRC over TLS.
- Request `twitch.tv/tags` and `twitch.tv/commands`.
- Request `twitch.tv/membership` only when the UI needs joins, parts, or chatter-list style behavior.
- Authenticate with username and OAuth token.
- Convert IRC callbacks into internal events consumed by Bubble Tea commands.
- Keep PING/PONG behavior delegated to the client library unless explicit handling is required.
- Use a local send queue for visible cooldown, rate-limit, retry, and failure status.

EventSub or API-based chat can be added later as another implementation of the same transport contract.

## Consequences

- The MVP can receive and send real Twitch chat without first building the EventSub path.
- IRC-specific details stay behind the internal Twitch boundary.
- UI tests can use a fake `ChatClient` instead of a network connection.
- IRC metadata limitations must be handled by the normalized message model and Helix asset wrapper.
- Future EventSub support will require a second adapter but should not require UI rewrites.

## Verification

- Unit-test IRC event mapping with captured or synthetic IRC fixtures for `PRIVMSG`, `USERNOTICE`, `NOTICE`, `ROOMSTATE`, `CLEARCHAT`, `CLEARMSG`, `USERSTATE`, reconnect, and connection status cases.
- Unit-test send queue behavior for success, failure, cooldown, and rate-limit feedback.
- Use fake `ChatClient` implementations in Bubble Tea update tests.
- Run `go test ./...` after implementation.
- Run `go test -race ./...` for transport callback, send queue, and reconnect changes.
