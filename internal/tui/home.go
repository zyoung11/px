package tui

import (
	"fmt"
	"strings"

	"px/internal/ipc"
	"px/internal/state"
	"px/internal/traffic"
)

// homeWideMin is the inner panel width that fits the two column layout.
const homeWideMin = 64

// badge background colors as raw RGB triplets for the 48 and 38 codes.
const (
	rgbGreen = "166;218;149"
	rgbRed   = "237;135;150"
	rgbBlue  = "138;173;244"
	rgbGray  = "110;115;141"
	rgbInk   = "36;39;58"
)

// version is the build version shown on the Home panel top border.
var version string

// SetVersion records the build version drawn on the Home panel.
func SetVersion(value string) {
	version = value
}

// Home is the first page: one status panel over two traffic charts.
type Home struct {
	app        *App
	status     ipc.Status
	nodes      ipc.NodesInfo
	traffic    traffic.Snapshot
	refreshing bool
}

// NewHome creates the Home page.
func NewHome(app *App, status ipc.Status) *Home {
	return &Home{app: app, status: status}
}

// Init reloads the page data in the background.
func (h *Home) Init() {
	h.refreshAsync()
}

// refreshAsync reloads the status and the traffic history without
// blocking the event loop and redraws when the answers arrive.
func (h *Home) refreshAsync() {
	if h.refreshing {
		return
	}
	h.refreshing = true
	go func() {
		status, err := fetchStatus()
		snap, _ := fetchTraffic()
		var nodes ipc.NodesInfo
		if err == nil && status.Active != "" {
			nodes, _ = fetchNodes(status.Active)
		}
		h.app.events <- event{done: func(error) {
			h.refreshing = false
			if err != nil {
				h.app.setMessage(err.Error())
				h.app.redrawCurrent(h)
				return
			}
			h.status = status
			h.traffic = snap
			h.nodes = nodes
			h.measureDelay()
			h.app.redrawCurrent(h)
		}}
	}()
}

// measureDelay tests the latency of the selected node in the background
// when no measurement is stored yet.
func (h *Home) measureDelay() {
	node := h.nodes.Selected
	profile := h.status.Active
	if node == "" || node == "DIRECT" || profile == "" || h.app.delayOf(profile, node) >= 0 {
		return
	}
	target := []ipc.DelayTarget{{Profile: profile, Nodes: []string{node}}}
	go func() {
		result, err := fetchDelays(target)
		h.app.events <- event{err: err, done: func(error) {
			if err == nil {
				h.app.mergeDelays(result)
				h.app.redrawCurrent(h)
			}
		}}
	}()
}

// Modal reports whether the page consumes every key itself.
func (h *Home) Modal() bool {
	return false
}

// HandleKey processes one key on the Home page.
func (h *Home) HandleKey(key rune) {
	if !IsKey(key, keymap.Home.ToggleKernel) {
		return
	}
	cmd := ipc.CmdStart
	if h.status.Running {
		cmd = ipc.CmdStop
	}
	h.app.run(cmd, nil, func(err error) {
		h.app.setMessage("")
		if err != nil {
			h.app.setMessage(err.Error())
		}
		h.refreshAsync()
	})
	h.View()
}

// footerLine returns the bottom line of the page.
func (h *Home) footerLine() string {
	if h.app.busy {
		return h.app.loadingLine()
	}
	return joinHints(
		hintItem(hintKeys(keymap.Home.ToggleKernel), txtHintStart),
		hintItem(hintKeys(keymap.Global.CyclePages), txtHintPages),
		hintItem(hintKeys(keymap.Global.ShowHelp), txtHintHelp),
	)
}

// drawFooterOnly repaints the bottom line alone.
func (h *Home) drawFooterOnly() {
	h.app.terminal().beginFrame()
	defer h.app.terminal().endFrame()
	w, hgt := h.app.terminal().Size()
	line := truncateStyled(h.footerLine(), w)
	x := max((w-visibleWidth(line))/2, 1)
	print("\x1b[" + itoa(hgt) + ";1H\x1b[K\x1b[" + itoa(hgt) + ";" + itoa(x) + "H\x1b[90m" + line + "\x1b[0m")
}

