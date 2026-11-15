package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// widthCondition measures text the way the terminal draws it: ambiguous
// width runes take one cell whatever the locale says.
var widthCondition = runewidth.Condition{EastAsianWidth: false, StrictEmojiNeutral: true}

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
		rw := widthCondition.RuneWidth(r)
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

// displayWidth returns the display width of text in cells.
func displayWidth(text string) int {
	return widthCondition.StringWidth(text)
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
		w += widthCondition.RuneWidth(r)
		i += size
	}
	return w
}

// truncateStyled cuts styled text to the visible width and marks the
// cut with an ellipsis in the style active at the cut, so the marker
// keeps the color of the text it replaces.
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
	last := ""
	for i := 0; i < len(s) && !cut; {
		if s[i] == 0x1b {
			end := ansiEnd(s, i)
			seq := s[i:end]
			if strings.HasSuffix(seq, "m") {
				last = seq
			}
			b.WriteString(seq)
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := widthCondition.RuneWidth(r)
		if used+rw > width-3 {
			cut = true
			continue
		}
		b.WriteRune(r)
		used += rw
		i += size
	}
	b.WriteString(last)
	b.WriteString("...")
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
