package tui

import (
	"encoding/json"
	"errors"
	"time"

	"px/internal/ipc"
	"px/internal/traffic"
)

// Page is one full screen page of the TUI.
type Page interface {
	// Init reloads the page state from the daemon.
	Init()
	// Modal reports whether the page consumes every key itself.
	Modal() bool
	// HandleKey processes one key.
	HandleKey(key rune)
	// View draws the page.
	View()
}

// event carries one asynchronous IPC result into the event loop.
type event struct {
	progress string
	err      error
	done     func(error)
}

// spinFrames are the spinner glyphs cycled while loading.
const spinFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

// App holds the shared TUI state and drives the event loop.
type App struct {
	term        *Terminal
	events      chan event
	busy        bool
	busyText    string
	spin        int
	pages       []Page
	current     int
	helpOpen    bool
	confirmQuit bool
	confirmMsg  string
	confirmYes  string
	confirmNo   string
	quit        bool
	tick        int
	message     string
	delays      map[string]map[string]uint16
}

// showConfirm draws the modal confirmation box and remembers its content
// so a resize can redraw it.
func (a *App) showConfirm(msg, suffixConfirm, suffixCancel string) {
	a.confirmMsg = msg
	a.confirmYes = suffixConfirm
	a.confirmNo = suffixCancel
	a.drawConfirmBox(msg, suffixConfirm, suffixCancel)
}

// terminal returns the terminal the app draws on.
func (a *App) terminal() *Terminal {
	return a.term
}

// setMessage shows a transient note on the active page.
func (a *App) setMessage(text string) {
	a.message = text
}

// run starts one asynchronous command with loading feedback and calls
// done on the event loop when it finishes.
func (a *App) run(cmd string, params map[string]string, done func(error)) {
	a.busy = true
	a.busyText = t(txtLoading)
	go func() {
		resp, err := ipc.CommandReport(ipc.Request{Command: cmd, Params: params}, func(text string) {
			a.events <- event{progress: text}
		})
		if err == nil && !resp.OK {
			err = errors.New(resp.Error)
		}
		a.events <- event{err: err, done: done}
	}()
}

// onEvent finishes one asynchronous command on the event loop.
func (a *App) onEvent(e event) {
	if e.done == nil {
		a.busyText = e.progress
		return
	}
	a.busy = false
	a.busyText = ""
	e.done(e.err)
}

// loadingLine returns the bottom line shown while loading.
func (a *App) loadingLine() string {
	frames := []rune(spinFrames)
	text := a.busyText
	if text == "" {
		text = t(txtLoading)
	}
	return string(frames[a.spin%len(frames)]) + " " + text
}

// redrawBottom repaints only the bottom line of the active page.
func (a *App) redrawBottom() {
	if a.helpOpen || a.confirmQuit {
		return
	}
	switch page := a.pages[a.current].(type) {
	case *Browser:
		page.drawFooterOnly()
	case *Home:
		page.drawFooterOnly()
	}
}

// Run connects to the daemon and executes the event loop until the user
// quits.
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

	setLang(status.Lang)
	setKeymap(status.Keys)
	app := &App{term: term, events: make(chan event, 16), delays: map[string]map[string]uint16{}}
	home := NewHome(app, status)
	browser := NewBrowser(app)
	app.pages = []Page{home, browser}
	if status.Page == 1 {
		app.current = 1
	}

	keys := readKeys()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	winch := term.Winch()

	app.pages[app.current].Init()
	app.pages[app.current].View()
	for !app.quit {
		select {
		case key, ok := <-keys:
			if !ok {
				app.quit = true
			} else {
				app.onKey(key)
			}
		case e := <-app.events:
			app.onEvent(e)
		case <-ticker.C:
			browser.expireMessage()
			if app.busy {
				app.spin++
				app.redrawBottom()
			}
			app.tick++
			if app.tick%10 == 0 {
				if _, isHome := app.pages[app.current].(*Home); isHome && !app.helpOpen && !app.confirmQuit {
					home.Refresh()
					home.View()
				}
			}
		case <-winch:
			switch {
			case app.helpOpen:
				app.drawHelpPage()
			case app.confirmQuit:
				app.drawConfirmBox(app.confirmMsg, app.confirmYes, app.confirmNo)
			default:
				app.pages[app.current].View()
			}
		}
	}
	app.saveUI()
	return nil
}

// onKey dispatches one key to the overlay, the page or the global map.
func (a *App) onKey(key rune) {
	if key == modCtrl|'c' {
		a.quit = true
		return
	}
	if a.helpOpen {
		a.handleHelpKey(key)
		return
	}
	if a.confirmQuit {
		if key == KeyEnter {
			a.quit = true
			return
		}
		if key == 0x1b || IsKey(key, keymap.Global.Quit) {
			a.confirmQuit = false
			a.pages[a.current].View()
		}
		return
	}
	if a.pages[a.current].Modal() {
		a.pages[a.current].HandleKey(key)
		return
	}
	switch {
	case IsKey(key, keymap.Global.Quit):
		a.confirmQuit = true
		a.showConfirm(t(txtQuitAsk), t(txtBtnQuit), t(txtBtnCancel))
	case IsKey(key, keymap.Global.ShowHelp):
		a.helpOpen = true
		a.drawHelpPage()
	case IsKey(key, keymap.Global.CyclePages):
		a.switchToPage((a.current + 1) % len(a.pages))
	case IsKey(key, keymap.Global.SwitchToHome):
		a.switchToPage(0)
	case IsKey(key, keymap.Global.SwitchToLibrary):
		a.switchToPage(1)
	default:
		a.pages[a.current].HandleKey(key)
	}
}

