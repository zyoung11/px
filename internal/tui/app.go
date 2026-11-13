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

// App holds the shared TUI state and drives the event loop.
type App struct {
	term        *Terminal
	pages       []Page
	current     int
	helpOpen    bool
	confirmQuit bool
	confirmMsg  string
	confirmYes  string
	confirmNo   string
	quit        bool
	message     string
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

	app := &App{term: term}
	home := NewHome(app, status)
	browser := NewBrowser(app)
	app.pages = []Page{home, browser}
	if status.Page == 1 {
		app.current = 1
	}

	keys := readKeys()
	ticker := time.NewTicker(time.Second)
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
		case <-ticker.C:
			browser.expireMessage()
			if _, isHome := app.pages[app.current].(*Home); isHome && !app.helpOpen && !app.confirmQuit {
				home.Refresh()
				home.View()
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
	return nil
}

// onKey dispatches one key to the overlay, the page or the global map.
func (a *App) onKey(key rune) {
	if key == 3 {
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
		a.showConfirm("确定要退出吗？", " 退出", " 取消")
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

// saveUI stores the sort mode and the active page in the state file.
func (a *App) saveUI() {
	sortMode := 0
	if browser, ok := a.pages[1].(*Browser); ok {
		sortMode = browser.sortMode
	}
	_ = command(ipc.CmdUI, map[string]string{
		"sort": itoa(sortMode),
		"page": itoa(a.current),
	})
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

// commandReport sends one command and forwards its progress reports.
func commandReport(cmd string, params map[string]string, report func(string)) error {
	resp, err := ipc.CommandReport(ipc.Request{Command: cmd, Params: params}, report)
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	return nil
}
