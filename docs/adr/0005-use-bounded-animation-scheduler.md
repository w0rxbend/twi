# 0005: Use A Bounded Animation Scheduler

## Status

Accepted for MVP.

## Context

Typed-in reveal for newly received messages is a core product behavior, but it must not block chat receive, input, sending, image loading, scrolling, or reconnect handling. `PLAN.md` requires Bubble Tea tick commands, animation modes, high-throughput degradation, off-screen static rendering, and deterministic timing for tests where practical.

Animation must operate on normalized render fragments instead of raw strings.

## Decision

Implement animation as a scheduler around normalized message and render fragment state.

The scheduler should:

- Maintain a bounded reveal queue for visible incoming messages.
- Advance animation through Bubble Tea tick commands and typed messages such as `animationTickMsg` and `messageRevealCompletedMsg`.
- Reveal by render fragment or grapheme-safe reveal unit, never by byte slicing.
- Reserve final layout width for image placeholders before reveal starts.
- Show metadata such as timestamp, badges, username, and reply context before or at the start of content reveal.
- Render off-screen messages statically.
- Coalesce, shorten, or skip reveal when message throughput is high or rendering falls behind.
- Respect animation modes: `off`, `reduced`, `fast`, and `expressive`.
- Respect reduced-motion config and environment settings.
- Use an injectable `AnimationClock` or ticker abstraction for deterministic tests.

Image fetches and terminal image rendering are independent async flows. An image update may replace a placeholder visually, but it must not restart or block text reveal.

## Consequences

- Normal chat receives a polished type-on effect without coupling animation to network callbacks.
- Busy channels remain usable because the queue is bounded and degradation is explicit.
- Users can disable or reduce motion.
- Tests can cover timing behavior without sleeping on real clocks.
- Animation state adds complexity to message retention, viewport scrolling, and resize handling.

## Verification

- Unit-test queue bounds and completion behavior with a fake clock.
- Unit-test `off`, `reduced`, `fast`, and `expressive` modes.
- Unit-test high-throughput degradation when queued work exceeds configured limits.
- Unit-test off-screen messages rendering statically.
- Unit-test reveal units against styled text, Unicode graphemes, emote tokens, and image placeholders.
- Run `go test ./...` after implementation.
- Run `go test -race ./...` for scheduler, async image update, and transport callback changes.
