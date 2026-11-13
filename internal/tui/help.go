package tui

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// minHelpColumnWidth is the narrowest readable help page column; layouts
// that would squeeze a column below it are replaced by a short message.
const minHelpColumnWidth = 26

// helpColumnGap is the horizontal gap kept between two help page columns.
const helpColumnGap = 4

// helpDescriptions maps keymap actions to their English and Chinese
// labels on the keyboard shortcuts help page, keyed by "<Section>.<Action>".
var helpDescriptions = map[string][2]string{
	"Global.Quit":            {"Quit the program", "退出程序"},
	"Global.ShowHelp":        {"Show this help page", "显示本帮助页"},
	"Global.CyclePages":      {"Cycle through pages", "循环切换页面"},
	"Global.SwitchToHome":    {"Switch to the Home page", "切换到首页"},
	"Global.SwitchToLibrary": {"Switch to the Library page", "切换到媒体库页面"},

	"Home.ToggleKernel": {"Toggle kernel and TUN", "开关内核与 TUN"},

	"Library.NavUp":          {"Navigate up", "向上导航"},
	"Library.NavDown":        {"Navigate down", "向下导航"},
	"Library.NavEnterDir":    {"Enter profile", "进入配置"},
	"Library.NavExitDir":     {"Exit profile", "退出配置"},
	"Library.SelectNode":     {"Switch to node", "切换到该节点"},
	"Library.AddProfile":     {"Add profile", "添加配置"},
	"Library.SyncProfile":    {"Sync profile", "同步配置"},
	"Library.DeleteProfile":  {"Delete profile", "删除配置"},
	"Library.Sort":           {"Toggle sort mode", "切换排序方式"},
	"Library.ConfirmInput":   {"Confirm input", "确认输入"},
	"Library.EscapeInput":    {"Cancel input", "取消输入"},
	"Library.InputBackspace": {"Delete input character", "删除输入字符"},
	"Library.CursorLeft":     {"Move input cursor left", "光标左移"},
	"Library.CursorRight":    {"Move input cursor right", "光标右移"},
}

// helpSectionTitles holds the English and Chinese names of the help page
// sections.
var helpSectionTitles = map[string][2]string{
	"Global":  {"Global", "全局"},
	"Home":    {"Home", "首页"},
	"Library": {"Profiles", "配置"},
}

// helpDescription returns the localized label for a keymap action,
// falling back to the action name when no translation exists.
func helpDescription(section, action string, zh bool) string {
	if labels, ok := helpDescriptions[section+"."+action]; ok {
		if zh {
			return labels[1]
		}
		return labels[0]
	}
	return action
}

// helpTitle returns the localized name of a help page section.
func helpTitle(section string, zh bool) string {
	if labels, ok := helpSectionTitles[section]; ok {
		if zh {
			return labels[1]
		}
		return labels[0]
	}
	return section
}

// helpRow describes a single keybinding line on the keyboard shortcuts
// help page.
type helpRow struct {
	name  string
	keys  string
	input bool
}

// helpBlock groups the keybindings of one keymap section under a heading.
type helpBlock struct {
	title string
	rows  []helpRow
}

// height returns the number of lines the block occupies, heading
// included.
func (b helpBlock) height() int {
	return 1 + len(b.rows)
}

// helpLine is one rendered line of the help page, either a section
// heading or a keybinding row.
type helpLine struct {
	blank bool
	title string
	row   helpRow
}

