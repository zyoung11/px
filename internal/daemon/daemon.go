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
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"px/internal/core"
	"px/internal/geo"
	"px/internal/ipc"
	"px/internal/logx"
	"px/internal/prof"
	"px/internal/progress"
	"px/internal/state"
	"px/internal/traffic"
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
	ctx          context.Context
	configPath   string
	geoDir       string
	traffic      *traffic.Sampler
	mu           sync.RWMutex
	kernelMu     sync.Mutex
	st           *state.State
	stMTime      time.Time
	profileMTime map[string]time.Time
	running      bool
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
	go d.watchReload()
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
	d := &Daemon{
		ctx:        ctx,
		configPath: configPath,
		geoDir:     filepath.Join(state.Dir(configPath), "mihomo"),
		traffic:    traffic.New("px0"),
	}
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
	d.profileMTime = map[string]time.Time{}
	d.stampAll()
	if st.Active != "" {
		if err := d.applyProfile(st.Active, nil); err != nil {
			logx.Warn("resume %s failed: %v", st.Active, err)
		}
	}
	return d, nil
}

func (d *Daemon) handle(req ipc.Request, report func(string)) ipc.Response {
	var err error
	switch req.Command {
	case ipc.CmdStatus:
		d.mu.RLock()
		defer d.mu.RUnlock()
		return okResponse(d.status())
	case ipc.CmdTraffic:
		return okResponse(d.traffic.Snapshot())
	case ipc.CmdNodes:
		d.mu.RLock()
		defer d.mu.RUnlock()
		info, nerr := d.nodesInfo(req.Params["name"])
		if nerr != nil {
			return ipc.Response{OK: false, Error: nerr.Error()}
		}
		return okResponse(info)
	case ipc.CmdAdd:
		err = d.add(req.Params["url"], report)
	case ipc.CmdSync:
		err = d.sync(req.Params["name"], report)
	case ipc.CmdDelete:
		err = d.delete(req.Params["name"])
	case ipc.CmdActivate:
		err = d.activate(req.Params["name"], report)
	case ipc.CmdSelect:
		err = d.selectNode(req.Params["name"], req.Params["node"])
	case ipc.CmdDelay:
		return d.delay(req.Params["group"])
	case ipc.CmdDelayNodes:
		return d.delayNodes(req.Params["targets"])
	case ipc.CmdUI:
		err = d.saveUI(req.Params["sort"], req.Params["page"], req.Params["align"], req.Params["browse"])
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
	d.mu.RLock()
	known := ""
	for _, p := range d.st.Profiles {
		if p.URL == rawURL {
			known = p.Name
			break
		}
	}
	d.mu.RUnlock()
	if known != "" {
		return d.sync(known, report)
	}
	data, fileName, traffic, err := download(d.ctx, rawURL, report)
	if err != nil {
		return err
	}
	if err := geo.Ensure(d.ctx, d.geoDir, data, report); err != nil {
		return err
	}
	if err := core.Validate(data); err != nil {
		return fmt.Errorf("profile rejected by the kernel: %w", err)
	}
	name := cleanName(fileName)
	if name == "" {
		name = deriveName(rawURL)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	base := name
	for i := 2; d.st.Find(name) >= 0; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	if err := d.writeProfile(name, data); err != nil {
		return err
	}
	d.st.Profiles = append(d.st.Profiles, state.Profile{
		Name:      name,
		URL:       rawURL,
		UpdatedAt: nowStamp(),
		Traffic:   traffic,
	})
	logx.Info("added %s", name)
	return d.saveState()
}

func (d *Daemon) sync(name string, report func(string)) error {
	d.mu.RLock()
	url := ""
	if i := d.st.Find(name); i >= 0 {
		url = d.st.Profiles[i].URL
	}
	d.mu.RUnlock()
	if url == "" {
		return fmt.Errorf("profile %s not found", name)
	}
	data, _, traffic, err := download(d.ctx, url, report)
	if err != nil {
		return err
	}
	if err := geo.Ensure(d.ctx, d.geoDir, data, report); err != nil {
		return err
	}
	if err := core.Validate(data); err != nil {
		return fmt.Errorf("profile rejected by the kernel: %w", err)
	}
	d.mu.Lock()
	if i := d.st.Find(name); i >= 0 {
		if err := d.writeProfile(name, data); err != nil {
			d.mu.Unlock()
			return err
		}
		d.st.Profiles[i].UpdatedAt = nowStamp()
		if traffic != nil {
			d.st.Profiles[i].Traffic = traffic
		}
	}
	err = d.saveState()
	stillActive := d.st.Active == name
	d.mu.Unlock()
	if err != nil {
		return err
	}
	if stillActive {
		if err := d.applyProfile(name, report); err != nil {
			return err
		}
	}
	logx.Info("synced %s", name)
	return nil
}

func (d *Daemon) delete(name string) error {
	d.mu.Lock()
	i := d.st.Find(name)
	if i < 0 {
		d.mu.Unlock()
		return fmt.Errorf("profile %s not found", name)
	}
	stop := d.st.Active == name
	if stop {
		d.running = false
		d.st.Active = ""
		d.st.Enabled = false
	}
	os.Remove(state.ProfilePath(d.configPath, name))
	d.st.Profiles = append(d.st.Profiles[:i], d.st.Profiles[i+1:]...)
	err := d.saveState()
	d.mu.Unlock()
	if stop {
		d.kernelMu.Lock()
		core.Stop()
		d.kernelMu.Unlock()
	}
	logx.Info("deleted %s", name)
	return err
}

func (d *Daemon) activate(name string, report func(string)) error {
	if err := d.applyProfile(name, report); err != nil {
		return err
	}
	d.mu.Lock()
	d.st.Active = name
	err := d.saveState()
	d.mu.Unlock()
	logx.Info("activated %s", name)
	return err
}

// selectNode switches every selectable group that carries the node and
// records the choice per group.
// saveUI records the TUI preferences in the state file.
func (d *Daemon) saveUI(sortParam, pageParam, alignParam, browseParam string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if v, err := strconv.Atoi(sortParam); err == nil {
		d.st.Sort = v
	}
	if v, err := strconv.Atoi(pageParam); err == nil {
		d.st.Page = v
	}
	if v, err := strconv.Atoi(alignParam); err == nil {
		d.st.Align = v == 1
	}
	if browseParam != "" {
		var browse state.BrowseState
		if err := json.Unmarshal([]byte(browseParam), &browse); err == nil {
			d.st.Browse = browse
		}
	}
	return d.saveState()
}

func (d *Daemon) selectNode(name, node string) error {
	if name == "" || node == "" {
		return fmt.Errorf("profile and node are required")
	}
	d.mu.RLock()
	active := d.st.Active == name
	d.mu.RUnlock()
	if !active {
		if err := d.applyProfile(name, nil); err != nil {
			return err
		}
		d.mu.Lock()
		d.st.Active = name
		d.mu.Unlock()
	}
	d.kernelMu.Lock()
	groups := core.Groups()
	changed := 0
	for _, g := range groups {
		if !containsNode(g.Nodes, node) {
			continue
		}
		if err := core.Select(g.Name, node); err != nil {
			continue
		}
		changed++
	}
	d.kernelMu.Unlock()
	if changed == 0 {
		return fmt.Errorf("node %s not found in profile %s", node, name)
	}
	d.mu.Lock()
	if i := d.st.Find(name); i >= 0 {
		for _, g := range groups {
			if !containsNode(g.Nodes, node) {
				continue
			}
			if d.st.Profiles[i].Selected == nil {
				d.st.Profiles[i].Selected = map[string]string{}
			}
			d.st.Profiles[i].Selected[g.Name] = node
		}
	}
	err := d.saveState()
	d.mu.Unlock()
	logx.Info("selected %s in %s for %d groups", node, name, changed)
	return err
}

// containsNode reports whether the node is one of the names.
func containsNode(names []string, node string) bool {
	for _, name := range names {
		if name == node {
			return true
		}
	}
	return false
}

// nodesInfo resolves the node list of one profile without loading it into
// the kernel.
func (d *Daemon) nodesInfo(name string) (ipc.NodesInfo, error) {
	data, err := os.ReadFile(state.ProfilePath(d.configPath, name))
	if err != nil {
		return ipc.NodesInfo{}, fmt.Errorf("read profile %s: %w", name, err)
	}
	info, err := prof.Parse(data)
	if err != nil {
		return ipc.NodesInfo{}, err
	}
	out := ipc.NodesInfo{Nodes: []ipc.NodeInfo{}}
	for _, node := range info.Nodes {
		out.Nodes = append(out.Nodes, ipc.NodeInfo{Name: node.Name, Type: node.Type})
	}
	i := d.st.Find(name)
	if i >= 0 {
		keys := make([]string, 0, len(d.st.Profiles[i].Selected))
		for key := range d.st.Profiles[i].Selected {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if value := d.st.Profiles[i].Selected[key]; value != "" {
				out.Selected = value
				break
			}
		}
	}
	return out, nil
}

func (d *Daemon) delay(group string) ipc.Response {
	if group == "" {
		return ipc.Response{OK: false, Error: "group is required"}
	}
	d.mu.RLock()
	url := d.st.DelayURL
	d.mu.RUnlock()
	result, err := core.Delay(group, url, core.DelayTimeout)
	if err != nil {
		return ipc.Response{OK: false, Error: err.Error()}
	}
	return okResponse(result)
}

// delayNodes tests the latency of the requested nodes of every profile and
// returns one table per profile.
func (d *Daemon) delayNodes(targetsParam string) ipc.Response {
	var targets []ipc.DelayTarget
	if err := json.Unmarshal([]byte(targetsParam), &targets); err != nil {
		return ipc.Response{OK: false, Error: "targets must be a JSON list"}
	}
	if len(targets) == 0 {
		return ipc.Response{OK: false, Error: "targets is required"}
	}
	d.mu.RLock()
	url := d.st.DelayURL
	d.mu.RUnlock()
	out := map[string]map[string]uint16{}
	for _, target := range targets {
		body, err := os.ReadFile(state.ProfilePath(d.configPath, target.Profile))
		if err != nil {
			out[target.Profile] = map[string]uint16{}
			continue
		}
		out[target.Profile] = core.DelayProfile(body, target.Nodes, url, core.DelayTimeout)
	}
	return okResponse(out)
}

func (d *Daemon) start() error {
	d.mu.RLock()
	active := d.st.Active
	port := d.st.Port
	running := d.running
	d.mu.RUnlock()
	if active == "" {
		return fmt.Errorf("no active profile")
	}
	d.kernelMu.Lock()
	defer d.kernelMu.Unlock()
	if !running {
		if !portFree(port) {
			return fmt.Errorf("port %d is already in use", port)
		}
		if err := d.applyProfileLocked(active, nil); err != nil {
			return err
		}
	}
	core.Start(port)
	d.warnTun(true)
	d.mu.Lock()
	d.running = true
	d.st.Enabled = true
	err := d.saveState()
	d.mu.Unlock()
	logx.Info("kernel listening on %d", port)
	return err
}

func (d *Daemon) stop() error {
	d.kernelMu.Lock()
	core.Stop()
	d.kernelMu.Unlock()
	d.mu.Lock()
	d.running = false
	d.st.Enabled = false
	err := d.saveState()
	d.mu.Unlock()
	logx.Info("kernel stopped")
	return err
}

// applyProfile loads one profile body into the kernel. Downloads run
// outside the locks and only the kernel work is serialised, so slow
// networks never block the readers.
func (d *Daemon) applyProfile(name string, report func(string)) error {
	data, err := os.ReadFile(state.ProfilePath(d.configPath, name))
	if err != nil {
		return fmt.Errorf("read profile %s: %w", name, err)
	}
	if err := geo.Ensure(d.ctx, d.geoDir, data, report); err != nil {
		return err
	}
	d.kernelMu.Lock()
	defer d.kernelMu.Unlock()
	return d.applyProfileLocked(name, data)
}

// applyProfileLocked swaps the kernel configuration while kernelMu is
// held. The state is read and written under the state lock.
func (d *Daemon) applyProfileLocked(name string, data []byte) error {
	d.mu.RLock()
	port := 0
	if d.st.Enabled {
		port = d.st.Port
	}
	enabled := d.st.Enabled
	running := d.running
	selected := map[string]string{}
	if i := d.st.Find(name); i >= 0 {
		for group, node := range d.st.Profiles[i].Selected {
			selected[group] = node
		}
	}
	d.mu.RUnlock()
	if port > 0 && !running && !portFree(port) {
		return fmt.Errorf("port %d is already in use", port)
	}
	if err := core.Apply(data, port); err != nil {
		return err
	}
	d.warnTun(port > 0)
	d.mu.Lock()
	d.running = enabled && port > 0
	d.mu.Unlock()
	for group, node := range selected {
		if err := core.Select(group, node); err != nil {
			logx.Warn("restore %s/%s failed: %v", group, node, err)
		}
	}
	return nil
}

func (d *Daemon) status() ipc.Status {
	st := ipc.Status{
		Running:    d.running,
		Tun:        d.running && core.TunUp(),
		Port:       d.st.Port,
		ChartWidth: d.st.ChartWidth,
		Enabled:    d.st.Enabled,
		Active:     d.st.Active,
		DelayURL:   d.st.DelayURL,
		Lang:       d.st.Lang,
		Sort:       d.st.Sort,
		Align:      d.st.Align,
		Browse:     d.st.Browse,
		Page:       d.st.Page,
		Keys:       d.st.Keys,
		Profiles:   []ipc.ProfileInfo{},
		Groups:     []ipc.GroupInfo{},
	}
	for _, p := range d.st.Profiles {
		info := ipc.ProfileInfo{
			Name:      p.Name,
			URL:       p.URL,
			UpdatedAt: p.UpdatedAt,
		}
		if p.Traffic != nil {
			info.Upload = p.Traffic.Upload
			info.Download = p.Traffic.Download
			info.Total = p.Traffic.Total
		}
		st.Profiles = append(st.Profiles, info)
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
	path := state.ProfilePath(d.configPath, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	d.profileMTime[name] = fileMTime(path)
	return nil
}

// saveState writes the state file and remembers its stamp so px's own
// writes are not treated as outside edits.
func (d *Daemon) saveState() error {
	if err := state.Save(d.configPath, d.st); err != nil {
		return err
	}
	d.stMTime = fileMTime(d.configPath)
	return nil
}

// stampAll records the current file stamps of the state file and every
// profile body.
func (d *Daemon) stampAll() {
	d.stMTime = fileMTime(d.configPath)
	for _, p := range d.st.Profiles {
		path := state.ProfilePath(d.configPath, p.Name)
		d.profileMTime[p.Name] = fileMTime(path)
	}
}

// fileMTime returns the modification time of a file, zero when missing.
func fileMTime(path string) time.Time {
	if st, err := os.Stat(path); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}

// watchReload polls the config file and the profile bodies for edits made
// outside px and reloads them into the kernel.
func (d *Daemon) watchReload() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.reload()
		}
	}
}

// reload refreshes outside edits so the kernel and the state follow
// whatever the user saved on disk. The kernel work runs outside the state
// lock so the readers stay free.
func (d *Daemon) reload() {
	d.mu.Lock()
	needKernel := false
	name := ""
	if t := fileMTime(d.configPath); !t.Equal(d.stMTime) {
		st, err := state.Load(d.configPath)
		if err != nil {
			logx.Warn("reload %s failed: %v", d.configPath, err)
		} else {
			d.st = st
			d.stMTime = t
			logx.Info("state reloaded from %s", d.configPath)
			needKernel = true
			name = d.st.Active
		}
	}
	for _, p := range d.st.Profiles {
		path := state.ProfilePath(d.configPath, p.Name)
		t := fileMTime(path)
		if t.Equal(d.profileMTime[p.Name]) {
			continue
		}
		d.profileMTime[p.Name] = t
		if p.Name == d.st.Active {
			needKernel = true
			name = p.Name
		}
	}
	d.mu.Unlock()
	if !needKernel {
		return
	}
	d.syncKernel(name)
}

// syncKernel re-applies the named profile so the kernel matches the state
// file after an outside edit.
func (d *Daemon) syncKernel(name string) {
	if name == "" {
		d.kernelMu.Lock()
		core.Stop()
		d.kernelMu.Unlock()
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
		return
	}
	if err := d.applyProfile(name, nil); err != nil {
		logx.Warn("reload profile %s failed: %v", name, err)
	} else {
		logx.Info("profile %s reloaded", name)
	}
}

// download fetches a subscription body and reports transfer shares. The
// returned name is the file name the server advertises in
// Content-Disposition and may be empty; the returned traffic is the
// subscription-userinfo accounting and may be nil.
func download(ctx context.Context, rawURL string, report func(string)) ([]byte, string, *state.UserInfo, error) {
	client := &http.Client{Timeout: downloadLimit}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", nil, err
	}
	req.Header.Set("User-Agent", profileUA)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", nil, fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := progress.Copy(&buf, io.LimitReader(resp.Body, profileLimit), resp.ContentLength, "下载订阅", report); err != nil {
		return nil, "", nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	if buf.Len() == 0 {
		return nil, "", nil, fmt.Errorf("download %s: empty body", rawURL)
	}
	return buf.Bytes(),
		dispositionName(resp.Header.Get("Content-Disposition")),
		userInfo(resp.Header.Get("Subscription-Userinfo")), nil
}

