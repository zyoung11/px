package logx

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

const (
	colorBlue   = "\x1b[38;2;138;173;244m"
	colorGreen  = "\x1b[38;2;166;218;149m"
	colorYellow = "\x1b[38;2;238;212;159m"
	colorRed    = "\x1b[38;2;237;135;150m"
	colorReset  = "\x1b[0m"
)

var colored = term.IsTerminal(int(os.Stdout.Fd()))

func prefix(color, mark string) string {
	if !colored {
		return mark
	}
	return color + mark + colorReset
}

// Info prints a status message.
func Info(format string, a ...any) {
	fmt.Println(prefix(colorBlue, "[i]"), fmt.Sprintf(format, a...))
}

// Success prints a completed operation.
func Success(format string, a ...any) {
	fmt.Println(prefix(colorGreen, "[+]"), fmt.Sprintf(format, a...))
}

// Warn prints a non-fatal notice.
func Warn(format string, a ...any) {
	fmt.Println(prefix(colorYellow, "[!]"), fmt.Sprintf(format, a...))
}

// Error prints a failure.
func Error(format string, a ...any) {
	fmt.Println(prefix(colorRed, "[-]"), fmt.Sprintf(format, a...))
}
