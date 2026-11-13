package prof

import (
	"context"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
)

// Node is one node of a profile with its protocol type.
type Node struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Info is the resolved node list of one profile.
type Info struct {
	Nodes []Node `json:"nodes"`
}

// providerTimeout bounds one proxy provider body download.
const providerTimeout = 20 * time.Second

var (
	cacheMu sync.Mutex
	cache   = map[string][]Node{}
)

// Parse reads a profile body and returns its full node list: DIRECT
// first, then the inline proxies and every proxy provider's proxies,
// deduplicated in config order.
func Parse(body []byte) (Info, error) {
	var doc rawDoc
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return Info{}, err
	}
	seen := map[string]bool{}
	nodes := []Node{}
	add := func(node Node) {
		if node.Name == "" || seen[node.Name] {
			return
		}
		seen[node.Name] = true
		nodes = append(nodes, node)
	}
	add(Node{Name: "DIRECT", Type: "direct"})
	for _, p := range doc.Proxies {
		add(Node{Name: p.Name, Type: p.Type})
	}
	for _, provider := range sortedProviderNames(doc.Providers) {
		for _, node := range providerNodes(doc, provider) {
			add(node)
		}
	}
	return Info{Nodes: nodes}, nil
}

// sortedProviderNames returns the proxy provider names in sorted order.
func sortedProviderNames(providers map[string]rawProvider) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// providerNodes returns the nodes one proxy provider contributes,
// downloading its body on first use.
func providerNodes(doc rawDoc, name string) []Node {
	spec, ok := doc.Providers[name]
	if !ok || spec.URL == "" {
		return nil
	}
	cacheMu.Lock()
	nodes, ok := cache[spec.URL]
	cacheMu.Unlock()
	if ok {
		return nodes
	}
	nodes = fetchProvider(spec.URL)
	cacheMu.Lock()
	cache[spec.URL] = nodes
	cacheMu.Unlock()
	return nodes
}

// fetchProvider downloads one proxy provider body and reads its nodes.
// A failed download contributes no nodes.
func fetchProvider(url string) []Node {
	ctx, cancel := context.WithTimeout(context.Background(), providerTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "clash.meta")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil
	}
	var body rawProviders
	if err := yaml.Unmarshal(data, &body); err != nil {
		return nil
	}
	nodes := []Node{}
	for _, p := range body.Proxies {
		nodes = append(nodes, Node{Name: p.Name, Type: p.Type})
	}
	return nodes
}

// rawDoc holds the node relevant fields of a Clash config body.
type rawDoc struct {
	Proxies   []rawNamed             `yaml:"proxies"`
	Providers map[string]rawProvider `yaml:"proxy-providers"`
	Groups    []rawGroup             `yaml:"proxy-groups"`
}

// rawNamed is a list entry carrying a name and a protocol type.
type rawNamed struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

// rawProvider is one proxy provider spec.
type rawProvider struct {
	URL string `yaml:"url"`
}

// rawGroup is one proxy group spec.
type rawGroup struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	Use     []string `yaml:"use"`
	Proxies []string `yaml:"proxies"`
}

// rawProviders is a provider body listing proxies.
type rawProviders struct {
	Proxies []rawNamed `yaml:"proxies"`
}
