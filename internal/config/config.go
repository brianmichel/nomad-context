package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	envHomeOverride = "NOMAD_CONTEXT_HOME"
	configFileName  = "config.json"
)

type Context struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type Config struct {
	Current  string              `json:"current_context"`
	Contexts map[string]*Context `json:"contexts"`
}

func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Contexts: map[string]*Context{}}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.ensure()
	return &cfg, nil
}

func Save(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}

	cfg.ensure()

	path, err := Path()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func Dir() (string, error) {
	if override := os.Getenv(envHomeOverride); override != "" {
		return override, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "nomad-context"), nil
}

func (c *Config) ensure() {
	if c.Contexts == nil {
		c.Contexts = make(map[string]*Context)
	}
}
