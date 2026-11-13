package tui

import (
	"os"
	"unicode/utf8"
)

// Key names decoded from terminal input.
const (
	KeyEnter     = "enter"
	KeyTab       = "tab"
	KeyBackspace = "backspace"
	KeyEsc       = "esc"
	KeyUp        = "up"
	KeyDown      = "down"
	KeyLeft      = "left"
	KeyRight     = "right"
)

// Key is one decoded key press. Rune holds a printable character, Name
// holds one of the key names above.
type Key struct {
	Rune rune
	Name string
}

// ReadKeys starts a goroutine that decodes stdin into key events.
func ReadKeys() <-chan Key {
	ch := make(chan Key, 16)
	go func() {
		defer close(ch)
		buf := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			for _, k := range parseKeys(buf[:n]) {
				ch <- k
			}
		}
	}()
	return ch
}

// parseKeys decodes one batch of terminal input bytes.
func parseKeys(b []byte) []Key {
	var keys []Key
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == 0x1b:
			k, n, ok := parseEsc(b[i:])
			if ok {
				keys = append(keys, k)
			}
			i += n
		case c == '\r' || c == '\n':
			keys = append(keys, Key{Name: KeyEnter})
			i++
		case c == '\t':
			keys = append(keys, Key{Name: KeyTab})
			i++
		case c == 0x7f || c == 0x08:
			keys = append(keys, Key{Name: KeyBackspace})
			i++
		case c < 0x20:
			i++
		default:
			r, size := utf8.DecodeRune(b[i:])
			keys = append(keys, Key{Rune: r})
			i += size
		}
	}
	return keys
}

// parseEsc decodes one escape sequence and returns its key, its byte
// length and whether it maps to a known key.
func parseEsc(b []byte) (Key, int, bool) {
	if len(b) < 2 || b[1] != '[' {
		return Key{Name: KeyEsc}, 1, true
	}
	if len(b) < 3 {
		return Key{}, len(b), false
	}
	switch b[2] {
	case 'A':
		return Key{Name: KeyUp}, 3, true
	case 'B':
		return Key{Name: KeyDown}, 3, true
	case 'C':
		return Key{Name: KeyRight}, 3, true
	case 'D':
		return Key{Name: KeyLeft}, 3, true
	case 'Z':
		return Key{Name: KeyTab}, 3, true
	}
	i := 2
	for i < len(b) && (b[i] < 0x40 || b[i] > 0x7e) {
		i++
	}
	return Key{}, i + 1, false
}
