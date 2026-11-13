package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"px/internal/ipc"
)

// messageTTL is how long a footer note stays before the hint line
// returns.
const messageTTL = 3 * time.Second

// browsePos is the remembered cursor and offset of one list.
type browsePos struct {
	cursor int
	offset int
}

// Browser is the Library page: a two level list of profiles and nodes in
// BM's multi column style.
type Browser struct {
	app          *App
	profiles     []ipc.ProfileInfo
	nodes        map[string]ipc.NodesInfo
	history      map[string]browsePos
	cursorDir    int
	cursorItem   int
	offsetDir    int
	offsetItem   int
	inside       bool
	input        inputLine
	confirm      string
	message      string
	messageErr   bool
	messageUntil time.Time
	sortMode     int
}

// inputLine is the one line add profile input drawn in BM's search
// footer style.
type inputLine struct {
	active bool
	value  []rune
	pos    int
}

// NewBrowser creates the Library page.
func NewBrowser(app *App) *Browser {
	return &Browser{app: app, nodes: map[string]ipc.NodesInfo{}, history: map[string]browsePos{}}
}

// Init reloads the profile list and the remembered UI state from the
// daemon.
func (b *Browser) Init() {
	status, err := fetchStatus()
	if err != nil {
		b.setMessage(err.Error(), true)
		return
	}
	b.profiles = status.Profiles
	b.sortMode = status.Sort
	b.clamp()
}

// Modal reports whether the page consumes every key itself.
func (b *Browser) Modal() bool {
	return b.input.active || b.confirm != ""
}

// HandleKey processes one key on the Library page.
func (b *Browser) HandleKey(key rune) {
	switch {
	case b.confirm != "":
		b.handleConfirmKey(key)
		return
	case b.input.active:
		b.handleInputKey(key)
		return
	}
	switch {
	case IsKey(key, keymap.Library.NavUp):
		b.moveCursor(-1)
	case IsKey(key, keymap.Library.NavDown):
		b.moveCursor(1)
	case IsKey(key, keymap.Library.NavEnterDir):
		b.enterDir()
	case IsKey(key, keymap.Library.NavExitDir):
		b.exitDir()
	case IsKey(key, keymap.Library.SelectNode):
		if b.inside {
			b.selectNode()
		} else {
			b.enterDir()
		}
	case IsKey(key, keymap.Library.AddProfile):
		b.input.active = true
		b.input.value = nil
		b.input.pos = 0
		b.drawFooterOnly()
	case IsKey(key, keymap.Library.SyncProfile):
		b.syncProfile()
	case IsKey(key, keymap.Library.Sort):
		b.sortMode = 1 - b.sortMode
		b.clamp()
		b.app.saveUI()
		b.setMessage("排序 "+sortModeName(b.sortMode), false)
		b.View()
	case IsKey(key, keymap.Library.DeleteProfile):
		if p, ok := b.currentProfile(); ok {
			b.confirm = p.Name
			b.app.showConfirm("确定要删除 "+p.Name+" 吗？", " 删除", " 取消")
		}
	}
}

func (b *Browser) handleConfirmKey(key rune) {
	name := b.confirm
	b.confirm = ""
	if key == KeyEnter {
		b.app.terminal().beginFrame()
		b.app.terminal().endFrame()
		if err := command(ipc.CmdDelete, map[string]string{"name": name}); err != nil {
			b.setMessage(err.Error(), true)
		} else {
			b.setMessage("已删除 "+name, false)
			b.Init()
		}
	}
	b.View()
}