// userInfo parses the subscription-userinfo header whose fields count
// bytes.
func userInfo(header string) *state.UserInfo {
	if header == "" {
		return nil
	}
	out := &state.UserInfo{}
	found := false
	for part := range strings.SplitSeq(header, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		number, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "upload":
			out.Upload = number
		case "download":
			out.Download = number
		case "total":
			out.Total = number
		case "expire":
			out.Expire = number
		default:
			continue
		}
		found = true
	}
	if !found {
		return nil
	}
	return out
}

// dispositionName extracts the advertised file name from a
// Content-Disposition header.
func dispositionName(header string) string {
	for part := range strings.SplitSeq(header, ";") {
		part = strings.TrimSpace(part)
		var value string
		switch {
		case strings.HasPrefix(part, "filename*="):
			value = strings.TrimPrefix(part, "filename*=")
			if i := strings.Index(value, "''"); i >= 0 {
				value = value[i+2:]
			}
			if decoded, err := url.QueryUnescape(value); err == nil {
				value = decoded
			}
		case strings.HasPrefix(part, "filename="):
			value = strings.TrimPrefix(part, "filename=")
		default:
			continue
		}
		value = strings.Trim(value, "\"")
		if value != "" {
			return value
		}
	}
	return ""
}

// cleanName turns an advertised file name into a profile name, dropping
// its extension and anything unsafe inside a path.
func cleanName(fileName string) string {
	name := strings.TrimSuffix(strings.TrimSuffix(fileName, ".yaml"), ".yml")
	name = strings.Trim(name, " .")
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == 0:
			b.WriteRune('_')
		case r < 0x20:
			continue
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
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
