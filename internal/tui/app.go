package tui

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"px/internal/ipc"
)

// Interaction modes of the event loop.
const (
	modeNormal = iota
	modeInput
	modeConfirm
)

// Asynchronous event kinds carried over the event channel.
const (
	evStatus   = "status"
	evDelay    = "delay"
	evOp       = "op"
	evProgress = "progress"
)

// event carries one asynchronous IPC result into the event loop.
type event struct {
	kind     string
	name     string
	progress string
	err      error
	status   ipc.Status
	delays   map[string]uint16
}

// App holds the TUI state and drives the event loop.
type App struct {
	term   *Terminal
	keys   <-chan Key
	events chan event
	winch  <-chan os.Signal

	status      ipc.Status
	delays      map[string]uint16
	focusLeft   bool
	cursorLeft  int
	cursorRight int
	offLeft     int
	offRight    int
	openGroup   string
	mode        int
	input       Input
	confirm     string
	message     string
	messageErr  bool
	busy        string
	helpFull    bool
	quit        bool
	width       int
	height      int
}

// Run fetches the first snapshot, then executes the event loop until the
// user quits.
func Run() error {
	status, err := fetchStatus()
	if err != nil {
		return err
	}
	term, err := NewTerminal()
	if err != nil {
		return err
	}
	defer term.Close()
	a := &App{
		term:   term,
		keys:   ReadKeys(),
		events: make(chan event, 16),
		winch:  term.Winch(),
		status: status,
	}
	a.width, a.height = term.Size()
	a.loop()
	return nil
}

func fetchStatus() (ipc.Status, error) {
	var status ipc.Status
	resp, err := ipc.Command(ipc.Request{Command: ipc.CmdStatus})
	if err != nil {
		return status, err
	}
	if !resp.OK {
		return status, errors.New(resp.Error)
	}
	err = json.Unmarshal(resp.Data, &status)
	return status, err
}

func (a *App) loop() {
	for !a.quit {
		a.draw()
		select {
		case k, ok := <-a.keys:
			if !ok {
				a.quit = true
			} else {
				a.onKey(k)
			}
		case e := <-a.events:
			a.onEvent(e)
		case <-a.winch:
			a.width, a.height = a.term.Size()
		}
	}
}

// request sends one command to the daemon without blocking the loop.
func (a *App) request(kind, name, cmd string, params map[string]string) {
	go func() {
		resp, err := ipc.CommandReport(ipc.Request{Command: cmd, Params: params}, func(text string) {
			a.events <- event{kind: evProgress, progress: text}
		})
		ev := event{kind: kind, name: name, err: err}
		if ev.err == nil && !resp.OK {
			ev.err = errors.New(resp.Error)
		}
		if ev.err == nil {
			switch kind {
			case evStatus:
				var status ipc.Status
				if err := json.Unmarshal(resp.Data, &status); err != nil {
					ev.err = err
				} else {
					ev.status = status
				}
			case evDelay:
				var delays map[string]uint16
				if err := json.Unmarshal(resp.Data, &delays); err != nil {
					ev.err = err
				} else {
					ev.delays = delays
				}
			}
		}
		a.events <- ev
	}()
}

// refresh asks the daemon for a fresh snapshot.
func (a *App) refresh() {
	a.request(evStatus, "", ipc.CmdStatus, nil)
}

func (a *App) onEvent(e event) {
	switch e.kind {
	case evProgress:
		a.busy = e.progress
	case evStatus:
		if e.err != nil {
			a.setMessage(e.err.Error(), true)
			return
		}
		a.status = e.status
		a.clamp()
	case evDelay:
		a.busy = ""
		if e.err != nil {
			a.setMessage(e.err.Error(), true)
			return
		}
		a.delays = e.delays
		a.setMessage("测速完成", false)
	default:
		a.busy = ""
		if e.err != nil {
			a.setMessage(e.err.Error(), true)
			return
		}
		a.setMessage(e.name+"完成", false)
		a.refresh()
	}
}

func (a *App) setMessage(text string, isErr bool) {
	a.message = text
	a.messageErr = isErr
}

func (a *App) onKey(k Key) {
	switch a.mode {
	case modeInput:
		a.keyInput(k)
	case modeConfirm:
		a.keyConfirm(k)
	default:
		a.keyNormal(k)
	}
}

func (a *App) keyInput(k Key) {
	done, cancelled := a.input.Key(k)
	if !done {
		return
	}
	a.mode = modeNormal
	value := strings.TrimSpace(a.input.Value())
	if cancelled || value == "" {
		a.setMessage("已取消", false)
		return
	}
	a.busy = "添加中"
	a.request(evOp, "添加", ipc.CmdAdd, map[string]string{"url": value})
}

