package geo

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/metacubex/mihomo/config"

	"px/internal/progress"
)

// Item names one geodata file together with the URL it downloads from.
type Item struct {
	File string
	URL  string
}

// budget bounds one file download including all retries.
const budget = 15 * time.Minute

// attemptTimeout bounds one HTTP request inside a download.
const attemptTimeout = 2 * time.Minute

// retryDelay is the pause between two download attempts.
const retryDelay = 5 * time.Second

// Ensure downloads every missing geodata file a profile body needs into
// dir. Existing files are left untouched and interrupted downloads
// resume, so slow links survive connection drops. The kernel downloads
// geodata itself with a 90 second deadline, which slow links cannot
// meet; this package gives the same files a longer budget. The context
// aborts the work and report receives transfer shares.
func Ensure(ctx context.Context, dir string, body []byte, report func(string)) error {
	for _, item := range Need(body) {
		path := filepath.Join(dir, item.File)
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := fetch(ctx, item.URL, path, report); err != nil {
			return err
		}
	}
	return nil
}

// Need reports the geodata files a profile body references. A body that
// is not valid YAML reports nothing, and the kernel reports the parse
// error on its own.
func Need(body []byte) []Item {
	var doc rawDoc
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil
	}
	base := config.DefaultRawConfig().GeoXUrl
	items := []Item{}
	seen := map[string]bool{}
	add := func(file, profileURL, defaultURL string) {
		if seen[file] {
			return
		}
		seen[file] = true
		url := profileURL
		if url == "" {
			url = defaultURL
		}
		items = append(items, Item{File: file, URL: url})
	}
	for _, rule := range doc.Rules {
		switch ruleName(rule) {
		case "GEOIP", "SRC-GEOIP":
			if doc.GeodataMode {
				add("GeoIP.dat", doc.GeoXUrl.GeoIp, base.GeoIp)
			} else {
				add("geoip.metadb", doc.GeoXUrl.Mmdb, base.Mmdb)
			}
		case "GEOSITE":
			add("GeoSite.dat", doc.GeoXUrl.GeoSite, base.GeoSite)
		case "IP-ASN", "SRC-IP-ASN":
			add("ASN.mmdb", doc.GeoXUrl.ASN, base.ASN)
		}
	}
	return items
}

// fetch downloads url into path through a temporary file, resuming the
// temporary file on every retry and renaming it when complete.
func fetch(ctx context.Context, url, path string, report func(string)) error {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	part := path + ".part"
	var err error
	for attempt := 0; attempt < 30 && ctx.Err() == nil; attempt++ {
		if err = fetchOnce(ctx, url, part, report); err == nil {
			return os.Rename(part, path)
		}
		if ctx.Err() != nil {
			break
		}
		time.Sleep(retryDelay)
	}
	return fmt.Errorf("download %s: %w", url, err)
}

// fetchOnce runs one download request and appends to the partial file
// when the server honours a range request.
func fetchOnce(ctx context.Context, url, part string, report func(string)) error {
	offset := int64(0)
	if st, err := os.Stat(part); err == nil {
		offset = st.Size()
	}
	reqCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusPartialContent:
	case http.StatusOK:
		offset = 0
	case http.StatusRequestedRangeNotSatisfiable:
		return nil
	default:
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	flag := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(part, flag, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	total := resp.ContentLength
	if total > 0 {
		total += offset
	}
	_, err = progress.Copy(f, resp.Body, total, "下载 "+filepath.Base(part), report)
	return err
}

// rawDoc holds the geodata relevant fields of a Clash config body.
type rawDoc struct {
	GeodataMode bool     `yaml:"geodata-mode"`
	GeoXUrl     rawGeoX  `yaml:"geox-url"`
	Rules       []string `yaml:"rules"`
}

// rawGeoX holds the per profile geodata URL overrides.
type rawGeoX struct {
	GeoIp   string `yaml:"geoip"`
	Mmdb    string `yaml:"mmdb"`
	ASN     string `yaml:"asn"`
	GeoSite string `yaml:"geosite"`
}

// ruleName returns the uppercase type of one rule line.
func ruleName(rule string) string {
	name, _, _ := strings.Cut(rule, ",")
	return strings.ToUpper(strings.TrimSpace(name))
}
