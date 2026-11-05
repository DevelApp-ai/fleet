package prefs

import (
	"os"
	"path/filepath"
)

// Path is the config file: $FLEET_CONFIG, else
// $XDG_CONFIG_HOME/fleet/config.toml, else ~/.config/fleet/config.toml.
func Path() string {
	if p := os.Getenv("FLEET_CONFIG"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "fleet", "config.toml")
}
