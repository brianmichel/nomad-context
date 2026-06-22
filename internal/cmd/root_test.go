package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestRootProxiesLeadingNomadOptions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "autocomplete install",
			args: []string{"-autocomplete-install"},
			want: "ARGS=-autocomplete-install",
		},
		{
			name: "address before command",
			args: []string{"-address=https://override.nomad.local:4646", "status"},
			want: "ARGS=-address=https://override.nomad.local:4646|status",
		},
		{
			name: "namespace before nested command",
			args: []string{"-namespace=platform", "job", "status", "example"},
			want: "ARGS=-namespace=platform|job|status|example",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordPath := setupProxyTest(t, "")

			root := NewRootCmd()
			root.SetArgs(tt.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			recorded := readRecord(t, recordPath)
			assertRecordContains(t, recorded, tt.want)
		})
	}
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

func TestRootKeepsLocalCommandsAndFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "help flag",
			args: []string{"--help"},
			want: "Manage Nomad CLI contexts or proxy commands to nomad",
		},
		{
			name: "short help flag",
			args: []string{"-h"},
			want: "Manage Nomad CLI contexts or proxy commands to nomad",
		},
		{
			name: "version flag",
			args: []string{"--version"},
			want: "nomad-context version ",
		},
		{
			name: "version command",
			args: []string{"version"},
			want: "nomad-context version ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			root := NewRootCmd()
			root.SetArgs(tt.args)
			root.SetOut(&out)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Fatalf("output missing %q:\n%s", tt.want, out.String())
			}
		})
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
	fakeNomad := filepath.Join(binDir, "nomad-test")
	if runtime.GOOS == "windows" {
		fakeNomad += ".exe"
	}
	sourcePath := filepath.Join(binDir, "main.go")
	source := strings.Join([]string{
		"package main",
		"",
		"import (",
		`  "fmt"`,
		`  "os"`,
		`  "strings"`,
		")",
		"",
		"func main() {",
		`  recordPath := os.Getenv("NOMAD_CONTEXT_TEST_RECORD")`,
		`  data := fmt.Sprintf("ARGS=%s\nNOMAD_ADDR=%s\nNOMAD_TOKEN=%s\n", strings.Join(os.Args[1:], "|"), os.Getenv("NOMAD_ADDR"), os.Getenv("NOMAD_TOKEN"))`,
		"  if err := os.WriteFile(recordPath, []byte(data), 0o644); err != nil {",
		"    panic(err)",
		"  }",
		"}",
	}, "\n")
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("WriteFile(fakeNomad source) error = %v", err)
	}
	build := exec.Command("go", "build", "-o", fakeNomad, sourcePath)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build fakeNomad error = %v:\n%s", err, output)
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
