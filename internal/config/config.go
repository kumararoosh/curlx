package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// Config holds persistent user preferences.
type Config struct {
	SpecSources []string `json:"spec_sources"`
}

func configPath() string {
	p, err := xdg.ConfigFile("curlx/config.json")
	if err != nil {
		p = filepath.Join(os.TempDir(), "curlx-config.json")
	}
	return p
}

// Load reads the config from disk, returning an empty config if none exists.
func Load() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return &Config{}, nil
	}
	return &c, nil
}

// Save writes the config to disk.
func (c *Config) Save() error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// AddSource appends a spec source if not already present.
func (c *Config) AddSource(source string) {
	for _, s := range c.SpecSources {
		if s == source {
			return
		}
	}
	c.SpecSources = append(c.SpecSources, source)
}

// RemoveSource removes a spec source by value.
func (c *Config) RemoveSource(source string) {
	out := c.SpecSources[:0]
	for _, s := range c.SpecSources {
		if s != source {
			out = append(out, s)
		}
	}
	c.SpecSources = out
}
