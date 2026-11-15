package core

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/outboundgroup"
	"github.com/metacubex/mihomo/config"
	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/listener"
	LC "github.com/metacubex/mihomo/listener/config"
	"github.com/metacubex/mihomo/tunnel"
)

// DelayTimeout bounds one latency test run.
const DelayTimeout = 5 * time.Second

// tunDevice is the name of the TUN interface px creates.
const tunDevice = "px0"

// GroupInfo describes one proxy group of the loaded profile.
type GroupInfo struct {
	Name     string   `json:"name"`
	Selected string   `json:"selected"`
	Nodes    []string `json:"nodes"`
}

var mu sync.Mutex

// Init points the kernel home directory at dir, where geodata files and
// kernel caches live. The kernel resolves its geodata paths by listing
// that directory, so the directory must exist before any Apply.
func Init(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	constant.SetHomeDir(dir)
	return nil
}

// Validate parses profile bytes without touching the running kernel.
func Validate(data []byte) error {
	_, err := parse(data)
	return err
}

// Apply parses profile bytes and rebuilds the listeners. A zero port
// keeps the kernel silent while rules and proxies stay ready.
func Apply(data []byte, port int) error {
	mu.Lock()
	defer mu.Unlock()
	cfg, err := parse(data)
	if err != nil {
		return err
	}
	patchInbound(cfg, port)
	executor.ApplyConfig(cfg, true)
	return nil
}

// Start opens the TUN device and the mixed listener.
func Start(port int) {
	mu.Lock()
	defer mu.Unlock()
	listener.ReCreateTun(tunConfig(), tunnel.Tunnel)
	listener.ReCreateMixed(port, tunnel.Tunnel)
}

// stopTimeout bounds the TUN and listener teardown so a stuck teardown
// cannot block shutdown.
const stopTimeout = 5 * time.Second

// Stop closes every inbound listener this package may have opened. It
// gives the teardown a deadline so shutdown always converges.
func Stop() {
	mu.Lock()
	defer mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		listener.ReCreateTun(LC.Tun{}, tunnel.Tunnel)
		listener.ReCreateMixed(0, tunnel.Tunnel)
	}()
	select {
	case <-done:
	case <-time.After(stopTimeout):
	}
}

// TunUp reports whether the TUN device exists on this machine.
func TunUp() bool {
	_, err := net.InterfaceByName(tunDevice)
	return err == nil
}

// Select sets the chosen node of a proxy group.
func Select(group, node string) error {
	mu.Lock()
	defer mu.Unlock()
	proxy, ok := tunnel.Proxies()[group]
	if !ok {
		return fmt.Errorf("group %s not found", group)
	}
	selector, ok := proxy.Adapter().(outboundgroup.SelectAble)
	if !ok {
		return fmt.Errorf("group %s does not accept node selection", group)
	}
	return selector.Set(node)
}

// Groups returns every proxy group of the loaded profile.
func Groups() []GroupInfo {
	mu.Lock()
	defer mu.Unlock()
	groups := []GroupInfo{}
	for name, proxy := range tunnel.Proxies() {
		if name == "GLOBAL" {
			continue
		}
		group, ok := proxy.Adapter().(outboundgroup.ProxyGroup)
		if !ok {
			continue
		}
		info := GroupInfo{Name: name, Selected: group.Now(), Nodes: []string{}}
		for _, node := range group.Proxies() {
			info.Nodes = append(info.Nodes, node.Name())
		}
		groups = append(groups, info)
	}
	sortGroups(groups)
	return groups
}

// Delay tests every node of one group and returns latency in milliseconds.
// A zero value marks a timeout or a failed test.
func Delay(group, url string, timeout time.Duration) (map[string]uint16, error) {
	mu.Lock()
	proxy, ok := tunnel.Proxies()[group]
	if !ok {
		mu.Unlock()
		return nil, fmt.Errorf("group %s not found", group)
	}
	target, ok := proxy.Adapter().(outboundgroup.ProxyGroup)
	if !ok {
		mu.Unlock()
		return nil, fmt.Errorf("group %s has no nodes", group)
	}
	mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return target.URLTest(ctx, url, nil)
}

// DelayProfile tests the named nodes of one profile body and returns
// latency in milliseconds. A zero value marks a timeout or a failed test.
func DelayProfile(body []byte, names []string, url string, timeout time.Duration) map[string]uint16 {
	out := make(map[string]uint16, len(names))
	cfg, err := parse(body)
	if err != nil {
		return out
	}
	defer func() {
		for _, proxy := range cfg.Proxies {
			_ = proxy.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var wg sync.WaitGroup
	var mu2 sync.Mutex
	for _, name := range names {
		proxy, ok := cfg.Proxies[name]
		if !ok {
			continue
		}
		wg.Add(1)
		go func(name string, proxy constant.Proxy) {
			defer wg.Done()
			ms, err := proxy.URLTest(ctx, url, nil)
			if err != nil {
				ms = 0
			}
			mu2.Lock()
			out[name] = ms
			mu2.Unlock()
		}(name, proxy)
	}
	wg.Wait()
	return out
}

// parse converts profile bytes into a kernel config and fills the DNS
// baseline a profile may leave out.
func parse(data []byte) (*config.Config, error) {
	raw, err := config.UnmarshalRawConfig(data)
	if err != nil {
		return nil, err
	}
	fillDNS(raw)
	return config.ParseRawConfig(raw)
}

// fillDNS enables DNS with default servers when a profile leaves DNS
// off, because hijacked queries get no answer without a resolver.
func fillDNS(raw *config.RawConfig) {
	if raw.DNS.Enable {
		return
	}
	raw.DNS.Enable = true
	raw.DNS.EnhancedMode = constant.DNSFakeIP
	raw.DNS.NameServer = []string{"https://223.5.5.5/dns-query", "223.6.6.6"}
}

// tunConfig builds the TUN section px applies while TUN mode is on. The
// stack and the interface addressing follow the kernel defaults.
func tunConfig() LC.Tun {
	return LC.Tun{
		Enable:              true,
		Device:              tunDevice,
		Stack:               constant.TunMips,
		DNSHijack:           []string{"0.0.0.0:53"},
		AutoRoute:           true,
		AutoDetectInterface: true,
		Inet4Address:        []netip.Prefix{netip.MustParsePrefix("198.18.0.1/30")},
		Inet6Address:        []netip.Prefix{netip.MustParsePrefix("fdfe:dcba:9876::1/126")},
	}
}

func patchInbound(cfg *config.Config, port int) {
	in := &cfg.General.Inbound
	in.Port = 0
	in.SocksPort = 0
	in.RedirPort = 0
	in.TProxyPort = 0
	in.MixedPort = port
	if port > 0 {
		in.Tun = tunConfig()
	} else {
		in.Tun = LC.Tun{}
	}
	in.TuicServer = LC.TuicServer{}
	in.ShadowSocksConfig = ""
	in.VmessConfig = ""
}

func sortGroups(groups []GroupInfo) {
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
}