// collectHelpBlocks returns the sections shown on the keyboard shortcuts
// help page in the given language.
func collectHelpBlocks(zh bool) []helpBlock {
	g := keymap.Global
	h := keymap.Home
	l := keymap.Library
	return []helpBlock{
		{title: helpTitle("Global", zh), rows: []helpRow{
			helpRowOf("Global", "Quit", g.Quit, false, zh),
			helpRowOf("Global", "ShowHelp", g.ShowHelp, false, zh),
			helpRowOf("Global", "CyclePages", g.CyclePages, false, zh),
			helpRowOf("Global", "SwitchToHome", g.SwitchToHome, false, zh),
			helpRowOf("Global", "SwitchToLibrary", g.SwitchToLibrary, false, zh),
		}},
		{title: helpTitle("Home", zh), rows: []helpRow{
			helpRowOf("Home", "ToggleKernel", h.ToggleKernel, false, zh),
		}},
		{title: helpTitle("Library", zh), rows: []helpRow{
			helpRowOf("Library", "NavUp", l.NavUp, false, zh),
			helpRowOf("Library", "NavDown", l.NavDown, false, zh),
			helpRowOf("Library", "NavEnterDir", l.NavEnterDir, false, zh),
			helpRowOf("Library", "NavExitDir", l.NavExitDir, false, zh),
			helpRowOf("Library", "SelectNode", l.SelectNode, false, zh),
			helpRowOf("Library", "AddProfile", l.AddProfile, false, zh),
			helpRowOf("Library", "SyncProfile", l.SyncProfile, false, zh),
			helpRowOf("Library", "DeleteProfile", l.DeleteProfile, false, zh),
			helpRowOf("Library", "Sort", l.Sort, false, zh),
			helpRowOf("Library", "ConfirmInput", l.Input.ConfirmInput, true, zh),
			helpRowOf("Library", "EscapeInput", l.Input.EscapeInput, true, zh),
			helpRowOf("Library", "InputBackspace", l.Input.InputBackspace, true, zh),
			helpRowOf("Library", "CursorLeft", l.Input.CursorLeft, true, zh),
			helpRowOf("Library", "CursorRight", l.Input.CursorRight, true, zh),
		}},
	}
}

// helpRowOf builds one keybinding row of the help page.
func helpRowOf(section, action string, keys Key, input, zh bool) helpRow {
	return helpRow{
		name:  helpDescription(section, action, zh),
		keys:  strings.Join(keys, ", "),
		input: input,
	}
}

// bestBlockPartition splits blocks into the given number of contiguous
// column groups and returns the cut positions minimizing the tallest
// column.
func bestBlockPartition(heights []int, groups int) ([]int, int) {
	n := len(heights)
	groupHeight := func(lo, hi int) int {
		total := 0
		for i := lo; i <= hi; i++ {
			total += heights[i]
		}
		return total + (hi - lo)
	}

	var bestCuts []int
	bestMax := -1
	var walk func(lo, col int, cuts []int, curMax int)
	walk = func(lo, col int, cuts []int, curMax int) {
		if col == groups-1 {
			tail := 0
			if lo <= n-1 {
				tail = groupHeight(lo, n-1)
			}
			m := max(curMax, tail)
			if bestMax < 0 || m < bestMax {
				bestMax = m
				bestCuts = append([]int(nil), cuts...)
			}
			return
		}
		for hi := lo - 1; hi < n; hi++ {
			height := 0
			if lo <= hi {
				height = groupHeight(lo, hi)
			}
			next := append(append([]int(nil), cuts...), hi)
			walk(hi+1, col+1, next, max(curMax, height))
		}
	}
	walk(0, 0, nil, 0)
	return bestCuts, bestMax
}

// columnLines builds the lines of one column from the blocks in the
// given range.
func columnLines(blocks []helpBlock, lo, hi int) []helpLine {
	lines := make([]helpLine, 0)
	for i := lo; i <= hi; i++ {
		if i > lo {
			lines = append(lines, helpLine{blank: true})
		}
		lines = append(lines, helpLine{title: blocks[i].title})
		for _, row := range blocks[i].rows {
			lines = append(lines, helpLine{row: row})
		}
	}
	return lines
}

// buildPartitionColumns packs whole blocks into columns along the given
// cuts.
func buildPartitionColumns(blocks []helpBlock, cuts []int) [][]helpLine {
	columns := make([][]helpLine, 0, len(cuts)+1)
	start := 0
	for _, end := range cuts {
		columns = append(columns, columnLines(blocks, start, end))
		start = end + 1
	}
	columns = append(columns, columnLines(blocks, start, len(blocks)-1))
	return columns
}

