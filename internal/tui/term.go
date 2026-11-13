package tui

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

// Terminal owns raw mode, the alternate screen and the terminal size.
type Terminal struct {
	fd    int
	state *term.State
}

// NewTerminal enters raw mode and switches to the alternate screen.
func NewTerminal() (*Terminal, error) {
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	t := &Terminal{fd: fd, state: state}
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l")
	return t, nil
}

// Close restores the terminal to its original state.
func (t *Terminal) Close() {
	os.Stdout.WriteString("\x1b[?25h\x1b[?1049l")
	term.Restore(t.fd, t.state)
}

// Size returns the terminal width and height in cells.
func (t *Terminal) Size() (int, int) {
	w, h, err := term.GetSize(t.fd)
	if err != nil {
		return 80, 24
	}
	return w, h
}

// Winch delivers a value whenever the terminal size changes.
func (t *Terminal) Winch() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	return ch
}

// Write sends raw bytes to the terminal.
func (t *Terminal) Write(s string) {
	os.Stdout.WriteString(s)
}
