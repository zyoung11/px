package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// itoa formats one integer.
func itoa(n int) string {
	return strconv.Itoa(n)
}

// truncateToWidthFromStart shortens text to the given display width from
// the start side, dropping the tail.
func truncateToWidthFromStart(text string, width int) string {
	if width <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if used+rw > width {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String()
}

// fit truncates or pads plain text to exactly the given display width.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	cut := truncateToWidthFromStart(s, w)
	if pad := w - displayWidth(cut); pad > 0 {
		cut += strings.Repeat(" ", pad)
	}
	return cut
}

// truncateToWidth shortens text to the given display width.
func truncateToWidth(text string, width int) string {
	return truncateToWidthFromStart(text, width)
}

// displayWidth returns the display width of text in cells.
func displayWidth(text string) int {
	return runewidth.StringWidth(text)
}

// cutName shortens a name past the limit and marks the cut with an
// ellipsis.
func cutName(s string, limit int) string {
	if displayWidth(s) <= limit {
		return s
	}
	return truncateToWidthFromStart(s, max(limit-3, 0)) + "..."
}

// visibleWidth returns the display width of styled text with ANSI
// sequences ignored.
func visibleWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = ansiEnd(s, i)
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		w += runewidth.RuneWidth(r)
		i += size
	}
	return w
}

// truncateStyled cuts styled text to the visible width, marks the cut
// with an ellipsis and closes any open color.
func truncateStyled(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if visibleWidth(s) <= width {
		return s
	}
	var b strings.Builder
	used := 0
	cut := false
	for i := 0; i < len(s) && !cut; {
		if s[i] == 0x1b {
			end := ansiEnd(s, i)
			b.WriteString(s[i:end])
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runewidth.RuneWidth(r)
		if used+rw > width-3 {
			cut = true
			continue
		}
		b.WriteRune(r)
		used += rw
		i += size
	}
	b.WriteString("\x1b[0m...")
	return b.String()
}

// ansiEnd returns the index just past the ANSI escape sequence
// starting at i. CSI sequences keep their parameter bytes inside the
// sequence so a cut never leaks them as text.
func ansiEnd(s string, i int) int {
	j := i + 1
	if j < len(s) && s[j] == '[' {
		j++
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3f {
			j++
		}
	}
	if j < len(s) {
		j++
	}
	return j
}
