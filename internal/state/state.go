package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultPort is the mixed http+socks listener port used when the JSON omits it.
// It avoids the 7890 clash family default so two proxy stacks never fight over
// one port.
const DefaultPort = 17890

// DefaultDelayURL is the latency test target used when the JSON omits it.
const DefaultDelayURL = "http://www.gstatic.com/generate_204"

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

// KeyOverrides maps action names to their bound keys per section.
type KeyOverrides struct {
	Global  map[string][]string `json:"global,omitempty"`
	Home    map[string][]string `json:"home,omitempty"`
	Library map[string][]string `json:"library,omitempty"`
	Input   map[string][]string `json:"input,omitempty"`
}

// State is the whole JSON document. The daemon is its only writer.
type State struct {
	Port     int           `json:"port"`
	DelayURL string        `json:"delay_url"`
	Lang     string        `json:"lang,omitempty"`
	Sort     int           `json:"sort,omitempty"`
	Page     int           `json:"page,omitempty"`
	Keys     *KeyOverrides `json:"keys,omitempty"`
	Enabled  bool          `json:"enabled"`
	Active   string        `json:"active"`
	Profiles []Profile     `json:"profiles"`
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
	if st.Profiles == nil {
		st.Profiles = []Profile{}
	}
}
