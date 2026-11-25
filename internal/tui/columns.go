package tui

import (
	"strings"

	"px/internal/ipc"
)

// Profile name width limit before an ellipsis cuts the rest off and the
// narrowest readable preview column.
const (
	profileNameCells = 15
	previewMinWidth  = 15
)

// entry is one column row before width fitting.
type entry struct {
	name     string
	proto    string
	mark     string
	delayMs  int
	nameW    int
	protoW   int
	delayW   int
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
	if e.nameW > 0 {
		w := displayWidth(e.mark) + e.nameW + 2 + e.protoW
		if e.delayW > 0 {
			w += 2 + e.delayW
		}
		return w
	}
	if e.proto == "" {
		return displayWidth(e.mark + e.name)
	}
	return displayWidth(e.mark+e.name) + 2 + displayWidth(e.proto)
}

// fitEntry renders one entry inside the given width. The name gives way
// before the protocol does, and the protocol goes first when it would
// eat half the width or more. A selected entry is green text with a
// check mark; only the cursor row draws the highlight bar, green when
// the entry is selected and white otherwise.
func fitEntry(e entry, width int) (string, string) {
	style := "\x1b[0m"
	if e.selected {
		style += "\x1b[32m"
	}
	if e.cursor {
		style += "\x1b[7m"
	}
	line := ""
	if e.dir {
		line = e.mark + cutName(e.name, profileNameCells) + "/"
	} else if e.nameW > 0 {
		line = fitAlignedEntry(e, width)
	} else {
		mark := displayWidth(e.mark)
		suffix := protoSuffix(e.proto, e.cursor) + delaySuffix(e.delayMs, e.cursor)
		suffixW := visibleWidth(suffix)
		switch {
		case suffix == "" || displayWidth(e.mark+e.name)+suffixW <= width:
			line = e.mark + e.name + suffix
		case width-mark-suffixW < 3:
			if suffix != "" && width-mark >= suffixW {
				line = e.mark + suffix
			} else {
				line = e.mark + cutEllipsis(e.name, width-mark)
			}
		default:
			line = e.mark + cutEllipsis(e.name, width-mark-suffixW) + suffix
		}
	}
	if e.cursor {
		line += strings.Repeat(" ", min(displayWidth(e.mark), max(width-visibleWidth(line), 0)))
	}
	return line, style
}

// delaySuffix renders the latency tag of one node row, plain on
// highlighted rows so the highlight keeps one color.
func delaySuffix(delayMs int, highlight bool) string {
	if delayMs < 0 {
		return ""
	}
	text := delayText(delayMs)
	if highlight {
		return "  " + text
	}
	return "  " + delayColor(delayMs) + text
}

// fitAlignedEntry renders one node row as fixed columns so the protocol
// and the delay line up across rows. Every row trims the name by the same
// width so the columns never shift, and the name gives way before the tags
// do.
func fitAlignedEntry(e entry, width int) string {
	mark := displayWidth(e.mark)
	suffixW := 2 + e.protoW
	if e.delayW > 0 {
		suffixW += 2 + e.delayW
	}
	nameW := min(e.nameW, max(width-mark-suffixW, 0))
	name := cutEllipsis(e.name, nameW)
	if nameW < 3 {
		name = truncateToWidthFromStart(e.name, nameW)
	}
	protoCell := fit(e.proto, e.protoW)
	delayCell := ""
	if e.delayMs >= 0 {
		delayCell = fit(delayText(e.delayMs), e.delayW)
	}
	if e.cursor {
		line := e.mark + fit(name, nameW) + "  " + protoCell
		if e.delayW > 0 {
			line += "  " + delayCell
		}
		return line
	}
	line := e.mark + fit(name, nameW) + "  "
	if e.proto != "" {
		line += protoColor(e.proto)
	}
	line += protoCell
	if e.delayW > 0 {
		line += "  "
		if e.delayMs >= 0 {
			line += delayColor(e.delayMs)
		}
		line += delayCell
	}
	return line
}

// alignColumns returns the widest node name, protocol and delay text of
// one list for the aligned row layout.
func alignColumns(nodes []ipc.NodeInfo, delays map[string]uint16) (int, int, int) {
	nameW, protoW, delayW := 0, 0, 0
	for _, node := range nodes {
		nameW = max(nameW, displayWidth(node.Name))
		protoW = max(protoW, displayWidth(node.Type))
		if ms, ok := delays[node.Name]; ok {
			delayW = max(delayW, displayWidth(delayText(int(ms))))
		}
	}
	return nameW, protoW, delayW
}

// protoSuffix renders the protocol tag, plain on highlighted rows so the
// highlight keeps one color.
func protoSuffix(proto string, highlight bool) string {
	if proto == "" {
		return ""
	}
	if highlight {
		return "  " + proto
	}
	return "  " + protoColor(proto) + proto
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
		selected := p.Name == b.active
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
		nodes := b.sortedNodes(name.Name)
		nameW, protoW, delayW := 0, 0, 0
		if b.align {
			nameW, protoW, delayW = alignColumns(nodes, b.app.delays[name.Name])
		}
		for i, node := range nodes {
			cursor := i == b.cursorItem
			selected := name.Name == b.active && node.Name == info.Selected
			mark := "  "
			if selected {
				mark = "✓ "
			}
			nodeCol.entries = append(nodeCol.entries, entry{
				name: node.Name, proto: node.Type, mark: mark, delayMs: b.app.delayOf(name.Name, node.Name),
				nameW: nameW, protoW: protoW, delayW: delayW,
				selected: selected, cursor: cursor,
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
		previewNameW, previewProtoW, previewDelayW := 0, 0, 0
		if b.align {
			previewNameW, previewProtoW, previewDelayW = alignColumns(b.sortedNodes(p.Name), b.app.delays[p.Name])
		}
		cursor := clampPos(saved.cursor, len(info.Nodes))
		offset := min(saved.offset, cursor)
		if cursor >= offset+listHeight {
			offset = cursor - listHeight + 1
		}
		preview.offset = max(offset, 0)
		for _, node := range b.sortedNodes(p.Name) {
			selected := p.Name == b.active && node.Name == info.Selected
			mark := "  "
			if selected {
				mark = "✓ "
			}
			preview.entries = append(preview.entries, entry{
				name: node.Name, proto: node.Type, mark: mark, delayMs: b.app.delayOf(p.Name, node.Name),
				nameW: previewNameW, protoW: previewProtoW, delayW: previewDelayW,
				selected: selected,
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
func drawColumnCell(x, width, y int, line, style string, dim bool) string {
	line = truncateStyled(line, width)
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
	return drawColumnCell(geom.x, geom.width, row+3, line, style, dim)
}