func (b *Browser) handleInputKey(key rune) {
	switch {
	case IsKey(key, keymap.Library.Input.ConfirmInput):
		value := strings.TrimSpace(string(b.input.value))
		b.input.active = false
		b.input.value = nil
		b.input.pos = 0
		if value == "" {
			b.View()
			return
		}
		err := commandReport(ipc.CmdAdd, map[string]string{"url": value}, func(text string) {
			b.setMessage(text, false)
			b.drawFooterOnly()
		})
		if err != nil {
			b.setMessage(err.Error(), true)
		} else {
			b.setMessage("已添加", false)
			b.Init()
		}
		b.View()
	case IsKey(key, keymap.Library.Input.EscapeInput):
		b.input.active = false
		b.input.value = nil
		b.input.pos = 0
		b.View()
	case IsKey(key, keymap.Library.Input.InputBackspace):
		if b.input.pos > 0 {
			b.input.value = append(b.input.value[:b.input.pos-1], b.input.value[b.input.pos:]...)
			b.input.pos--
		}
		b.drawFooterOnly()
	case IsKey(key, keymap.Library.Input.CursorLeft):
		if b.input.pos > 0 {
			b.input.pos--
		}
		b.drawFooterOnly()
	case IsKey(key, keymap.Library.Input.CursorRight):
		if b.input.pos < len(b.input.value) {
			b.input.pos++
		}
		b.drawFooterOnly()
	case key >= 0x20 && key != 0x7f:
		b.input.value = append(b.input.value[:b.input.pos], append([]rune{key}, b.input.value[b.input.pos:]...)...)
		b.input.pos++
		b.drawFooterOnly()
	}
}

func (b *Browser) moveCursor(delta int) {
	if b.inside {
		count := 0
		if p, ok := b.currentProfile(); ok {
			count = len(b.nodes[p.Name].Nodes)
		}
		if count > 0 {
			b.cursorItem = (b.cursorItem + delta + count) % count
		}
	} else if count := len(b.profiles); count > 0 {
		b.cursorDir = (b.cursorDir + delta + count) % count
	}
	b.clamp()
	b.View()
}

// enterDir opens the node list of the profile under the cursor and
// restores its remembered position.
func (b *Browser) enterDir() {
	p, ok := b.currentProfile()
	if !ok {
		return
	}
	b.history[""] = browsePos{cursor: b.cursorDir, offset: b.offsetDir}
	b.inside = true
	b.loadNodes(p.Name)
	saved := b.history[p.Name]
	b.cursorItem = saved.cursor
	b.offsetItem = saved.offset
	b.clamp()
	b.View()
}

// exitDir returns to the profile list and restores its remembered
// position.
func (b *Browser) exitDir() {
	if p, ok := b.currentProfile(); ok {
		b.history[p.Name] = browsePos{cursor: b.cursorItem, offset: b.offsetItem}
	}
	b.inside = false
	saved := b.history[""]
	b.cursorDir = saved.cursor
	b.offsetDir = saved.offset
	b.clamp()
	b.View()
}

// selectNode switches the profile and every group carrying the node
// under the cursor.
func (b *Browser) selectNode() {
	p, ok := b.currentProfile()
	if !ok {
		return
	}
	nodes := b.sortedNodes(p.Name)
	if b.cursorItem < 0 || b.cursorItem >= len(nodes) {
		return
	}
	node := nodes[b.cursorItem]
	if err := command(ipc.CmdSelect, map[string]string{"name": p.Name, "node": node}); err != nil {
		b.setMessage(err.Error(), true)
		b.View()
		return
	}
	delete(b.nodes, p.Name)
	b.loadNodes(p.Name)
	b.setMessage("", false)
	b.View()
}

// syncProfile re-downloads the profile under the cursor and reports the
// traffic accounting the subscription advertises.
func (b *Browser) syncProfile() {
	p, ok := b.currentProfile()
	if !ok {
		return
	}
	err := commandReport(ipc.CmdSync, map[string]string{"name": p.Name}, func(text string) {
		b.setMessage(text, false)
		b.drawFooterOnly()
	})
	if err != nil {
		b.setMessage(err.Error(), true)
	} else {
		delete(b.nodes, p.Name)
		b.Init()
		message := "已同步 " + p.Name
		if line := b.trafficLine(p.Name); line != "" {
			message = line
		}
		b.setMessage(message, false)
		if b.inside {
			b.loadNodes(p.Name)
		}
	}
	b.View()
}