// handleHelpKey closes the help page or jumps to the page a number key
// names.
func (a *App) handleHelpKey(key rune) {
	if IsKey(key, keymap.Global.SwitchToHome) {
		a.helpOpen = false
		a.switchToPage(0)
		return
	}
	if IsKey(key, keymap.Global.SwitchToLibrary) {
		a.helpOpen = false
		a.switchToPage(1)
		return
	}
	if key == 0x1b || IsKey(key, keymap.Global.Quit) || IsKey(key, keymap.Global.ShowHelp) {
		a.helpOpen = false
		a.pages[a.current].View()
	}
}

// switchToPage activates the page with the given index and remembers the
// choice.
func (a *App) switchToPage(index int) {
	if index < 0 || index >= len(a.pages) || index == a.current {
		return
	}
	a.current = index
	a.saveUI()
	a.pages[a.current].Init()
	a.pages[a.current].View()
}

// saveUI stores the sort mode, the align mode, the browse position and
// the active page in the state file.
func (a *App) saveUI() {
	sortMode := 0
	align := 0
	browse := ""
	if browser, ok := a.pages[1].(*Browser); ok {
		sortMode = browser.sortMode
		if browser.align {
			align = 1
		}
		pos := browser.browseState()
		pos.Width, pos.Height = a.term.Size()
		if data, err := json.Marshal(pos); err == nil {
			browse = string(data)
		}
	}
	_ = command(ipc.CmdUI, map[string]string{
		"sort":   itoa(sortMode),
		"page":   itoa(a.current),
		"align":  itoa(align),
		"browse": browse,
	})
}

// delayOf returns the latency of one node in milliseconds, or -1 when the
// node has no measurement yet.
func (a *App) delayOf(profile, node string) int {
	ms, ok := a.delays[profile][node]
	if !ok {
		return -1
	}
	return int(ms)
}

// mergeDelays stores one batch of latency measurements per profile.
func (a *App) mergeDelays(result map[string]map[string]uint16) {
	for profile, nodes := range result {
		if a.delays[profile] == nil {
			a.delays[profile] = map[string]uint16{}
		}
		for node, ms := range nodes {
			a.delays[profile][node] = ms
		}
	}
}

// runDelay tests the requested nodes in the background and stores the result.
func (a *App) runDelay(targets []ipc.DelayTarget, done func(error)) {
	a.busy = true
	a.busyText = t(txtDelayBusy)
	go func() {
		result, err := fetchDelays(targets)
		a.events <- event{err: err, done: func(err error) {
			if err == nil {
				a.mergeDelays(result)
			}
			if done != nil {
				done(err)
			}
		}}
	}()
}

// fetchDelays asks the daemon to test the latency of the requested nodes.
func fetchDelays(targets []ipc.DelayTarget) (map[string]map[string]uint16, error) {
	out := map[string]map[string]uint16{}
	data, err := json.Marshal(targets)
	if err != nil {
		return out, err
	}
	resp, err := ipc.Command(ipc.Request{Command: ipc.CmdDelayNodes, Params: map[string]string{"targets": string(data)}})
	if err != nil {
		return out, err
	}
	if !resp.OK {
		return out, errors.New(resp.Error)
	}
	err = json.Unmarshal(resp.Data, &out)
	return out, err
}

// fetchStatus asks the daemon for one state snapshot.
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

// fetchTraffic asks the daemon for the recorded traffic history.
func fetchTraffic() (traffic.Snapshot, error) {
	var snap traffic.Snapshot
	resp, err := ipc.Command(ipc.Request{Command: ipc.CmdTraffic})
	if err != nil {
		return snap, err
	}
	if !resp.OK {
		return snap, errors.New(resp.Error)
	}
	err = json.Unmarshal(resp.Data, &snap)
	return snap, err
}

// fetchNodes asks the daemon for the node list of one profile.
func fetchNodes(name string) (ipc.NodesInfo, error) {
	var info ipc.NodesInfo
	resp, err := ipc.Command(ipc.Request{Command: ipc.CmdNodes, Params: map[string]string{"name": name}})
	if err != nil {
		return info, err
	}
	if !resp.OK {
		return info, errors.New(resp.Error)
	}
	err = json.Unmarshal(resp.Data, &info)
	return info, err
}

// command sends one mutating command and returns its error.
func command(cmd string, params map[string]string) error {
	resp, err := ipc.Command(ipc.Request{Command: cmd, Params: params})
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	return nil
}
