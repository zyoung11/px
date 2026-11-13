package tui

import (
	"strconv"
	"strings"

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

// truncateWithEllipsis shortens text to the given display width and marks
// the cut with an ellipsis at the tail.
func truncateWithEllipsis(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(text) <= width {
		return text
	}
	keep := max(width-1, 0)
	cut := truncateToWidthFromStart(text, keep)
	return cut + "…"
}
