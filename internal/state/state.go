package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"px/internal/keymap"
)

// DefaultPort is the mixed http+socks listener port used when the JSON omits it.
// It avoids the 7890 clash family default so two proxy stacks never fight over
// one port.
const DefaultPort = 17890

// DefaultDelayURL is the latency test target used when the JSON omits it.
const DefaultDelayURL = "http://www.gstatic.com/generate_204"

// DefaultChartWidth is the traffic chart width in cells used when the JSON
// omits it.
const DefaultChartWidth = 50

// UserInfo is the subscription traffic accounting carried by the
// subscription-userinfo response header, all values in bytes.
type UserInfo struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire,omitempty"`
}

// Profile is one subscription entry in the state file.
type Profile struct {
	Name      string            `json:"name"`
	URL       string            `json:"url"`
	UpdatedAt string            `json:"updated_at,omitempty"`
	Traffic   *UserInfo         `json:"traffic,omitempty"`
	Selected  map[string]string `json:"selected,omitempty"`
}

// KeyOverrides maps action names to their bound keys per section, the
// shape the config file stores.
type KeyOverrides = keymap.Overrides

// BrowsePos is the remembered list position of one profile.
type BrowsePos struct {
	Cursor int `json:"cursor"`
	Offset int `json:"offset"`
}

// BrowseState is the remembered position of the Library page.
type BrowseState struct {
	Inside     string               `json:"inside,omitempty"`
	Width      int                  `json:"width"`
	Height     int                  `json:"height"`
	CursorDir  int                  `json:"cursor_dir"`
	OffsetDir  int                  `json:"offset_dir"`
	CursorItem int                  `json:"cursor_item"`
	OffsetItem int                  `json:"offset_item"`
	History    map[string]BrowsePos `json:"history,omitempty"`
}

// State is the whole JSON document. The daemon is its only writer.
type State struct {
	Port       int           `json:"port"`
	DelayURL   string        `json:"delay_url"`
	ChartWidth int           `json:"chart_width"`
	Lang       string        `json:"lang"`
	Sort       int           `json:"sort"`
	Align      bool          `json:"align"`
	Browse     BrowseState   `json:"browse"`
	Page       int           `json:"page"`
	Keys       *KeyOverrides `json:"keys"`
	Enabled    bool          `json:"enabled"`
	Active     string        `json:"active"`
	Profiles   []Profile     `json:"profiles"`
}

// Dir returns the directory that holds the state file and profile files.
func Dir(configPath string) string {
	return filepath.Dir(configPath)
}

// ProfilesDir returns the directory that holds downloaded profile files.
func ProfilesDir(configPath string) string {
	return filepath.Join(Dir(configPath), "profiles")
}

// ProfilePath returns the YAML file path of one profile.
func ProfilePath(configPath, name string) string {
	return filepath.Join(ProfilesDir(configPath), name+".yaml")
}

// Load reads the state file and fills defaults for missing fields.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	st.applyDefaults()
	return &st, nil
}

// Save writes the state file atomically through a temporary file and a rename.
func Save(path string, st *State) error {
	if err := os.MkdirAll(Dir(path), 0o755); err != nil {
		return err
	}
	st.applyDefaults()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Find returns the index of a profile by name, or -1.
func (st *State) Find(name string) int {
	for i := range st.Profiles {
		if st.Profiles[i].Name == name {
			return i
		}
	}
	return -1
}

// FindURL returns the index of a profile by subscription URL, or -1.
func (st *State) FindURL(url string) int {
	for i := range st.Profiles {
		if st.Profiles[i].URL == url {
			return i
		}
	}
	return -1
}

func (st *State) applyDefaults() {
	if st.Port <= 0 || st.Port > 65535 {
		st.Port = DefaultPort
	}
	if st.DelayURL == "" {
		st.DelayURL = DefaultDelayURL
	}
	if st.ChartWidth <= 0 {
		st.ChartWidth = DefaultChartWidth
	}
	st.ChartWidth = max(20, min(st.ChartWidth, 200))
	if st.Lang == "" {
		st.Lang = "en"
	}
	if st.Keys == nil {
		st.Keys = keymap.AsOverrides(keymap.DefaultKeymap())
	}
	if st.Profiles == nil {
		st.Profiles = []Profile{}
	}
	if st.Browse.History == nil {
		st.Browse.History = map[string]BrowsePos{}
	}
}
