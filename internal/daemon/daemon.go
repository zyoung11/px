package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"px/internal/core"
	"px/internal/geo"
	"px/internal/ipc"
	"px/internal/logx"
	"px/internal/progress"
	"px/internal/state"
)

const (
	profileUA     = "clash.meta"
	profileLimit  = 8 << 20
	downloadLimit = 30 * time.Second
	stopGrace     = 10 * time.Second
)

// Daemon owns the state file and the kernel. The TUI reaches it over the
// control socket only.
type Daemon struct {
	ctx        context.Context
	configPath string
	geoDir     string
	mu         sync.Mutex
	st         *state.State
	running    bool
}

// Run loads the state, resumes the kernel and serves the control socket
// until SIGINT or SIGTERM arrives. Shutdown aborts running downloads and
// never waits for a stuck handler beyond stopGrace.
func Run(configPath string) error {
	if conn, err := ipc.Dial(ipc.SocketPath); err == nil {
		conn.Close()
		return fmt.Errorf("daemon already running at %s", ipc.SocketPath)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d, err := open(ctx, configPath)
	if err != nil {
		cancel()
		return err
	}
	ln, err := ipc.Listen(ipc.SocketPath)
	if err != nil {
		cancel()
		return err
	}
	go ipc.Serve(ln, d.handle)
	logx.Info("daemon ready, socket %s", ipc.SocketPath)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	cancel()
	ln.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.mu.Lock()
		core.Stop()
		d.running = false
		d.mu.Unlock()
		os.Remove(ipc.SocketPath)
		logx.Info("daemon stopped")
	}()
	select {
	case <-done:
	case <-time.After(stopGrace):
		os.Remove(ipc.SocketPath)
		logx.Warn("shutdown timed out, exiting")
	}
	return nil
}

func open(ctx context.Context, configPath string) (*Daemon, error) {
	d := &Daemon{ctx: ctx, configPath: configPath, geoDir: filepath.Join(state.Dir(configPath), "mihomo")}
	if err := core.Init(d.geoDir); err != nil {
		return nil, err
	}
	st, err := state.Load(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		st = &state.State{}
		if err := state.Save(configPath, st); err != nil {
			return nil, err
		}
		logx.Info("created %s", configPath)
	}
	d.st = st
	if st.Active != "" {
		if err := d.applyProfile(st.Active, nil); err != nil {
			logx.Warn("resume %s failed: %v", st.Active, err)
		}
	}
	return d, nil
}

func (d *Daemon) handle(req ipc.Request, report func(string)) ipc.Response {
	d.mu.Lock()
	defer d.mu.Unlock()
	var err error
	switch req.Command {
	case ipc.CmdStatus:
		return okResponse(d.status())
	case ipc.CmdAdd:
		err = d.add(req.Params["url"], report)
	case ipc.CmdSync:
		err = d.sync(req.Params["name"], report)
	case ipc.CmdDelete:
		err = d.delete(req.Params["name"])
	case ipc.CmdActivate:
		err = d.activate(req.Params["name"], report)
	case ipc.CmdSelect:
		err = d.selectNode(req.Params["group"], req.Params["node"])
	case ipc.CmdDelay:
		return d.delay(req.Params["group"])
	case ipc.CmdStart:
		err = d.start()
	case ipc.CmdStop:
		err = d.stop()
	default:
		err = fmt.Errorf("unknown command %s", req.Command)
	}
	if err != nil {
		return ipc.Response{OK: false, Error: err.Error()}
	}
	return ipc.Response{OK: true}
}

// portFree reports whether no service listens on a localhost TCP port.
// The kernel swallows bind errors, so the daemon checks the port before
// it asks the kernel to bind.
func portFree(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err != nil {
		return true
	}
	conn.Close()
	return false
}

func (d *Daemon) add(rawURL string, report func(string)) error {
	if rawURL == "" {
		return fmt.Errorf("url is empty")
	}
	if _, err := url.Parse(rawURL); err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if i := d.st.FindURL(rawURL); i >= 0 {
		return d.sync(d.st.Profiles[i].Name, report)
	}
	data, err := download(d.ctx, rawURL, report)
	if err != nil {
		return err
	}
	if err := geo.Ensure(d.ctx, d.geoDir, data, report); err != nil {
		return err
	}
	if err := core.Validate(data); err != nil {
		return fmt.Errorf("profile rejected by the kernel: %w", err)
	}
	name := deriveName(rawURL)
	for i := 2; d.st.Find(name) >= 0; i++ {
		name = fmt.Sprintf("%s-%d", deriveName(rawURL), i)
	}
	if err := d.writeProfile(name, data); err != nil {
		return err
	}
	d.st.Profiles = append(d.st.Profiles, state.Profile{
		Name:      name,
		URL:       rawURL,
		UpdatedAt: nowStamp(),
	})
	logx.Info("added %s", name)
	return state.Save(d.configPath, d.st)
}

func (d *Daemon) sync(name string, report func(string)) error {
	i := d.st.Find(name)
	if i < 0 {
		return fmt.Errorf("profile %s not found", name)
	}
	data, err := download(d.ctx, d.st.Profiles[i].URL, report)
	if err != nil {
		return err
	}
	if err := geo.Ensure(d.ctx, d.geoDir, data, report); err != nil {
		return err
	}
	if err := core.Validate(data); err != nil {
		return fmt.Errorf("profile rejected by the kernel: %w", err)
	}
	if err := d.writeProfile(name, data); err != nil {
		return err
	}
	d.st.Profiles[i].UpdatedAt = nowStamp()
	if d.st.Active == name {
		if err := d.applyProfile(name, report); err != nil {
			return err
		}
	}
	logx.Info("synced %s", name)
	return state.Save(d.configPath, d.st)
}

