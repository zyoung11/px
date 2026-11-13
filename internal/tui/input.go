package tui

// Input is a one line text field shown at the bottom of the screen.
type Input struct {
	value []rune
}

// Reset clears the field.
func (in *Input) Reset() {
	in.value = nil
}

// Value returns the current text.
func (in *Input) Value() string {
	return string(in.value)
}

// Key handles one key press and reports whether editing finished and
// whether the user cancelled.
func (in *Input) Key(k Key) (done, cancelled bool) {
	switch {
	case k.Name == KeyEnter:
		return true, false
	case k.Name == KeyEsc:
		return true, true
	case k.Name == KeyBackspace:
		if len(in.value) > 0 {
			in.value = in.value[:len(in.value)-1]
		}
		return false, false
	case k.Rune >= 0x20:
		in.value = append(in.value, k.Rune)
		return false, false
	}
	return false, false
}
