# 0002: Wrap Helix Identity And Asset APIs

## Status

Accepted for MVP.

## Context

The chat UI needs Twitch identity and asset data that is not fully available from IRC alone. User avatars come from Helix Get Users `profile_image_url` values. Twitch emotes should use Helix emote metadata and URL templates. Badges, token validation, and future API-based features also belong behind a Twitch API boundary.

The app must batch lookups, cache results, avoid blocking chat rendering, and avoid leaking Twitch client library types through the UI or renderer.

## Decision

Create an internal `IdentityAssetClient` interface for Twitch identity and asset metadata. The first implementation may use `github.com/nicklaw5/helix/v2` or a similarly maintained Helix client.

The interface should return internal data types and cover:

- OAuth token validation and account identity checks.
- User lookup by login and user ID.
- Profile image URL lookup for avatars.
- Global and channel emote metadata.
- Global and channel badge metadata.
- Emote image URL expansion from Twitch templates using `id`, `format`, `theme_mode`, and `scale`.

The wrapper will:

- Batch user lookups when resolving avatars for many chat authors.
- Cache user, avatar URL, emote, and badge metadata by stable IDs and channel scope.
- Respect API rate limits and expose typed retry or degraded-state results.
- Never print client secrets, OAuth tokens, or raw authorization headers in logs, errors, config output, or debug panes.
- Publish asset metadata updates back to the app as typed Bubble Tea messages, such as `emoteCacheUpdatedMsg` and `badgeCacheUpdatedMsg`.

Image download, transform, and terminal rendering remain separate asset pipeline concerns. The Helix wrapper only resolves Twitch metadata and URLs.

## Consequences

- Twitch API usage is centralized and testable.
- Rendering code can depend on internal asset models instead of Helix response structs.
- Avatar, badge, and emote fetching can evolve without changing the message renderer.
- Caching adds invalidation and TTL decisions that must be explicit.
- The app can keep rendering text fallbacks while identity or asset metadata is loading or unavailable.

## Verification

- Unit-test batching and cache hit behavior for repeated user lookups.
- Unit-test emote URL template expansion for multiple formats, theme modes, and scales.
- Unit-test token validation failures and redacted error messages.
- Use fake Helix responses for badges, emotes, and users.
- Run `go test ./...` after implementation.
- Run `go test -race ./...` for cache and async asset update changes.