// View draws the Home page: one status panel over the upload and the
// download charts side by side.
func (h *Home) View() {
	term := h.app.terminal()
	term.beginFrame()
	defer term.endFrame()
	w, hgt := term.Size()

	var buf strings.Builder
	buf.WriteString("\x1b[2J\x1b[3J\x1b[H")

	if w < 30 || hgt < 12 {
		msg := t(txtTooSmallDisplay)
		x := max((w-displayWidth(msg))/2, 1)
		fmt.Fprintf(&buf, "\x1b[%d;%dH\x1b[90m%s\x1b[0m", max(hgt/2, 1), x, msg)
		print(buf.String())
		return
	}

	chartCells := h.status.ChartWidth
	if chartCells <= 0 {
		chartCells = state.DefaultChartWidth
	}
	chartCells = max(20, min(chartCells, 200))
	boxW := min((w-6)/2, chartCells)
	blockW := boxW*2 + 1
	boxX := 1 + max((w-1-blockW)/2, 0)
	panelH := 6
	if blockW-2 < homeWideMin {
		panelH = 7
	}
	stacked := boxW < 20
	chartW := boxW
	chartH := min(max(boxW*2/5, 6), 20, hgt-panelH-2)
	chartsH := chartH + 1
	if stacked {
		chartW = blockW
		chartH = min(max(blockW*2/5, 6), 20, (hgt-panelH-3)/2)
		chartsH = chartH*2 + 1
	}
	top := 1 + max((hgt-1-(panelH+1+chartsH))/2, 0)

	h.drawStatus(&buf, boxX, top, blockW)
	chartTop := top + panelH + 1
	if stacked {
		h.drawPanel(&buf, boxX, chartTop, chartW, chartH, t(txtUpload), colUpload, h.txSeries())
		h.drawPanel(&buf, boxX, chartTop+chartH+1, chartW, chartH, t(txtDownload), colDownload, h.rxSeries())
	} else {
		h.drawPanel(&buf, boxX, chartTop, chartW, chartH, t(txtUpload), colUpload, h.txSeries())
		h.drawPanel(&buf, boxX+chartW+1, chartTop, chartW, chartH, t(txtDownload), colDownload, h.rxSeries())
	}

	hint := truncateStyled(h.footerLine(), w)
	hintX := max((w-visibleWidth(hint))/2, 1)
	fmt.Fprintf(&buf, "\x1b[%d;1H\x1b[K\x1b[%d;%dH\x1b[90m%s\x1b[0m", hgt, hgt, hintX, hint)

	print(buf.String())
}

