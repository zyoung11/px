package tui

import (
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Catppuccin Macchiato palette in true color.
const (
	colorBlue   = "\x1b[38;2;138;173;244m"
	colorGreen  = "\x1b[38;2;166;218;149m"
	colorYellow = "\x1b[38;2;238;212;159m"
	colorRed    = "\x1b[38;2;237;135;150m"
	colorGray   = "\x1b[38;2;147;154;183m"
	colorReset  = "\x1b[0m"
	inverseOn   = "\x1b[7m"
)

// Help lines shown at the bottom of the screen.
const (
	helpShort = "j/k 移动  Tab 切栏  Enter 选择  a 添加  s 同步  d 删除  t 测速  空格 开关  q 退出  ? 键位"
	helpLong  = "↑↓/j/k 移动  Tab 切栏  Enter 激活/打开分组/选中节点  a 添加订阅  s 同步  d 删除  t 测速  空格 开关内核  Esc 返回  q 退出"
)

// fg wraps text in one foreground color.
func fg(color, s string) string {
	return color + s + colorReset
}

// inverse renders text with reversed colors.
func inverse(s string) string {
	return inverseOn + s + colorReset
}

// fit truncates or pads plain text to exactly the given display width.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if used+rw > w {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	if used < w {
		b.WriteString(strings.Repeat(" ", w-used))
	}
	return b.String()
}

// delayText renders one latency value with its tier color.
func delayText(ms uint16) string {
	if ms == 0 {
		return fg(colorGray, "超时")
	}
	var color string
	switch {
	case ms < 200:
		color = colorGreen
	case ms <= 500:
		color = colorYellow
	default:
		color = colorRed
	}
	return fg(color, strconv.Itoa(int(ms))+" ms")
}

// draw paints one full frame and sends it to the terminal.
func (a *App) draw() {
	w, h := a.width, a.height
	if w < 60 || h < 12 {
		a.term.Write("\x1b[H\x1b[2J" + fg(colorYellow, "窗口过小"))
		return
	}
	var b strings.Builder
	b.WriteString("\x1b[H")
	for i, line := range a.render(w, h) {
		if i > 0 {
			b.WriteString("\r\n")
		}
		b.WriteString(line)
		b.WriteString("\x1b[K")
	}
	a.term.Write(b.String())
}

// render builds every screen row of one frame.
func (a *App) render(w, h int) []string {
	contentH := h - 3
	leftW := min(max(w/3, 20), 30)
	rightW := w - 2 - leftW
	left, right := a.renderPanes(leftW, rightW, contentH)
	lines := make([]string, 0, h)
	lines = append(lines, a.renderTop(w))
	for i := range contentH {
		lines = append(lines, left[i]+fg(colorGray, "│")+right[i])
	}
	lines = append(lines, fg(colorBlue, fit(a.helpLine(), w-1)))
	lines = append(lines, a.renderMessage(w))
	return lines
}

// renderTop builds the status bar on the first row.
func (a *App) renderTop(w int) string {
	state := "已停止"
	if a.status.Running {
		state = "运行中"
	}
	tun := "关"
	if a.status.Tun {
		tun = "开"
	}
	active := a.status.Active
	if active == "" {
		active = "-"
	}
	plain := " px   内核 " + state + "   TUN " + tun + "   端口 " + strconv.Itoa(a.status.Port) + "   配置 " + active
	return inverse(fit(plain, w-1))
}

// renderPanes builds the left profile list and the right group or node list.
func (a *App) renderPanes(leftW, rightW, contentH int) ([]string, []string) {
	left := make([]string, contentH)
	right := make([]string, contentH)
	left[0] = fg(colorBlue, fit("订阅", leftW))
	itemRows := contentH - 1
	a.scroll(&a.offLeft, a.cursorLeft, itemRows)
	for i := range itemRows {
		idx := a.offLeft + i
		if idx >= len(a.status.Profiles) {
			left[i+1] = fit("", leftW)
			continue
		}
		p := a.status.Profiles[idx]
		marker := " "
		if p.Name == a.status.Active {
			marker = "●"
		}
		row := fit(marker+" "+p.Name, leftW)
		switch {
		case idx == a.cursorLeft && a.focusLeft:
			left[i+1] = inverse(row)
		case p.Name == a.status.Active:
			left[i+1] = fg(colorGreen, row)
		default:
			left[i+1] = row
		}
	}
	if len(a.status.Profiles) == 0 {
		left[1] = fg(colorGray, fit("无配置，按 a 添加", leftW))
	}
	a.renderRight(right, rightW, contentH)
	return left, right
}

// renderRight builds the group list or the node list of the open group.
func (a *App) renderRight(right []string, rightW, contentH int) {
	itemRows := contentH - 1
	if a.openGroup == "" {
		right[0] = fg(colorBlue, fit("分组", rightW))
		a.scroll(&a.offRight, a.cursorRight, itemRows)
		for i := range itemRows {
			idx := a.offRight + i
			if idx >= len(a.status.Groups) {
				right[i+1] = fit("", rightW)
				continue
			}
			g := a.status.Groups[idx]
			row := fit(g.Name+"  → "+g.Selected, rightW)
			if idx == a.cursorRight && !a.focusLeft {
				row = inverse(row)
			}
			right[i+1] = row
		}
		if len(a.status.Groups) == 0 {
			right[1] = fg(colorGray, fit("未激活配置", rightW))
		}
		return
	}
	right[0] = fg(colorBlue, fit(a.openGroup+"   Esc 返回", rightW))
	var nodes []string
	selected := ""
	for _, g := range a.status.Groups {
		if g.Name == a.openGroup {
			nodes = g.Nodes
			selected = g.Selected
		}
	}
	a.scroll(&a.offRight, a.cursorRight, itemRows)
	for i := range itemRows {
		idx := a.offRight + i
		if idx >= len(nodes) {
			right[i+1] = fit("", rightW)
			continue
		}
		node := nodes[idx]
		marker := " "
		if node == selected {
			marker = "●"
		}
		row := fit(marker+" "+node, rightW)
		highlight := idx == a.cursorRight && !a.focusLeft
		if a.delays != nil {
			name := fit(marker+" "+node, rightW-8)
			if highlight {
				name = inverse(name)
			}
			right[i+1] = name + delayText(a.delays[node])
			continue
		}
		if highlight {
			row = inverse(row)
		}
		right[i+1] = row
	}
}

// renderMessage builds the bottom row for input, confirmation or status.
func (a *App) renderMessage(w int) string {
	switch a.mode {
	case modeInput:
		plain := "[>] 订阅链接: " + a.input.Value() + "▌"
		return fg(colorBlue, fit(plain, w-1))
	case modeConfirm:
		plain := "[?] 删除 " + a.confirm + " ? y 确认  n 取消"
		return fg(colorYellow, fit(plain, w-1))
	}
	if a.busy != "" {
		return fg(colorBlue, fit("[i] "+a.busy, w-1))
	}
	if a.message == "" {
		return fit("", w-1)
	}
	if a.messageErr {
		return fg(colorRed, fit("[-] "+a.message, w-1))
	}
	return fg(colorBlue, fit("[i] "+a.message, w-1))
}

func (a *App) helpLine() string {
	if a.helpFull {
		return helpLong
	}
	return helpShort
}

// scroll moves a list offset so the cursor row stays visible.
func (a *App) scroll(offset *int, cursor, rows int) {
	if rows <= 0 {
		*offset = 0
		return
	}
	if cursor < *offset {
		*offset = cursor
	}
	if cursor >= *offset+rows {
		*offset = cursor - rows + 1
	}
	if *offset < 0 {
		*offset = 0
	}
}