// trafficLine formats the stored traffic accounting of one profile.
func (b *Browser) trafficLine(name string) string {
	for _, p := range b.profiles {
		if p.Name == name && p.Total > 0 {
			return "流量 " + fmtBytes(uint64(p.Upload+p.Download)) + " / " + fmtBytes(uint64(p.Total))
		}
	}
	return ""
}

// sortedNodes returns the cached node list of one profile in the current
// sort order.
func (b *Browser) sortedNodes(name string) []string {
	return sortNodes(b.nodes[name].Nodes, b.sortMode)
}

// loadNodes fills the node cache of one profile.
func (b *Browser) loadNodes(name string) {
	if _, ok := b.nodes[name]; ok {
		return
	}
	var info ipc.NodesInfo
	resp, err := ipc.Command(ipc.Request{Command: ipc.CmdNodes, Params: map[string]string{"name": name}})
	if err != nil {
		b.setMessage(err.Error(), true)
		return
	}
	if !resp.OK {
		b.setMessage(resp.Error, true)
		return
	}
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		b.setMessage(err.Error(), true)
		return
	}
	b.nodes[name] = info
	b.clamp()
}

func (b *Browser) currentProfile() (ipc.ProfileInfo, bool) {
	if b.cursorDir < 0 || b.cursorDir >= len(b.profiles) {
		return ipc.ProfileInfo{}, false
	}
	return b.profiles[b.cursorDir], true
}

// setMessage shows a note in the footer. Notes fade back to the hint
// line after three seconds while errors stay until the next action.
func (b *Browser) setMessage(text string, isErr bool) {
	b.message = text
	b.messageErr = isErr
	if isErr {
		b.messageUntil = time.Time{}
		return
	}
	b.messageUntil = time.Now().Add(messageTTL)
}

// expireMessage fades a timed note back to the hint line and reports
// whether the footer needs a repaint.
func (b *Browser) expireMessage() {
	if b.message == "" || b.messageErr {
		return
	}
	if time.Now().Before(b.messageUntil) {
		return
	}
	b.message = ""
	b.drawFooterOnly()
}

func (b *Browser) clamp() {
	if b.inside {
		count := 0
		if p, ok := b.currentProfile(); ok {
			count = len(b.nodes[p.Name].Nodes)
		}
		b.cursorItem = clampPos(b.cursorItem, count)
		b.offsetItem = min(b.offsetItem, b.cursorItem)
		return
	}
	b.cursorDir = clampPos(b.cursorDir, len(b.profiles))
	b.offsetDir = min(b.offsetDir, b.cursorDir)
}

// clampPos keeps a cursor inside a list of count entries.
func clampPos(cursor, count int) int {
	if count <= 0 {
		return 0
	}
	return min(max(cursor, 0), count-1)
}

// scrollToCursor moves the offset the least amount that keeps the cursor
// row visible, the same rule BM applies when scrolling.
func (b *Browser) scrollToCursor(listHeight int) {
	if b.inside {
		b.offsetItem = scrollOffset(b.offsetItem, b.cursorItem, listHeight)
		return
	}
	b.offsetDir = scrollOffset(b.offsetDir, b.cursorDir, listHeight)
}

// scrollOffset returns the offset that keeps the cursor visible with the
// smallest move.
func scrollOffset(offset, cursor, listHeight int) int {
	if listHeight <= 0 {
		return 0
	}
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+listHeight {
		return cursor - listHeight + 1
	}
	return offset
}

// hintLine returns the operation hints shown on the bottom line.
func (b *Browser) hintLine() string {
	return "hjkl 移动  space 切换  n 新增  x 删除  r 同步  ? 帮助"
}

// footerText returns the plain bottom line of the page.
func (b *Browser) footerText() string {
	if b.message != "" {
		return b.message
	}
	return b.hintLine()
}

// drawFooterOnly repaints the footer line alone.
func (b *Browser) drawFooterOnly() {
	b.app.terminal().beginFrame()
	defer b.app.terminal().endFrame()
	w, h := b.app.terminal().Size()
	print(b.footer(w, h))
}