func (a *App) keyConfirm(k Key) {
	a.mode = modeNormal
	if k.Rune == 'y' || k.Rune == 'Y' {
		a.busy = "删除中"
		a.request(evOp, "删除", ipc.CmdDelete, map[string]string{"name": a.confirm})
		return
	}
	a.setMessage("已取消", false)
}

func (a *App) keyNormal(k Key) {
	switch {
	case k.Rune == 'q':
		a.quit = true
	case k.Name == KeyEsc:
		a.openGroup = ""
		a.cursorRight = 0
		a.offRight = 0
		a.delays = nil
	case k.Name == KeyTab:
		a.focusLeft = !a.focusLeft
	case k.Name == KeyUp || k.Rune == 'k':
		a.moveCursor(-1)
	case k.Name == KeyDown || k.Rune == 'j':
		a.moveCursor(1)
	case k.Name == KeyEnter:
		a.activate()
	case k.Rune == 'a':
		a.mode = modeInput
		a.input.Reset()
		a.setMessage("输入订阅链接，Enter 确认，Esc 取消", false)
	case k.Rune == 's':
		a.commandProfile(ipc.CmdSync, "同步", "同步中")
	case k.Rune == 'd':
		if p, ok := a.currentProfile(); ok {
			a.mode = modeConfirm
			a.confirm = p.Name
		}
	case k.Rune == 't':
		a.testDelay()
	case k.Rune == ' ':
		a.toggleKernel()
	case k.Rune == '?':
		a.helpFull = !a.helpFull
	}
}

func (a *App) moveCursor(delta int) {
	if a.focusLeft {
		a.cursorLeft += delta
		a.clamp()
		return
	}
	a.cursorRight += delta
	a.clamp()
}

// activate runs Enter: open a group, pick a node or activate a profile.
func (a *App) activate() {
	if a.focusLeft {
		a.commandProfile(ipc.CmdActivate, "激活", "激活中")
		return
	}
	if a.openGroup == "" {
		if g, ok := a.currentGroupItem(); ok {
			a.openGroup = g.Name
			a.cursorRight = 0
			a.offRight = 0
			a.delays = nil
		}
		return
	}
	node := a.currentNode()
	if node == "" {
		return
	}
	a.busy = "选中中"
	a.request(evOp, "选中", ipc.CmdSelect, map[string]string{"group": a.openGroup, "node": node})
}

func (a *App) testDelay() {
	if a.openGroup == "" {
		a.setMessage("先按 Enter 打开分组再测速", false)
		return
	}
	a.busy = "测速中"
	a.request(evDelay, "", ipc.CmdDelay, map[string]string{"group": a.openGroup})
}

func (a *App) toggleKernel() {
	if a.status.Running {
		a.busy = "停止中"
		a.request(evOp, "停止", ipc.CmdStop, nil)
		return
	}
	a.busy = "启动中"
	a.request(evOp, "启动", ipc.CmdStart, nil)
}

func (a *App) commandProfile(cmd, name, busy string) {
	p, ok := a.currentProfile()
	if !ok {
		return
	}
	a.busy = busy
	a.request(evOp, name, cmd, map[string]string{"name": p.Name})
}

func (a *App) currentProfile() (ipc.ProfileInfo, bool) {
	if a.cursorLeft < 0 || a.cursorLeft >= len(a.status.Profiles) {
		return ipc.ProfileInfo{}, false
	}
	return a.status.Profiles[a.cursorLeft], true
}

func (a *App) currentGroupItem() (ipc.GroupInfo, bool) {
	if a.cursorRight < 0 || a.cursorRight >= len(a.status.Groups) {
		return ipc.GroupInfo{}, false
	}
	return a.status.Groups[a.cursorRight], true
}

func (a *App) currentNode() string {
	for _, g := range a.status.Groups {
		if g.Name != a.openGroup {
			continue
		}
		if a.cursorRight < 0 || a.cursorRight >= len(g.Nodes) {
			return ""
		}
		return g.Nodes[a.cursorRight]
	}
	return ""
}

func (a *App) clamp() {
	if len(a.status.Profiles) == 0 {
		a.cursorLeft = 0
	} else if a.cursorLeft >= len(a.status.Profiles) {
		a.cursorLeft = len(a.status.Profiles) - 1
	}
	if a.cursorLeft < 0 {
		a.cursorLeft = 0
	}
	count := len(a.status.Groups)
	if a.openGroup != "" {
		count = 0
		for _, g := range a.status.Groups {
			if g.Name == a.openGroup {
				count = len(g.Nodes)
			}
		}
	}
	if a.cursorRight >= count {
		a.cursorRight = count - 1
	}
	if a.cursorRight < 0 {
		a.cursorRight = 0
	}
}
