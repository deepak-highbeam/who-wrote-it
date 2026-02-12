package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config holds all daemon configuration.
type Config struct {
	DataDir        string   `json:"data_dir"`
	SocketPath     string   `json:"socket_path"`
	DBPath         string   `json:"db_path"`
	WatchPaths     []string `json:"watch_paths"`
	IgnorePatterns []string `json:"ignore_patterns"`
}

// DefaultDataDir returns the default data directory (~/.whowroteit).
func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".whowroteit")
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	dataDir := DefaultDataDir()
	return &Config{
		DataDir:    dataDir,
		SocketPath: filepath.Join(dataDir, "whowroteit.sock"),
		DBPath:     filepath.Join(dataDir, "whowroteit.db"),
		WatchPaths: []string{},
		IgnorePatterns: []string{
			".git",
			"node_modules",
			"vendor",
			".DS_Store",
			"*.swp",
			"*.swo",
		},
	}
}

// Load reads configuration from a JSON file, falling back to defaults
// for any unset fields.
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file is fine, use defaults.
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Expand ~ in all path fields.
	cfg.DataDir = expandTilde(cfg.DataDir)
	cfg.SocketPath = expandTilde(cfg.SocketPath)
	cfg.DBPath = expandTilde(cfg.DBPath)
	for i, p := range cfg.WatchPaths {
		cfg.WatchPaths[i] = expandTilde(p)
	}

	// Re-derive paths if DataDir was overridden but socket/db paths were not.
	if cfg.SocketPath == "" {
		cfg.SocketPath = filepath.Join(cfg.DataDir, "whowroteit.sock")
	}
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "whowroteit.db")
	}

	return cfg, nil
}

// expandTilde replaces a leading ~ with the user's home directory.
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// EnsureDataDir creates the data directory if it does not exist.
func (c *Config) EnsureDataDir() error {
	return os.MkdirAll(c.DataDir, 0755)
}

// ConfigPath returns the default path to the config file.
func ConfigPath() string {
	return filepath.Join(DefaultDataDir(), "config.json")
}
