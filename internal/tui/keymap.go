package tui

import "strings"

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

// keyNameToRune converts one key name to its rune value.
func keyNameToRune(name string) (rune, bool) {
	name = strings.ToLower(name)
	if r, ok := specialKeyMap[name]; ok {
		return r, true
	}
	if len(name) == 1 {
		return rune(name[0]), true
	}
	return 0, false
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
