package tui

import (
	km "px/internal/keymap"
)

// Key lists the key names bound to one action.
type Key = km.Key

// Keymap holds every keybinding of the application.
type Keymap = km.Keymap

// Keys without a printable character the decoder emits.
const (
	KeyArrowUp    = km.KeyArrowUp
	KeyArrowDown  = km.KeyArrowDown
	KeyArrowLeft  = km.KeyArrowLeft
	KeyArrowRight = km.KeyArrowRight
	KeyEnter      = km.KeyEnter
	KeyBackspace  = km.KeyBackspace
)

// Modifier bits carried by an encoded key rune.
const (
	modCtrl = km.ModCtrl
	modAlt  = km.ModAlt
)

// keymap holds the active keybinding set.
var keymap = km.DefaultKeymap()

// setKeymap merges the configured key lists over the defaults.
func setKeymap(overrides *km.Overrides) {
	keymap = km.Merge(keymap, overrides)
}

// IsKey reports whether the pressed key is bound to the action.
func IsKey(key rune, keys Key) bool {
	return km.IsKey(key, keys)
}