func (d *Daemon) delete(name string) error {
	i := d.st.Find(name)
	if i < 0 {
		return fmt.Errorf("profile %s not found", name)
	}
	if d.st.Active == name {
		core.Stop()
		d.running = false
		d.st.Active = ""
		d.st.Enabled = false
	}
	os.Remove(state.ProfilePath(d.configPath, name))
	d.st.Profiles = append(d.st.Profiles[:i], d.st.Profiles[i+1:]...)
	logx.Info("deleted %s", name)
	return state.Save(d.configPath, d.st)
}

func (d *Daemon) activate(name string, report func(string)) error {
	if d.st.Find(name) < 0 {
		return fmt.Errorf("profile %s not found", name)
	}
	if err := d.applyProfile(name, report); err != nil {
		return err
	}
	d.st.Active = name
	logx.Info("activated %s", name)
	return state.Save(d.configPath, d.st)
}

func (d *Daemon) selectNode(group, node string) error {
	if group == "" || node == "" {
		return fmt.Errorf("group and node are required")
	}
	if err := core.Select(group, node); err != nil {
		return err
	}
	i := d.st.Find(d.st.Active)
	if i >= 0 {
		if d.st.Profiles[i].Selected == nil {
			d.st.Profiles[i].Selected = map[string]string{}
		}
		d.st.Profiles[i].Selected[group] = node
	}
	return state.Save(d.configPath, d.st)
}

func (d *Daemon) delay(group string) ipc.Response {
	if group == "" {
		return ipc.Response{OK: false, Error: "group is required"}
	}
	result, err := core.Delay(group, d.st.DelayURL, core.DelayTimeout)
	if err != nil {
		return ipc.Response{OK: false, Error: err.Error()}
	}
	return okResponse(result)
}

func (d *Daemon) start() error {
	if d.st.Active == "" {
		return fmt.Errorf("no active profile")
	}
	if !d.running {
		if !portFree(d.st.Port) {
			return fmt.Errorf("port %d is already in use", d.st.Port)
		}
		if err := d.applyProfile(d.st.Active, nil); err != nil {
			return err
		}
	}
	core.Start(d.st.Port)
	d.warnTun(true)
	d.running = true
	d.st.Enabled = true
	logx.Info("kernel listening on %d", d.st.Port)
	return state.Save(d.configPath, d.st)
}

func (d *Daemon) stop() error {
	core.Stop()
	d.running = false
	d.st.Enabled = false
	logx.Info("kernel stopped")
	return state.Save(d.configPath, d.st)
}

func (d *Daemon) applyProfile(name string, report func(string)) error {
	data, err := os.ReadFile(state.ProfilePath(d.configPath, name))
	if err != nil {
		return fmt.Errorf("read profile %s: %w", name, err)
	}
	if err := geo.Ensure(d.ctx, d.geoDir, data, report); err != nil {
		return err
	}
	port := 0
	if d.st.Enabled {
		port = d.st.Port
	}
	if port > 0 && !d.running && !portFree(port) {
		return fmt.Errorf("port %d is already in use", port)
	}
	if err := core.Apply(data, port); err != nil {
		return err
	}
	d.warnTun(port > 0)
	d.running = d.st.Enabled && port > 0
	i := d.st.Find(name)
	if i >= 0 {
		for group, node := range d.st.Profiles[i].Selected {
			if err := core.Select(group, node); err != nil {
				logx.Warn("restore %s/%s failed: %v", group, node, err)
			}
		}
	}
	return nil
}

func (d *Daemon) status() ipc.Status {
	st := ipc.Status{
		Running:  d.running,
		Tun:      d.running && core.TunUp(),
		Port:     d.st.Port,
		Enabled:  d.st.Enabled,
		Active:   d.st.Active,
		DelayURL: d.st.DelayURL,
		Profiles: []ipc.ProfileInfo{},
		Groups:   []ipc.GroupInfo{},
	}
	for _, p := range d.st.Profiles {
		st.Profiles = append(st.Profiles, ipc.ProfileInfo{
			Name:      p.Name,
			URL:       p.URL,
			UpdatedAt: p.UpdatedAt,
		})
	}
	if d.st.Active != "" {
		for _, g := range core.Groups() {
			st.Groups = append(st.Groups, ipc.GroupInfo(g))
		}
	}
	return st
}

// warnTun logs a notice when TUN is requested but the device is not
// there, which happens without root privileges.
func (d *Daemon) warnTun(tun bool) {
	if tun && !core.TunUp() {
		logx.Warn("TUN device %s is missing, the kernel needs root for TUN", "px0")
	}
}

func (d *Daemon) writeProfile(name string, data []byte) error {
	dir := state.ProfilesDir(d.configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".yaml"), data, 0o644)
}

// download fetches a subscription body and reports transfer shares.
func download(ctx context.Context, rawURL string, report func(string)) ([]byte, error) {
	client := &http.Client{Timeout: downloadLimit}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", profileUA)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := progress.Copy(&buf, io.LimitReader(resp.Body, profileLimit), resp.ContentLength, "下载订阅", report); err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("download %s: empty body", rawURL)
	}
	return buf.Bytes(), nil
}

func deriveName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "profile"
	}
	base := strings.TrimSuffix(pathSegment(u.Path), ".yaml")
	base = sanitize(base)
	if base == "" {
		base = sanitize(u.Hostname())
	}
	if base == "" {
		return "profile"
	}
	return base
}

func pathSegment(p string) string {
	p = strings.Trim(p, "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), ".")
}

func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func okResponse(data any) ipc.Response {
	raw, err := json.Marshal(data)
	if err != nil {
		return ipc.Response{OK: false, Error: err.Error()}
	}
	return ipc.Response{OK: true, Data: raw}
}
