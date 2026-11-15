package keymap

import "strings"

// ModCtrl and ModAlt are the modifier bits carried by an encoded key
// rune.
const (
	ModCtrl rune = 1 << 29
	ModAlt  rune = 1 << 30
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
	Delay         Key
	Align         Key
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

// DefaultKeymap returns the keybindings px ships with. Keys shared with
// BM keep BM's defaults.
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
			ToggleKernel: Key{"space", "enter"},
		},
		Library: LibraryKeymap{
			NavUp:         Key{"k", "w", "up"},
			NavDown:       Key{"j", "s", "down"},
			NavEnterDir:   Key{"l", "d", "right"},
			NavExitDir:    Key{"h", "a", "left"},
			SelectNode:    Key{"space", "enter"},
			AddProfile:    Key{"n"},
			SyncProfile:   Key{"r"},
			DeleteProfile: Key{"x"},
			Sort:          Key{","},
			Delay:         Key{"t"},
			Align:         Key{"."},
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

// Overrides maps action names to their bound keys per section. This is
// the shape the config file stores.
type Overrides struct {
	Global  map[string][]string `json:"global,omitempty"`
	Home    map[string][]string `json:"home,omitempty"`
	Library map[string][]string `json:"library,omitempty"`
	Input   map[string][]string `json:"input,omitempty"`
}

// AsOverrides converts the keymap to the config file shape so a
// generated config lists every bindable action.
func AsOverrides(k Keymap) *Overrides {
	return &Overrides{
		Global: map[string][]string{
			"Quit":            k.Global.Quit,
			"ShowHelp":        k.Global.ShowHelp,
			"CyclePages":      k.Global.CyclePages,
			"SwitchToHome":    k.Global.SwitchToHome,
			"SwitchToLibrary": k.Global.SwitchToLibrary,
		},
		Home: map[string][]string{
			"ToggleKernel": k.Home.ToggleKernel,
		},
		Library: map[string][]string{
			"NavUp":         k.Library.NavUp,
			"NavDown":       k.Library.NavDown,
			"NavEnterDir":   k.Library.NavEnterDir,
			"NavExitDir":    k.Library.NavExitDir,
			"SelectNode":    k.Library.SelectNode,
			"AddProfile":    k.Library.AddProfile,
			"SyncProfile":   k.Library.SyncProfile,
			"DeleteProfile": k.Library.DeleteProfile,
			"Sort":          k.Library.Sort,
			"Delay":         k.Library.Delay,
			"Align":         k.Library.Align,
		},
		Input: map[string][]string{
			"ConfirmInput":   k.Library.Input.ConfirmInput,
			"EscapeInput":    k.Library.Input.EscapeInput,
			"InputBackspace": k.Library.Input.InputBackspace,
			"CursorLeft":     k.Library.Input.CursorLeft,
			"CursorRight":    k.Library.Input.CursorRight,
		},
	}
}

// Merge returns the keymap with the configured lists applied. An empty
// list unbinds the action and an absent entry keeps the default.
func Merge(k Keymap, o *Overrides) Keymap {
	if o == nil {
		return k
	}
	set := func(target *Key, names []string) {
		if names != nil {
			*target = Key(names)
		}
	}
	set(&k.Global.Quit, o.Global["Quit"])
	set(&k.Global.ShowHelp, o.Global["ShowHelp"])
	set(&k.Global.CyclePages, o.Global["CyclePages"])
	set(&k.Global.SwitchToHome, o.Global["SwitchToHome"])
	set(&k.Global.SwitchToLibrary, o.Global["SwitchToLibrary"])
	set(&k.Home.ToggleKernel, o.Home["ToggleKernel"])
	set(&k.Library.NavUp, o.Library["NavUp"])
	set(&k.Library.NavDown, o.Library["NavDown"])
	set(&k.Library.NavEnterDir, o.Library["NavEnterDir"])
	set(&k.Library.NavExitDir, o.Library["NavExitDir"])
	set(&k.Library.SelectNode, o.Library["SelectNode"])
	set(&k.Library.AddProfile, o.Library["AddProfile"])
	set(&k.Library.SyncProfile, o.Library["SyncProfile"])
	set(&k.Library.DeleteProfile, o.Library["DeleteProfile"])
	set(&k.Library.Sort, o.Library["Sort"])
	set(&k.Library.Delay, o.Library["Delay"])
	set(&k.Library.Align, o.Library["Align"])
	set(&k.Library.Input.ConfirmInput, o.Input["ConfirmInput"])
	set(&k.Library.Input.EscapeInput, o.Input["EscapeInput"])
	set(&k.Library.Input.InputBackspace, o.Input["InputBackspace"])
	set(&k.Library.Input.CursorLeft, o.Input["CursorLeft"])
	set(&k.Library.Input.CursorRight, o.Input["CursorRight"])
	return k
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

// KeyEnter, KeyBackspace and the arrow keys name the runes the key
// decoder emits for keys without a printable character.
const (
	KeyArrowUp = 1000 + iota
	KeyArrowDown
	KeyArrowLeft
	KeyArrowRight
	KeyEnter
	KeyBackspace
)

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
		return name[5:], ModCtrl, true
	}
	if strings.HasPrefix(lower, "alt+") {
		return name[4:], ModAlt, true
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
