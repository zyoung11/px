package tui

import (
	"fmt"
	"strings"

	"px/internal/ipc"
	"px/internal/traffic"
)

// Home is the first page: kernel status over two traffic charts.
type Home struct {
	app     *App
	status  ipc.Status
	nodes   ipc.NodesInfo
	traffic traffic.Snapshot
}

// NewHome creates the Home page.
func NewHome(app *App, status ipc.Status) *Home {
	return &Home{app: app, status: status}
}

// Init refreshes the status and the traffic history from the daemon.
func (h *Home) Init() {
	h.Refresh()
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
	if err := command(cmd, nil); err != nil {
		h.app.setMessage(err.Error())
	}
	h.Refresh()
	h.View()
}

// Refresh reloads status, traffic and the selected node from the daemon.
func (h *Home) Refresh() {
	status, err := fetchStatus()
	if err != nil {
		h.app.setMessage(err.Error())
		return
	}
	h.status = status
	snap, err := fetchTraffic()
	if err == nil {
		h.traffic = snap
	}
	if status.Active != "" {
		info, err := fetchNodes(status.Active)
		if err == nil {
			h.nodes = info
		}
	} else {
		h.nodes = ipc.NodesInfo{}
	}
}

// View draws the Home page: the kernel status block and the upload and
// download charts side by side.
func (h *Home) View() {
	term := h.app.terminal()
	term.beginFrame()
	defer term.endFrame()
	w, hgt := term.Size()

	var buf strings.Builder
	buf.WriteString("\x1b[2J\x1b[3J\x1b[H")

	title := "Home"
	titleX := max((w-displayWidth(title))/2, 1)
	fmt.Fprintf(&buf, "\x1b[1;1H\x1b[K\x1b[1;%dH\x1b[1m%s\x1b[0m", titleX, title)

	if w < 40 || hgt < 12 {
		msg := "终端过小，无法显示"
		x := max((w-displayWidth(msg))/2, 1)
		fmt.Fprintf(&buf, "\x1b[%d;%dH\x1b[90m%s\x1b[0m", max(hgt/2, 1), x, msg)
		print(buf.String())
		return
	}

	state := "已停止"
	if h.status.Running {
		state = "运行中"
	}
	tun := "关"
	if h.status.Tun {
		tun = "开"
	}
	active := h.status.Active
	if active == "" {
		active = "-"
	}
	node := h.nodes.Selected
	if node == "" {
		node = "-"
	}
	traffic := "-"
	for _, p := range h.status.Profiles {
		if p.Name == active && p.Total > 0 {
			traffic = fmtBytes(uint64(p.Upload+p.Download)) + " / " + fmtBytes(uint64(p.Total))
		}
	}
	drawInfoLine(&buf, 3, w, []infoPair{{"内核", state}, {"TUN", tun}, {"端口", itoa(h.status.Port)}})
	drawInfoLine(&buf, 4, w, []infoPair{{"配置", active}})
	drawInfoLine(&buf, 5, w, []infoPair{{"节点", node}})
	drawInfoLine(&buf, 6, w, []infoPair{{"流量", traffic}})

	boxW := (w - 6) / 2
	boxTop := 8
	boxH := hgt - boxTop
	h.drawPanel(&buf, 2, boxTop, boxW, boxH, "上传", h.txSeries(), h.traffic.TxTotal)
	h.drawPanel(&buf, 3+boxW+1, boxTop, boxW, boxH, "下载", h.rxSeries(), h.traffic.RxTotal)

	hint := "space 开启  tab 换页  ? 帮助"
	hintX := max((w-displayWidth(hint))/2, 1)
	fmt.Fprintf(&buf, "\x1b[%d;1H\x1b[K\x1b[%d;%dH\x1b[90m%s\x1b[0m", hgt, hgt, hintX, hint)

	print(buf.String())
}

// infoPair is one label and value of the Home info block.
type infoPair struct {
	label string
	value string
}

// drawInfoLine renders label value pairs centered on one row, labels in
// green and values in bold.
func drawInfoLine(buf *strings.Builder, y, w int, pairs []infoPair) {
	var plain strings.Builder
	for i, p := range pairs {
		if i > 0 {
			plain.WriteString("    ")
		}
		plain.WriteString(fit(p.label, 4))
		plain.WriteString(" ")
		plain.WriteString(p.value)
	}
	x := max((w-displayWidth(plain.String()))/2, 1)
	fmt.Fprintf(buf, "\x1b[%d;1H\x1b[K\x1b[%d;%dH", y, y, x)
	for i, p := range pairs {
		if i > 0 {
			buf.WriteString("    ")
		}
		fmt.Fprintf(buf, "\x1b[32m%s\x1b[0m \x1b[1m%s\x1b[0m", fit(p.label, 4), p.value)
	}
}

// drawPanel renders one titled traffic chart inside a box.
func (h *Home) drawPanel(buf *strings.Builder, x, top, width, height int, label string, data []float64, total int64) {
	inner := width - 2
	if inner < 8 || height < 4 {
		return
	}
	current := 0.0
	if len(data) > 0 {
		current = data[len(data)-1]
	}
	title := " " + label + " " + fmtBytes(uint64(current)) + "/s  共 " + fmtBytes(uint64(total)) + " "
	title = truncateToWidthFromStart(title, max(inner-1, 0))
	fill := max(inner-1-displayWidth(title), 0)
	fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[90m┌─\x1b[0m\x1b[1m%s%s\x1b[0m\x1b[90m%s┐\x1b[0m", top, x, chartColor, title, strings.Repeat("─", fill))

	chart := miniChart(data, inner, height-2, chartMax(data))
	for i, line := range strings.Split(chart, "\n") {
		pad := max(inner-displayWidth(line), 0)
		fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[90m│\x1b[0m%s%s\x1b[0m\x1b[90m│\x1b[0m", top+1+i, x, chartColor+line, strings.Repeat(" ", pad))
	}
	fmt.Fprintf(buf, "\x1b[%d;%dH\x1b[90m└%s┘\x1b[0m", top+height-1, x, strings.Repeat("─", inner))
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
