// Package config persists user preferences in config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const currentVersion = 1

// Config holds the user preferences.
type Config struct {
	OutputDir string `json:"outputDir"`
	LastMode  string `json:"lastMode"`
	Version   int    `json:"version"`
}

// Default returns the configuration used when no valid file exists.
func Default() Config {
	return Config{
		OutputDir: defaultOutputDir(),
		LastMode:  "video_audio",
		Version:   currentVersion,
	}
}

func defaultOutputDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, name := range []string{"Videos", "Downloads"} {
		dir := filepath.Join(home, name)
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return filepath.Join(home, "Downloads")
}

// Load reads the configuration at path. A missing or invalid file yields the
// defaults: it never blocks the application startup.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("config unreadable, using defaults", "path", path, "err", err)
		}
		return Default(), nil
	}
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("config invalid, using defaults", "path", path, "err", err)
		return Default(), nil
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = Default().OutputDir
	}
	return cfg, nil
}

// Save writes cfg atomically (temporary file then rename).
func Save(path string, cfg Config) error {
	if cfg.Version == 0 {
		cfg.Version = currentVersion
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// UsableOutputDir returns cfg.OutputDir if it is an existing directory.
// Otherwise it returns the default directory (falling back to the user home)
// and a non-empty warning in French for the UI.
func UsableOutputDir(dir string) (string, string) {
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return dir, ""
	}
	def := Default().OutputDir
	if st, err := os.Stat(def); err != nil || !st.IsDir() {
		if home, err := os.UserHomeDir(); err == nil {
			def = home
		}
	}
	return def, "Le dossier de sortie précédent est introuvable : " + def + " est utilisé à la place."
}
