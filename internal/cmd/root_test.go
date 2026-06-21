package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/brianmichel/nomad-context/internal/config"
	"github.com/brianmichel/nomad-context/internal/contexts"
)

func TestRootProxiesUnknownFlagsToNomadSubcommands(t *testing.T) {
	recordPath := setupProxyTest(t, "dev-token")

	root := NewRootCmd()
	root.SetArgs([]string{"jobs", "-json"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	recorded := readRecord(t, recordPath)
	assertRecordContains(t, recorded, "ARGS=jobs|-json")
	assertRecordContains(t, recorded, "NOMAD_ADDR=https://dev.nomad.local:4646")
	assertRecordContains(t, recorded, "NOMAD_TOKEN=dev-token")
}

func TestRootProxiesUnknownFlagsAfterNestedNomadSubcommands(t *testing.T) {
	recordPath := setupProxyTest(t, "")

	root := NewRootCmd()
	root.SetArgs([]string{"job", "status", "-json", "example"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	recorded := readRecord(t, recordPath)
	assertRecordContains(t, recorded, "ARGS=job|status|-json|example")
	assertRecordContains(t, recorded, "NOMAD_ADDR=https://dev.nomad.local:4646")
	assertRecordContains(t, recorded, "NOMAD_TOKEN=")
}

func TestRootDoesNotIgnoreUnknownContextCommandFlags(t *testing.T) {
	t.Setenv("NOMAD_CONTEXT_HOME", t.TempDir())
	keyring.MockInit()

	root := NewRootCmd()
	root.SetArgs([]string{"ctx", "set", "dev", "--adrr", "https://dev.nomad.local:4646"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want unknown flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag: --adrr") {
		t.Fatalf("Execute() error = %v, want unknown flag error", err)
	}
}

func setupProxyTest(t *testing.T, token string) string {
	t.Helper()

	configHome := t.TempDir()
	t.Setenv("NOMAD_CONTEXT_HOME", configHome)
	keyring.MockInit()

	cfg := &config.Config{
		Current: "dev",
		Contexts: map[string]*config.Context{
			"dev": {
				Name:    "dev",
				Address: "https://dev.nomad.local:4646",
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if token != "" {
		if err := contexts.NewManager().SaveToken("dev", token); err != nil {
			t.Fatalf("SaveToken() error = %v", err)
		}
	}

	binDir := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "nomad-record")
	fakeNomad := filepath.Join(binDir, "nomad")
	script := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		"args=",
		"for arg do",
		"  if [ -n \"$args\" ]; then args=\"$args|$arg\"; else args=\"$arg\"; fi",
		"done",
		"{",
		"  printf 'ARGS=%s\\n' \"$args\"",
		"  printf 'NOMAD_ADDR=%s\\n' \"${NOMAD_ADDR-}\"",
		"  printf 'NOMAD_TOKEN=%s\\n' \"${NOMAD_TOKEN-}\"",
		"} > \"$NOMAD_CONTEXT_TEST_RECORD\"",
	}, "\n")
	if err := os.WriteFile(fakeNomad, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(fakeNomad) error = %v", err)
	}

	t.Setenv(nomadBinaryEnv, fakeNomad)
	t.Setenv("NOMAD_CONTEXT_TEST_RECORD", recordPath)
	t.Setenv("NOMAD_TOKEN", "outer-token")
	return recordPath
}

func readRecord(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(record) error = %v", err)
	}
	return string(data)
}

func assertRecordContains(t *testing.T, record string, want string) {
	t.Helper()
	if !strings.Contains(record, want) {
		t.Fatalf("record missing %q:\n%s", want, record)
	}
}