// drawStatus renders the bordered status panel: the top border with the
// page title, the badge row and the node profile and traffic rows.
func (h *Home) drawStatus(buf *strings.Builder, x, y, width int) {
	inner := width - 2
	drawStatusTop(buf, x, y, inner)
	fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[K%s", y+1, x+1, h.badgeLine(inner))
	fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[K\x1b[90m└%s┘\x1b[0m", y+panelRows(inner)+1, x, strings.Repeat("─", max(width-2, 0)))

	node, proto, profile, trafficText, used, total := h.statusText()
	pad := max(displayWidth(t(txtNode)), displayWidth(t(txtProfile)), displayWidth(t(txtTrafficLabel)), displayWidth(t(txtDelayLabel)))
	ms := h.app.delayOf(h.status.Active, node)
	if inner >= homeWideMin {
		leftW := inner * 55 / 100
		rightX := x + 1 + leftW + 2
		rightW := inner - leftW - 2
		lineW := displayWidth(t(txtTrafficLabel)) + 1 + displayWidth(trafficText)
		drawDelayRow(buf, usageLineX(rightX, rightW, trafficText), y+2, rightW, 0, delayText(ms), ms, lineW)
		drawKV(buf, x+1, y+3, leftW, pad, t(txtNode), node, proto)
		drawKV(buf, x+1, y+4, leftW, pad, t(txtProfile), profile, "")
		h.drawUsage(buf, rightX, rightW, y+3, trafficText, used, total)
		return
	}
	drawDelayRow(buf, x+1, y+2, inner, pad, delayText(ms), ms, 0)
	drawKV(buf, x+1, y+3, inner, pad, t(txtNode), node, proto)
	drawKV(buf, x+1, y+4, inner, pad, t(txtProfile), profile, "")
	h.drawTrafficRow(buf, x+1, y+5, inner, pad, trafficText, used, total)
}

// panelRows returns the count of the content rows between the panel borders.
func panelRows(inner int) int {
	if inner >= homeWideMin {
		return 4
	}
	return 5
}

// drawStatusTop renders the panel top border with the page title and the
// build version.
func drawStatusTop(buf *strings.Builder, x, y, inner int) {
	title := " px "
	ver := ""
	if version != "" {
		ver = " v" + version + " "
	}
	fill := inner - 1 - displayWidth(title) - displayWidth(ver)
	if fill < 1 {
		ver = ""
		fill = inner - 1 - displayWidth(title)
	}
	if fill < 1 {
		fill = 1
	}
	fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[90m┌─\x1b[0m%s%s\x1b[0m\x1b[90m%s\x1b[0m%s%s\x1b[0m\x1b[90m┐\x1b[0m",
		y, x, colHeading, title, strings.Repeat("─", fill), colVersion, ver)
}

// badgeLine renders the status pills and the listening port of the panel.
func (h *Home) badgeLine(width int) string {
	port := colSub + t(txtPort) + "\x1b[0m " + colTextBold + itoa(h.status.Port) + "\x1b[0m"
	if width < homeWideMin {
		port = colTextBold + itoa(h.status.Port) + "\x1b[0m"
	}
	segs := []string{pill(t(txtRunning), rgbGreen), pill("TUN "+t(txtOn), rgbBlue), port}
	if !h.status.Running {
		segs[0] = pill(t(txtStopped), rgbRed)
	}
	if !h.status.Tun {
		segs[1] = pill("TUN "+t(txtOff), rgbGray)
	}

	var b strings.Builder
	used := 0
	for _, seg := range segs {
		segW := visibleWidth(seg)
		gap := 2
		if b.Len() == 0 {
			gap = 0
		}
		if used+gap+segW > width {
			break
		}
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(seg)
		used += gap + segW
	}
	return b.String()
}

// pill renders one status badge as a solid block of color.
func pill(text, bg string) string {
	return "\x1b[48;2;" + bg + "m\x1b[38;2;" + rgbInk + "m\x1b[1m " + text + " \x1b[0m"
}

// statusText returns the node and profile names and the traffic figures
// shown on the status panel.
func (h *Home) statusText() (node, proto, profile, trafficText string, used, total int64) {
	profile = h.status.Active
	for _, p := range h.status.Profiles {
		if p.Name == profile {
			used = p.Upload + p.Download
			total = p.Total
		}
	}
	if profile == "" {
		profile = "-"
	}
	node = h.nodes.Selected
	if node == "" {
		node = "-"
	}
	for _, n := range h.nodes.Nodes {
		if n.Name == node {
			proto = n.Type
		}
	}
	switch {
	case total > 0:
		trafficText = fmtBytes(uint64(used)) + " / " + fmtBytes(uint64(total))
	case used > 0:
		trafficText = fmtBytes(uint64(used))
	default:
		trafficText = "-"
	}
	return node, proto, profile, trafficText, used, total
}

// drawKV renders one label and value pair on a row and returns the column
// right after the rendered value. A long value ends in an ellipsis and
// gives way before the protocol tag does, like the Profiles column rows.
func drawKV(buf *strings.Builder, x, y, width, pad int, label, value, proto string) int {
	protoW := 0
	if proto != "" {
		protoW = 2 + displayWidth(proto)
	}
	name := ""
	if nameW := width - pad - 2 - protoW; nameW >= 3 {
		name = cutEllipsis(value, nameW)
	}
	line := colGreen + fit(label, pad) + "\x1b[0m  " + colTextBold + name + "\x1b[0m" + protoSuffix(proto, false) + "\x1b[0m"
	line = truncateStyled(line, width)
	fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[K%s", y, x, line)
	return x + min(visibleWidth(line), width)
}

// usageLineX returns the left column of the right block so the delay and
// the usage rows line up.
func usageLineX(x, width int, value string) int {
	lineW := displayWidth(t(txtTrafficLabel)) + 1 + displayWidth(value)
	return x + width - max(lineW, 0)
}

// drawUsage renders the usage figures and the quota gauge as one block
// aligned to the right edge of the panel.
func (h *Home) drawUsage(buf *strings.Builder, x, width, y int, value string, used, total int64) {
	label := t(txtTrafficLabel)
	valueW := displayWidth(value)
	lineW := displayWidth(label) + 1 + valueW
	lineX := usageLineX(x, width, value)
	fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[K%s%s\x1b[0m %s%s\x1b[0m",
		y, lineX, colGreen, label, colTextBold, value)
	drawGaugeRow(buf, lineX, y+1, lineW, used, total)
}

// drawDelayRow renders the delay label and its value in the color of the
// latency. When blockW is positive the value is right aligned inside the
// block so the row lines up with the usage rows.
func drawDelayRow(buf *strings.Builder, x, y, width, pad int, text string, ms, blockW int) {
	label := t(txtDelayLabel)
	line := delayColor(ms) + fit(label, pad) + "  " + text + "\x1b[0m"
	if blockW > 0 {
		pad := blockW - displayWidth(label) - 2 - displayWidth(text)
		line = strings.Repeat(" ", max(pad, 0)) + delayColor(ms) + label + "  " + text + "\x1b[0m"
	}
	line = truncateStyled(line, width)
	fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[K%s", y, x, line)
}

// drawGaugeRow renders the quota gauge on one row, its block sized to
// the value block above so both rows line up on the right.
func drawGaugeRow(buf *strings.Builder, x, y, width int, used, total int64) {
	if total <= 0 {
		return
	}
	ratio := min(float64(used)/float64(total), 1)
	pct := itoa(int(ratio*100+0.5)) + "%"
	barW := width - displayWidth(pct) - 1
	if barW < 4 {
		return
	}
	fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[K%s %s%s\x1b[0m", y, x, gradientBar(ratio, barW), colTextBold, pct)
}

// drawTrafficRow renders the traffic figures and the quota gauge on one row.
func (h *Home) drawTrafficRow(buf *strings.Builder, x, y, width, pad int, value string, used, total int64) {
	end := drawKV(buf, x, y, width, pad, t(txtTrafficLabel), value, "")
	if total <= 0 {
		return
	}
	ratio := min(float64(used)/float64(total), 1)
	pct := itoa(int(ratio*100+0.5)) + "%"
	barW := x + width - end - displayWidth(pct) - 2
	if barW < 4 {
		return
	}
	fmt.Fprintf(buf, " %s %s%s\x1b[0m", gradientBar(ratio, barW), colTextBold, pct)
}

// drawPanel renders one titled traffic chart inside a box.
func (h *Home) drawPanel(buf *strings.Builder, x, top, width, height int, label, accent string, data []float64) {
	inner := width - 2
	if inner < 8 || height < 4 {
		return
	}
	current := 0.0
	if len(data) > 0 {
		current = data[len(data)-1]
	}
	drawBorderRow(buf, x, top, inner, accent+" "+label+" \x1b[0m", "┌", "┐")
	chart := miniChart(data, inner, height-2, chartMax(data))
	lines := strings.Split(chart, "\n")
	for row := range height - 2 {
		line := ""
		if row < len(lines) {
			line = lines[row]
		}
		pad := max(inner-displayWidth(line), 0)
		fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[90m│\x1b[0m%s%s\x1b[0m\x1b[90m│\x1b[0m", top+1+row, x, accent+line, strings.Repeat(" ", pad))
	}
	value, unit := fmtRate(current)
	drawBorderRow(buf, x, top+height-1, inner, " "+accent+value+" "+unit+"/s\x1b[0m ", "└", "┘")
}

// drawBorderRow renders one box border row with a styled label centered
// between the left and the right corner glyphs.
func drawBorderRow(buf *strings.Builder, x, y, inner int, text, left, right string) {
	fill := inner - visibleWidth(text)
	if fill < 2 {
		text = truncateStyled(text, max(inner-2, 0))
		fill = inner - visibleWidth(text)
	}
	l := fill / 2
	r := fill - l
	fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[90m%s%s\x1b[0m%s\x1b[0m\x1b[90m%s%s\x1b[0m",
		y, x, left, strings.Repeat("─", l), text, strings.Repeat("─", r), right)
}

// txSeries returns the upload samples in bytes per second.
func (h *Home) txSeries() []float64 {
	out := make([]float64, 0, len(h.traffic.Samples))
	for _, s := range h.traffic.Samples {
		out = append(out, float64(s.Tx))
	}
	return out
}

// rxSeries returns the download samples in bytes per second.
func (h *Home) rxSeries() []float64 {
	out := make([]float64, 0, len(h.traffic.Samples))
	for _, s := range h.traffic.Samples {
		out = append(out, float64(s.Rx))
	}
	return out
}
