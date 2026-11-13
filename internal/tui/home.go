package tui

import (
	"fmt"
	"strings"

	"px/internal/ipc"
	"px/internal/traffic"
)

// homeNodeCells is the node name width limit on the Home page.
const homeNodeCells = 24

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

	if w < 40 || hgt < 12 {
		msg := t(txtTooSmallDisplay)
		x := max((w-displayWidth(msg))/2, 1)
		fmt.Fprintf(&buf, "\x1b[%d;%dH\x1b[90m%s\x1b[0m", max(hgt/2, 1), x, msg)
		print(buf.String())
		return
	}

	state := t(txtStopped)
	if h.status.Running {
		state = t(txtRunning)
	}
	tun := t(txtOff)
	if h.status.Tun {
		tun = t(txtOn)
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
	active = cutName(active, profileNameCells)
	node = cutName(node, homeNodeCells)
	pad := max(displayWidth(t(txtKernel)), displayWidth("TUN"), displayWidth(t(txtPort)), displayWidth(t(txtProfile)), displayWidth(t(txtNode)), displayWidth(t(txtTrafficLabel)))
	boxW := min((w-6)/2, 60)
	boxH := min(max(boxW*2/5, 6), 20)
	boxX := 1 + max((w-1-(boxW*2+1))/2, 0)
	blockTop := 2 + max((hgt-2-(boxH+6))/2, 0)
	fmt.Fprintf(&buf, "\x1b[%d;1H\x1b[K\x1b[%d;%dH%s%s\x1b[0m", blockTop, blockTop, titleX, colHeading, title)
	drawInfoLine(&buf, blockTop+2, w, []infoPair{{t(txtKernel), state}, {"TUN", tun}, {t(txtPort), itoa(h.status.Port)}}, pad)
	drawInfoLine(&buf, blockTop+4, w, []infoPair{{t(txtProfile), active}, {t(txtNode), node}, {t(txtTrafficLabel), traffic}}, pad)

	h.drawPanel(&buf, boxX, blockTop+6, boxW, boxH, t(txtUpload), h.txSeries(), h.traffic.TxTotal)
	h.drawPanel(&buf, boxX+boxW+1, blockTop+6, boxW, boxH, t(txtDownload), h.rxSeries(), h.traffic.RxTotal)

	hint := t(txtHintHome)
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
func drawInfoLine(buf *strings.Builder, y, w int, pairs []infoPair, pad int) {
	var plain strings.Builder
	for i, p := range pairs {
		if i > 0 {
			plain.WriteString("    ")
		}
		plain.WriteString(fit(p.label, pad))
		plain.WriteString(" ")
		plain.WriteString(p.value)
	}
	x := max((w-displayWidth(plain.String()))/2, 1)
	fmt.Fprintf(buf, "\x1b[%d;1H\x1b[K\x1b[%d;%dH", y, y, x)
	for i, p := range pairs {
		if i > 0 {
			buf.WriteString("    ")
		}
		fmt.Fprintf(buf, "%s%s\x1b[0m %s%s\x1b[0m", colGreen, fit(p.label, pad), colTextBold, p.value)
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
	title := fmt.Sprintf(t(txtPanelTitle), label, fmtBytes(uint64(current)), fmtBytes(uint64(total)))
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
