package config_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/brianmichel/nomad-context/internal/config"
)

func TestLoadCreatesDefaultConfig(t *testing.T) {
	setConfigHome(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Current != "" {
		t.Fatalf("expected no current context, got %q", cfg.Current)
	}
	if cfg.Contexts == nil {
		t.Fatalf("expected contexts map to be initialized")
	}
	if len(cfg.Contexts) != 0 {
		t.Fatalf("expected zero contexts, got %d", len(cfg.Contexts))
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := setConfigHome(t)

	original := &config.Config{
		Current: "dev",
		Contexts: map[string]*config.Context{
			"dev": {
				Name:    "dev",
				Address: "https://dev.nomad.local:4646",
			},
		},
	}

	if err := config.Save(original); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	path, err := config.Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	reloaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !reflect.DeepEqual(original, reloaded) {
		t.Fatalf("reloaded config mismatch, want %+v got %+v", original, reloaded)
	}

	if !strings.HasPrefix(path, dir) {
		t.Fatalf("config path %q should be inside %q", path, dir)
	}
}

func TestDirHonorsEnvOverride(t *testing.T) {
	override := setConfigHome(t)

	got, err := config.Dir()
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}

	if got != override {
		t.Fatalf("Dir() = %q, want %q", got, override)
	}
}

func setConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("NOMAD_CONTEXT_HOME", dir)
	return dir
}