// View draws the Library page.
func (b *Browser) View() {
	term := b.app.terminal()
	term.beginFrame()
	defer term.endFrame()
	w, h := term.Size()

	var buf strings.Builder
	buf.WriteString("\x1b[2J\x1b[3J\x1b[H")

	title := "Profiles"
	titleX := max((w-displayWidth(title))/2, 1)
	fmt.Fprintf(&buf, "\x1b[1;1H\x1b[K\x1b[1;%dH\x1b[1m%s\x1b[0m", titleX, title)
	buf.WriteString("\x1b[2;1H\x1b[K")

	listHeight := h - 4
	if w < 20 || h < 8 {
		buf.WriteString("\x1b[2;1H\x1b[K")
		msg := "终端过小，无法浏览"
		msg = truncateToWidthFromStart(msg, max(w-1, 1))
		x := max((w-displayWidth(msg))/2, 0) + 1
		fmt.Fprintf(&buf, "\x1b[%d;%dH\x1b[90m%s\x1b[0m", max(h/2, 1), x, msg)
		buf.WriteString(b.footer(w, h))
		print(buf.String())
		return
	}

	b.scrollToCursor(listHeight)
	geoms := b.planColumns(w)
	for row := range listHeight {
		y := row + 3
		fmt.Fprintf(&buf, "\x1b[%d;1H\x1b[K", y)
		for _, geom := range geoms {
			buf.WriteString(b.drawColumnRow(geom, row, geom.dim))
		}
	}

	fmt.Fprintf(&buf, "\x1b[%d;1H\x1b[K", h-1)
	buf.WriteString(b.footer(w, h))
	buf.WriteString(b.scrollbar(listHeight))
	print(buf.String())
}

// footer renders the bottom line: the input editor with its movable
// cursor, a message or the operation hints.
func (b *Browser) footer(w, h int) string {
	out := "\x1b[" + itoa(h) + ";1H\x1b[K"
	if b.input.active {
		text := "add: " + string(b.input.value)
		x := max((w-displayWidth(text)-1)/2, 1)
		before := displayWidth("add: " + string(b.input.value[:b.input.pos]))
		cursorChar := " "
		if b.input.pos < len(b.input.value) {
			cursorChar = string(b.input.value[b.input.pos])
		}
		out += "\x1b[" + itoa(h) + ";" + itoa(x) + "H\x1b[37m" + text + "\x1b[0m"
		out += "\x1b[" + itoa(h) + ";" + itoa(x+before) + "H\x1b[7m" + cursorChar + "\x1b[0m"
		return out
	}
	line := truncateToWidth(b.footerText(), w)
	style := "\x1b[90m"
	if b.message != "" {
		style = "\x1b[37m"
		if b.messageErr {
			style = "\x1b[31m"
		}
	}
	x := max((w-displayWidth(line))/2, 1)
	out += "\x1b[" + itoa(h) + ";" + itoa(x) + "H" + style + line + "\x1b[0m"
	return out
}

// scrollbar draws the scroll indicator on the right screen edge.
func (b *Browser) scrollbar(listHeight int) string {
	w, _ := b.app.terminal().Size()
	total := len(b.profiles)
	offset := 0
	cursor := b.cursorDir
	if b.inside {
		if p, ok := b.currentProfile(); ok {
			total = len(b.nodes[p.Name].Nodes)
		}
		cursor = b.cursorItem
	}
	if cursor >= listHeight {
		offset = cursor - listHeight + 1
	}
	thumbSize := 0
	thumbStart := 0
	if total > listHeight {
		thumbSize = max(listHeight*listHeight/total, 1)
		scrollRange := total - listHeight
		thumbRange := listHeight - thumbSize
		if scrollRange > 0 {
			thumbStart = offset * thumbRange / scrollRange
		}
	}
	var buf strings.Builder
	for i := range listHeight {
		cell := "│"
		if total <= listHeight {
			cell = " "
		} else if i >= thumbStart && i < thumbStart+thumbSize {
			cell = "┃"
		}
		fmt.Fprintf(&buf, "\x1b[%d;%dH%s", i+3, w, cell)
	}
	return buf.String()
}
