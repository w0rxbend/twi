# 0003: Use Kitty Graphics As First Image Protocol

## Status

Accepted for MVP.

## Context

Inline images are a core product feature for avatars, Twitch emotes, and standard emoji. `PLAN.md` names Kitty graphics as the first terminal image target because it is supported by Kitty and compatible terminals such as Ghostty. Unsupported terminals must still provide a polished text, initials, or Unicode fallback.

Image loading and rendering must not block live chat, input, send actions, or reconnect handling.

## Decision

Implement the first `ImageRenderer` target with the Kitty graphics protocol, using `github.com/dolmen-go/kittyimg` unless implementation research identifies a better maintained option before coding.

The image subsystem will:

- Detect terminal graphics capability at startup and through `twi doctor`.
- Respect config and environment switches such as `TWI_ENABLE_KITTY_IMAGES`, `TWI_IMAGE_MODE`, `TWI_AVATAR_MODE`, `TWI_EMOJI_MODE`, and `TWI_EMOTE_MODE`.
- Render avatars, Twitch emotes, and standard emoji only when capability detection and user settings allow it.
- Use stable placeholders with the same layout width as the final image.
- Keep Unicode emoji, text labels, colored tokens, or initials available as fallbacks.
- Keep image discovery, metadata resolution, download/cache, transform/crop/scale, and terminal rendering as separate steps.
- Avoid changing message layout width when an image finishes loading.

The renderer must degrade to non-image output for unsupported terminals, failed downloads, disabled modes, remote sessions that cannot support Kitty graphics, or cache errors.

## Consequences

- Tier 1 terminals can show richer chat rows with avatars, emotes, and emoji images.
- The same renderer pipeline can later add other protocols without changing normalized message data.
- Layout and width accounting become stricter because late image loads cannot reflow chat history.
- Manual terminal verification is still required because inline image behavior varies by terminal.
- Text fallback quality remains part of the primary product, not an error path.

## Verification

- Unit-test capability detection branches for enabled, disabled, unsupported, and unknown terminals.
- Unit-test placeholder width stability for avatars, emotes, and emoji.
- Unit-test renderer fallback when image files are missing, failed, or disabled.
- Add golden or snapshot tests for text fallback output where practical.
- Manually run `twi doctor` and a sample chat view in Kitty or Ghostty-compatible terminals before release.
- Run `go test ./...` after implementation.