// buildFlowColumns streams the lines through the columns, splitting
// oversized blocks only when no whole-block layout fits.
func buildFlowColumns(blocks []helpBlock, contentRows, cols int) [][]helpLine {
	all := make([]helpLine, 0)
	for i, block := range blocks {
		if i > 0 {
			all = append(all, helpLine{blank: true})
		}
		all = append(all, helpLine{title: block.title})
		for _, row := range block.rows {
			all = append(all, helpLine{row: row})
		}
	}

	columns := make([][]helpLine, 0, cols)
	col := make([]helpLine, 0, contentRows)
	for _, line := range all {
		if len(col) >= contentRows {
			columns = append(columns, col)
			if len(columns) == cols {
				break
			}
			col = make([]helpLine, 0, contentRows)
		}
		if line.title != "" && len(col) == contentRows-1 {
			col = append(col, helpLine{blank: true})
			continue
		}
		col = append(col, line)
	}
	if len(columns) < cols {
		columns = append(columns, col)
	}
	return columns
}

// planHelpColumns assigns the help blocks to readable columns and
// returns the lines of each column, or nil when the terminal is too small
// to be readable.
func planHelpColumns(blocks []helpBlock, contentRows, maxCols int) [][]helpLine {
	if maxCols < 2 {
		return nil
	}

	heights := make([]int, len(blocks))
	totalRows := 0
	for i, block := range blocks {
		heights[i] = block.height()
		totalRows += heights[i]
	}
	totalRows += max(len(blocks)-1, 0)

	for groups := 2; groups <= min(maxCols, len(blocks)); groups++ {
		cuts, best := bestBlockPartition(heights, groups)
		if best <= contentRows {
			return buildPartitionColumns(blocks, cuts)
		}
	}

	flowCols := max((totalRows+contentRows-1)/contentRows, 2)
	if flowCols > maxCols {
		return nil
	}
	return buildFlowColumns(blocks, contentRows, flowCols)
}

// renderHelpLine draws one help page line at the given position,
// truncated to the column width.
func renderHelpLine(y, x, colWidth, keysWidth int, line helpLine) {
	if line.blank {
		return
	}
	if line.title != "" {
		heading := truncateToWidthFromStart(line.title, colWidth-1)
		dashes := max(colWidth-displayWidth(heading)-2, 0)
		fmt.Printf("\x1b[%d;%dH%s%s\x1b[0m \x1b[90m%s\x1b[0m", y, x, colHeading, heading, strings.Repeat("─", dashes))
		return
	}
	keys := truncateToWidthFromStart(line.row.keys, keysWidth)
	nameWidth := max(colWidth-keysWidth-2, 1)
	if line.row.input {
		nameWidth = max(nameWidth-1, 1)
	}
	name := truncateToWidthFromStart(line.row.name, nameWidth)
	fmt.Printf("\x1b[%d;%dH%s%s\x1b[0m", y, x, colGreen, keys)
	if line.row.input {
		name += colGreen + "*\x1b[0m"
	}
	fmt.Printf("\x1b[%d;%dH%s", y, x+keysWidth+2, name)
}

// drawHelpPage renders the full-screen keyboard shortcuts help page from
// the configured keybindings.
func (a *App) drawHelpPage() {
	a.term.beginFrame()
	defer a.term.endFrame()
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		w, h = 80, 24
	}

	fmt.Print("\x1b[2J\x1b[3J\x1b[H")

	zh := lang == 1
	title := t(txtHelpTitle)
	titleX := max((w-displayWidth(title))/2, 0) + 1
	fmt.Printf("\x1b[1;%dH%s%s\x1b[0m", titleX, colHeading, title)

	blocks := collectHelpBlocks(zh)
	contentRows := max(h-4, 1)
	columns := planHelpColumns(blocks, contentRows, w/minHelpColumnWidth)

	if columns != nil {
		keysWidths := make([]int, len(columns))
		nameWidths := make([]int, len(columns))
		for col, lines := range columns {
			for _, line := range lines {
				if line.blank || line.title != "" {
					continue
				}
				keysWidths[col] = max(keysWidths[col], displayWidth(line.row.keys))
				nameWidth := displayWidth(line.row.name)
				if line.row.input {
					nameWidth++
				}
				nameWidths[col] = max(nameWidths[col], nameWidth)
			}
		}

		each := max((w-helpColumnGap*(len(columns)-1))/len(columns), 1)
		widths := make([]int, len(columns))
		totalWidth := 0
		for col := range columns {
			widths[col] = min(max(keysWidths[col]+2+nameWidths[col], 3), each)
			totalWidth += widths[col]
		}
		totalWidth += helpColumnGap * (len(columns) - 1)

		x := max((w-totalWidth)/2, 0) + 1
		for col, lines := range columns {
			for i, line := range lines {
				renderHelpLine(3+i, x, widths[col], keysWidths[col], line)
			}
			x += widths[col] + helpColumnGap
		}
	} else {
		msg := t(txtHelpTooSmall)
		msg = truncateToWidthFromStart(msg, max(w-1, 1))
		x := max((w-displayWidth(msg))/2, 0) + 1
		fmt.Printf("\x1b[%d;%dH\x1b[90m%s\x1b[0m", max(h/2, 1), x, msg)
	}

	legend := t(txtHelpLegend)
	hint := fmt.Sprintf(t(txtHelpBack), strings.Join(keymap.Global.Quit, ", "))
	hintX := max((w-displayWidth(hint))/2, 0) + 1
	if columns != nil {
		legendX := max((w-displayWidth(legend))/2, 0) + 1
		fmt.Printf("\x1b[%d;%dH%s*\x1b[0m\x1b[90m%s\x1b[0m", h-1, legendX, colGreen, legend[1:])
	}
	fmt.Printf("\x1b[%d;%dH\x1b[90m%s\x1b[0m", h, hintX, hint)
}

