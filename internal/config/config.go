// Package config loads tetra's user-level config from TOML.
//
// The file lives at $XDG_CONFIG_HOME/tetra/config.toml (or
// ~/.config/tetra/config.toml). A missing file is not an error — defaults
// apply. CLI flags and arguments override config values.
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the deserialized form of the user's config.toml. Fields that
// were absent in the file keep their Default() values. true_color is a
// pointer so we can distinguish "not set" (nil = autodetect) from an
// explicit false.
type Config struct {
	Theme     string `toml:"theme"`
	TrueColor *bool  `toml:"true_color"`
	Icons     bool   `toml:"icons"`
}

func Default() Config {
	return Config{
		Theme: "default",
		Icons: true,
	}
}

// Load reads cfg from path. If path == "", DefaultPath() is used. A
// missing file returns Default() with no error.
func Load(path string) (Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	// Decode merges into cfg: keys absent from the file leave the
	// corresponding default in place.
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Default(), err
	}
	return cfg, nil
}

// DefaultPath returns the canonical config location, honoring
// $XDG_CONFIG_HOME when set.
func DefaultPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "tetra", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to a relative path; Load() will treat it as missing.
		return filepath.Join(".config", "tetra", "config.toml")
	}
	return filepath.Join(home, ".config", "tetra", "config.toml")
}
