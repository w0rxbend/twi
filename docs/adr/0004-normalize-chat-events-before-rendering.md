# 0004: Normalize Chat Events Before Rendering

## Status

Accepted for MVP.

## Context

Twitch IRC, future EventSub chat events, Helix metadata, and internal system events all expose different shapes. Rendering also needs stable fragments for usernames, badges, avatars, text, mentions, emotes, emoji, replies, moderation notices, and animation state.

`PLAN.md` requires typed-in reveal to operate on normalized fragments rather than raw string slicing so ANSI styles, grapheme clusters, image placeholders, Twitch emote tokens, and standard emoji are not corrupted.

## Decision

Normalize every incoming transport or system event into internal message models before it reaches the renderer.

The model should include internal types such as:

- `ChatMessage`
- `MessageFragment`
- `RenderFragment`
- `AssetRef`
- `AvatarAsset`
- `EmoteAsset`
- `EmojiAsset`
- `AnimatedMessageState`
- `ChannelState`
- `ConnectionState`

`ChatMessage` should carry stable fields for ID, channel, timestamp, author login, author user ID, author display name, author color, avatar reference, badges, raw text, fragments, emotes, emoji fragments, bits or cheer data, reply metadata, message type, moderation state, reveal state, and raw tags for debug inspection.

Rendering will use precomputed `RenderFragment` values for:

- Metadata fragments such as avatar, timestamp, badges, username, and reply context.
- Content fragments such as text, mention, Twitch emote, standard emoji, and cheer or bits.
- Layout fragments such as wrapping, placeholders, and continuation indentation.
- Animation fragments such as reveal units and frame state.

Raw transport tags and library response structs must not be rendered directly in the main chat viewport. Raw data can appear in a debug or inspect panel.

## Consequences

- The UI becomes independent from IRC-specific and Helix-specific response formats.
- Future EventSub support can map into the same app and renderer contracts.
- Rendering and animation can be tested without network clients.
- Fragment parsing must handle Unicode, grapheme clusters, Twitch emote positions, and display width carefully.
- There is an up-front conversion layer, but it reduces complexity in the Bubble Tea model and view.

## Verification

- Unit-test normalization for representative IRC events and internal system events.
- Unit-test Twitch emote position parsing with ASCII, Unicode, and mixed emoji text.
- Unit-test mention, reply, moderation, and `/me` action fragments.
- Unit-test render fragment width accounting with `lipgloss.Width` or the selected width-aware helper.
- Unit-test partial reveal output to ensure grapheme clusters, ANSI styles, emote tokens, and image placeholders are not split.
- Run `go test ./...` after implementation.
