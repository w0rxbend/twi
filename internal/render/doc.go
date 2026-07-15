// Package render converts normalized chat messages and asset state into
// terminal-safe rows.
//
// It owns semantic fragments, width-aware wrapping, and color decisions.
// Avatars, badges, emotes, and emoji always render as text (initials,
// labels, and unicode/name fallbacks) - there is no image rendering path.
package render
