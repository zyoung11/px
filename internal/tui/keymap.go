package tui

import (
	"strings"

	"px/internal/state"
)

// Modifier bits carried by an encoded key rune.
const (
	modCtrl rune = 1 << 29
	modAlt  rune = 1 << 30
)

// Key lists the key names bound to one action.
type Key []string

// GlobalKeymap holds keybindings that work across all pages.
type GlobalKeymap struct {
	Quit            Key
	ShowHelp        Key
	CyclePages      Key
	SwitchToHome    Key
	SwitchToLibrary Key
}

// HomeKeymap holds keybindings specific to the Home page.
type HomeKeymap struct {
	ToggleKernel Key
}

// LibraryKeymap holds keybindings specific to the Library page.
type LibraryKeymap struct {
	NavUp         Key
	NavDown       Key
	NavEnterDir   Key
	NavExitDir    Key
	SelectNode    Key
	AddProfile    Key
	SyncProfile   Key
	DeleteProfile Key
	Sort          Key
	Input         InputModeKeymap
}

// InputModeKeymap holds keybindings specific to the input line.
type InputModeKeymap struct {
	ConfirmInput   Key
	EscapeInput    Key
	InputBackspace Key
	CursorLeft     Key
	CursorRight    Key
}

// Keymap holds every keybinding of the application.
type Keymap struct {
	Global  GlobalKeymap
	Home    HomeKeymap
	Library LibraryKeymap
}

// DefaultKeymap returns the keybindings px ships with. Keys shared with BM
// keep BM's defaults.
func DefaultKeymap() Keymap {
	return Keymap{
		Global: GlobalKeymap{
			Quit:            Key{"esc", "q"},
			ShowHelp:        Key{"?"},
			CyclePages:      Key{"tab"},
			SwitchToHome:    Key{"1"},
			SwitchToLibrary: Key{"2"},
		},
		Home: HomeKeymap{
			ToggleKernel: Key{"enter", "space"},
		},
		Library: LibraryKeymap{
			NavUp:         Key{"k", "w", "up"},
			NavDown:       Key{"j", "s", "down"},
			NavEnterDir:   Key{"l", "d", "right"},
			NavExitDir:    Key{"h", "a", "left"},
			SelectNode:    Key{"enter", "space"},
			AddProfile:    Key{"n"},
			SyncProfile:   Key{"r"},
			DeleteProfile: Key{"x"},
			Sort:          Key{","},
			Input: InputModeKeymap{
				ConfirmInput:   Key{"enter"},
				EscapeInput:    Key{"esc"},
				InputBackspace: Key{"backspace"},
				CursorLeft:     Key{"left"},
				CursorRight:    Key{"right"},
			},
		},
	}
}

// keymap is the active keybinding set.
var keymap = DefaultKeymap()

// setKeymap merges the configured key lists over the defaults.
func setKeymap(overrides *state.KeyOverrides) {
	k := DefaultKeymap()
	if overrides != nil {
		g := overrides.Global
		setKey(&k.Global.Quit, g["Quit"])
		setKey(&k.Global.ShowHelp, g["ShowHelp"])
		setKey(&k.Global.CyclePages, g["CyclePages"])
		setKey(&k.Global.SwitchToHome, g["SwitchToHome"])
		setKey(&k.Global.SwitchToLibrary, g["SwitchToLibrary"])
		h := overrides.Home
		setKey(&k.Home.ToggleKernel, h["ToggleKernel"])
		l := overrides.Library
		setKey(&k.Library.NavUp, l["NavUp"])
		setKey(&k.Library.NavDown, l["NavDown"])
		setKey(&k.Library.NavEnterDir, l["NavEnterDir"])
		setKey(&k.Library.NavExitDir, l["NavExitDir"])
		setKey(&k.Library.SelectNode, l["SelectNode"])
		setKey(&k.Library.AddProfile, l["AddProfile"])
		setKey(&k.Library.SyncProfile, l["SyncProfile"])
		setKey(&k.Library.DeleteProfile, l["DeleteProfile"])
		setKey(&k.Library.Sort, l["Sort"])
		i := overrides.Input
		setKey(&k.Library.Input.ConfirmInput, i["ConfirmInput"])
		setKey(&k.Library.Input.EscapeInput, i["EscapeInput"])
		setKey(&k.Library.Input.InputBackspace, i["InputBackspace"])
		setKey(&k.Library.Input.CursorLeft, i["CursorLeft"])
		setKey(&k.Library.Input.CursorRight, i["CursorRight"])
	}
	keymap = k
}

// setKey replaces one binding when the configured list is not empty.
func setKey(target *Key, names []string) {
	if len(names) > 0 {
		*target = Key(names)
	}
}

// specialKeyMap maps key names to their rune values.
var specialKeyMap = map[string]rune{
	"enter":     KeyEnter,
	"space":     ' ',
	"backspace": KeyBackspace,
	"esc":       0x1b,
	"tab":       '\t',
	"up":        KeyArrowUp,
	"down":      KeyArrowDown,
	"left":      KeyArrowLeft,
	"right":     KeyArrowRight,
}

// keyNameToRune converts one key name to its encoded rune value. The
// name may carry ctrl+ and alt+ prefixes; shift is expressed by the
// shifted character itself so shift+a is written A.
func keyNameToRune(name string) (rune, bool) {
	var mods rune
	for {
		rest, mod, ok := cutModifier(name)
		if !ok {
			break
		}
		mods |= mod
		name = rest
	}
	if base, ok := specialKeyMap[strings.ToLower(name)]; ok {
		return mods | base, true
	}
	runes := []rune(name)
	if len(runes) == 1 {
		return mods | runes[0], true
	}
	return 0, false
}

// cutModifier strips one modifier prefix from a key name.
func cutModifier(name string) (string, rune, bool) {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "ctrl+") {
		return name[5:], modCtrl, true
	}
	if strings.HasPrefix(lower, "alt+") {
		return name[4:], modAlt, true
	}
	return name, 0, false
}

// IsKey reports whether the pressed key is bound to the action.
func IsKey(key rune, keys Key) bool {
	for _, name := range keys {
		if r, ok := keyNameToRune(name); ok && r == key {
			return true
		}
	}
	return false
}