// buildQuitKeysLine renders the quit prompt key hint sized to fit the
// given width, dropping the action words when space is tight.
func buildQuitKeysLine(width int, confirmKey, cancelKey, suffixConfirm, suffixCancel string) string {
	gap := 6
	total := displayWidth(confirmKey+suffixConfirm) + gap + displayWidth(cancelKey+suffixCancel)
	if total > width {
		suffixConfirm = ""
		suffixCancel = ""
		gap = 2
		total = displayWidth(confirmKey) + gap + displayWidth(cancelKey)
	}
	pad := max((width-total)/2, 0)
	tail := max(width-total-pad, 0)
	return strings.Repeat(" ", pad) +
		colGreen + confirmKey + "\x1b[0m" + suffixConfirm + strings.Repeat(" ", gap) +
		colGreen + cancelKey + "\x1b[0m" + suffixCancel + strings.Repeat(" ", tail)
}

// drawConfirmBox renders the modal confirmation box centered over the
// current page, the same shape as BM's quit prompt.
func (a *App) drawConfirmBox(msg, suffixConfirm, suffixCancel string) {
	a.term.beginFrame()
	defer a.term.endFrame()
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		w, h = 80, 24
	}
	if w < 40 || h < 7 {
		return
	}

	const boxW = 38
	const boxH = 5
	width := min(boxW, w)
	inner := width - 2
	left := max((w-width)/2, 0) + 1
	top := max((h-boxH)/2, 0) + 1

	fmt.Printf("\x1b[%d;%dH\x1b[90m┌%s┐\x1b[0m", top, left, strings.Repeat("─", inner))

	msg = truncateToWidthFromStart(msg, inner)
	msgWidth := displayWidth(msg)
	msgPad := max((inner-msgWidth)/2, 0)
	msgLine := strings.Repeat(" ", msgPad) + msg + strings.Repeat(" ", max(inner-msgWidth-msgPad, 0))
	fmt.Printf("\x1b[%d;%dH\x1b[90m│\x1b[0m%s%s\x1b[0m\x1b[90m│\x1b[0m", top+1, left, colTextBold, msgLine)

	fmt.Printf("\x1b[%d;%dH\x1b[90m│%s│\x1b[0m", top+2, left, strings.Repeat(" ", inner))

	confirmKey := "enter"
	cancelKey := "esc"
	if keys := keymap.Global.Quit; len(keys) > 0 {
		cancelKey = keys[0]
	}
	keysLine := buildQuitKeysLine(inner, confirmKey, cancelKey, suffixConfirm, suffixCancel)
	fmt.Printf("\x1b[%d;%dH\x1b[90m│\x1b[0m%s\x1b[90m│\x1b[0m", top+3, left, keysLine)

	fmt.Printf("\x1b[%d;%dH\x1b[90m└%s┘\x1b[0m", top+4, left, strings.Repeat("─", inner))
}
