package tui

import "strings"

// Profile name width limit before an ellipsis cuts the rest off, the
// narrowest readable preview column and the widest any column grows.
const (
	profileNameCells = 12
	previewMinWidth  = 15
	columnMaxWidth   = 44
)

// entry is one column row before width fitting.
type entry struct {
	name     string
	proto    string
	mark     string
	dir      bool
	selected bool
	cursor   bool
}

// columnGeometry is the position and the content of one visible column.
type columnGeometry struct {
	x       int
	width   int
	entries []entry
	cursor  int
	dim     bool
	preview bool
	offset  int
}

// entryWidth returns the display width one entry needs when shown in
// full.
func entryWidth(e entry) int {
	if e.dir {
		return displayWidth(e.mark + cutName(e.name, profileNameCells) + "/")
	}
	if e.proto == "" {
		return displayWidth(e.mark + e.name)
	}
	return displayWidth(e.mark+e.name) + 2 + displayWidth(e.proto)
}

// fitEntry renders one entry inside the given width. The name gives way
// before the protocol does, and the protocol goes first when it would
// eat half the width or more.
func fitEntry(e entry, width int) (string, string) {
	style := "\x1b[0m"
	if e.selected {
		style += "\x1b[32m"
	}
	if e.cursor {
		style += "\x1b[7m"
	}
	if e.dir {
		return e.mark + cutName(e.name, profileNameCells) + "/", style
	}
	mark := displayWidth(e.mark)
	proto := displayWidth(e.proto)
	if e.proto == "" || displayWidth(e.mark+e.name)+2+proto <= width {
		return e.mark + e.name + protoSuffix(e.proto, e.cursor), style
	}
	rest := width - mark
	if proto*2 >= width || rest-2-proto < 3 {
		return e.mark + cutEllipsis(e.name, rest), style
	}
	return e.mark + cutEllipsis(e.name, rest-2-proto) + protoSuffix(e.proto, e.cursor), style
}

// protoSuffix renders the protocol tag, plain on cursor rows so the
// reverse highlight stays uniform.
func protoSuffix(proto string, cursor bool) string {
	if proto == "" {
		return ""
	}
	if cursor {
		return "  " + proto
	}
	return "  " + protoColor(proto) + proto + "\x1b[0m"
}

// cutEllipsis shortens plain text to the width and marks the cut with an
// ellipsis.
func cutEllipsis(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if displayWidth(s) <= width {
		return s
	}
	return truncateToWidthFromStart(s, max(width-3, 0)) + "..."
}

// protoColor returns the tag color of one protocol type.
func protoColor(proto string) string {
	switch strings.ToLower(proto) {
	case "ss", "ss-2022", "shadowsocks":
		return "\x1b[38;2;240;198;198m"
	case "vmess":
		return "\x1b[38;2;138;173;244m"
	case "vless":
		return "\x1b[38;2;238;212;159m"
	case "trojan":
		return "\x1b[38;2;198;160;246m"
	case "hysteria", "hysteria2":
		return "\x1b[38;2;237;135;150m"
	case "tuic":
		return "\x1b[38;2;245;169;127m"
	case "anytls":
		return "\x1b[38;2;139;213;202m"
	case "wireguard", "amnezia-wg":
		return "\x1b[38;2;145;215;227m"
	default:
		return "\x1b[38;2;147;154;183m"
	}
}

// columnWidth measures the display width one column needs for its
// entries in full.
func columnWidth(g columnGeometry) int {
	width := 1
	for _, e := range g.entries {
		width = max(width, entryWidth(e))
	}
	return width
}

// planColumns builds the geometry of the visible columns: the profile
// list, the node list when one profile is open, and the dimmed preview
// of the nodes under the cursor at the root level.
func (b *Browser) planColumns(w, listHeight int) []columnGeometry {
	if !b.inside {
		if p, ok := b.currentProfile(); ok {
			b.loadNodes(p.Name)
		}
	}
	profileCol := columnGeometry{cursor: -1}
	for i, p := range b.profiles {
		selected := false
		if info, ok := b.nodes[p.Name]; ok && info.Selected != "" {
			selected = true
		}
		cursor := i == b.cursorDir && !b.inside
		mark := "▸ "
		if selected {
			mark = "✓ "
		}
		profileCol.entries = append(profileCol.entries, entry{
			name: p.Name, mark: mark, dir: true, selected: selected, cursor: cursor,
		})
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
			mark := "  "
			if node.Name == info.Selected {
				mark = "✓ "
			}
			nodeCol.entries = append(nodeCol.entries, entry{
				name: node.Name, proto: node.Type, mark: mark,
				selected: node.Name == info.Selected, cursor: cursor,
			})
			if cursor {
				nodeCol.cursor = i
			}
		}
		cols = append(cols, nodeCol)
	} else if p, ok := b.currentProfile(); ok {
		info := b.nodes[p.Name]
		preview := columnGeometry{cursor: -1, dim: true, preview: true}
		saved := b.history[p.Name]
		cursor := clampPos(saved.cursor, len(info.Nodes))
		offset := min(saved.offset, cursor)
		if cursor >= offset+listHeight {
			offset = cursor - listHeight + 1
		}
		preview.offset = max(offset, 0)
		for _, node := range b.sortedNodes(p.Name) {
			mark := "  "
			if node.Name == info.Selected {
				mark = "✓ "
			}
			preview.entries = append(preview.entries, entry{
				name: node.Name, proto: node.Type, mark: mark,
				selected: node.Name == info.Selected,
			})
		}
		cols = append(cols, preview)
	}
	cols[0].offset = b.offsetDir

	avail := w - 1
	used := 0
	placed := []int{}
	for i := range cols {
		full := columnWidth(cols[i])
		left := avail - used
		width := min(full, left)
		if cols[i].preview {
			switch {
			case full <= left:
				width = full
			case left >= previewMinWidth:
				width = left
			default:
				width = 0
			}
		}
		width = min(width, columnMaxWidth)
		if width < 1 {
			cols[i].width = 0
			continue
		}
		cols[i].width = width
		placed = append(placed, i)
		used += width + 1
	}
	x := 1
	for _, i := range placed {
		cols[i].x = x
		x += cols[i].width + 1
	}
	return cols
}

// drawColumnCell draws one cell inside a column, padding it with spaces
// so no stale content survives. Styled text is cut on its visible width.
func drawColumnCell(x, width, y int, line, style string, isCursor, dim bool) string {
	line = truncateStyled(line, width)
	if isCursor {
		leading := len(line) - len(strings.TrimLeft(line, " "))
		line += strings.Repeat(" ", min(leading, max(width-visibleWidth(line), 0)))
	}
	if dim {
		style += "\x1b[2m"
	}
	pad := max(width-visibleWidth(line), 0)
	return "\x1b[" + itoa(y) + ";" + itoa(x) + "H" + style + line + "\x1b[0m" + strings.Repeat(" ", pad)
}

// drawColumnRow draws one visible row of a column.
func (b *Browser) drawColumnRow(geom columnGeometry, row int, dim bool) string {
	if geom.width < 1 {
		return ""
	}
	idx := geom.offset + row
	if idx < 0 || idx >= len(geom.entries) {
		return ""
	}
	e := geom.entries[idx]
	line, style := fitEntry(e, geom.width)
	return drawColumnCell(geom.x, geom.width, row+3, line, style, e.cursor, dim)
}
