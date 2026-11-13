package tui

import (
	"strings"
)

// columnGeometry is the position and the content of one visible column.
type columnGeometry struct {
	x       int
	width   int
	lines   []string
	styles  []string
	cursor  int
	dim     bool
	preview bool
	offset  int
}

// entryLine renders one profile or node row in BM's entry style: a
// triangle for profiles, a check mark for anything carrying a selection,
// and reverse video for the cursor row.
func entryLine(name string, isDir, selected, cursor bool) (string, string) {
	line := ""
	style := "\x1b[0m"
	switch {
	case isDir && selected:
		line = "✓ " + name + "/"
		style += "\x1b[32m"
	case isDir:
		line = "▸ " + name + "/"
	case selected:
		line = "✓ " + name
		style += "\x1b[32m"
	default:
		line = "  " + name
	}
	if cursor {
		style += "\x1b[7m"
	}
	return line, style
}

// columnWidth measures the display width one column needs for its
// entries.
func columnWidth(g columnGeometry) int {
	width := 1
	for _, line := range g.lines {
		width = max(width, displayWidth(line)+1)
	}
	return width
}

// planColumns builds the geometry of the visible columns: the profile
// list, the node list when one profile is open, and the dimmed preview of
// the nodes under the cursor at the root level.
func (b *Browser) planColumns(w int) []columnGeometry {
	profileCol := columnGeometry{cursor: -1}
	for i, p := range b.profiles {
		selected := false
		if info, ok := b.nodes[p.Name]; ok && info.Selected != "" {
			selected = true
		}
		cursor := i == b.cursorDir && !b.inside
		line, style := entryLine(p.Name, true, selected, cursor)
		profileCol.lines = append(profileCol.lines, line)
		profileCol.styles = append(profileCol.styles, style)
		if cursor {
			profileCol.cursor = i
		}
	}
	if b.inside {
		profileCol.dim = true
	}

	cols := []columnGeometry{profileCol}
	if b.inside {
		nodeCol := columnGeometry{cursor: -1, offset: b.offsetItem}
		name, _ := b.currentProfile()
		info := b.nodes[name.Name]
		for i, node := range b.sortedNodes(name.Name) {
			cursor := i == b.cursorItem
			line, style := entryLine(node, false, node == info.Selected, cursor)
			nodeCol.lines = append(nodeCol.lines, line)
			nodeCol.styles = append(nodeCol.styles, style)
			if cursor {
				nodeCol.cursor = i
			}
		}
		cols = append(cols, nodeCol)
	} else if p, ok := b.currentProfile(); ok {
		b.loadNodes(p.Name)
		info := b.nodes[p.Name]
		preview := columnGeometry{cursor: -1, dim: true, preview: true}
		for _, node := range b.sortedNodes(p.Name) {
			line, style := entryLine(node, false, node == info.Selected, false)
			preview.lines = append(preview.lines, line)
			preview.styles = append(preview.styles, style)
		}
		cols = append(cols, preview)
	}
	cols[0].offset = b.offsetDir

	avail := w - 1
	x := 1
	for i := range cols {
		full := columnWidth(cols[i])
		width := min(full, avail-(x-1))
		if width < 1 {
			cols[i].width = 0
			continue
		}
		cols[i].x = x
		cols[i].width = width
		x += width + 1
	}
	return cols
}

// drawColumnCell draws one cell inside a column, padding it with spaces
// so no stale content survives.
func drawColumnCell(x, width, y int, line, style string, isCursor, dim bool) string {
	line = truncateWithEllipsis(line, width)
	if isCursor {
		leading := len(line) - len(strings.TrimLeft(line, " "))
		line += strings.Repeat(" ", min(leading, max(width-displayWidth(line), 0)))
	}
	if dim {
		style += "\x1b[2m"
	}
	pad := max(width-displayWidth(line), 0)
	return "\x1b[" + itoa(y) + ";" + itoa(x) + "H" + style + line + "\x1b[0m" + strings.Repeat(" ", pad)
}

// drawColumnRow draws one visible row of a column.
func (b *Browser) drawColumnRow(geom columnGeometry, row int, dim bool) string {
	if geom.width < 1 {
		return ""
	}
	idx := geom.offset + row
	if idx < 0 || idx >= len(geom.lines) {
		return ""
	}
	y := row + 3
	return drawColumnCell(geom.x, geom.width, y, geom.lines[idx], geom.styles[idx], idx == geom.cursor, dim)
}
