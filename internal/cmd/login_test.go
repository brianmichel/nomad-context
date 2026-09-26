package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"

	"github.com/brianmichel/nomad-context/internal/config"
	"github.com/brianmichel/nomad-context/internal/contexts"
)

func TestPerformNomadLoginStoresTokenAndTargetsContext(t *testing.T) {
	manager := setupLoginManager(t)
	bin, record := buildLoginHelper(t)
	t.Setenv(nomadBinaryEnv, bin)
	t.Setenv("NOMAD_CONTEXT_LOGIN_JSON", `{"SecretID":"sso-secret","ExpirationTime":"2026-09-27T12:30:00Z"}`)
	t.Setenv("NOMAD_CONTEXT_LOGIN_RECORD", record)
	t.Setenv("NOMAD_TOKEN", "unrelated-shell-token")

	cmd := &cobra.Command{}
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	expiresAt, err := performNomadLogin(cmd, &config.Context{
		Name: "dev", Address: "https://dev.nomad.local:4646", AuthMethod: "corp-oidc",
	}, "127.0.0.1:4650", manager)
	if err != nil {
		t.Fatalf("performNomadLogin() error = %v", err)
	}
	wantExpiry := time.Date(2026, 9, 27, 12, 30, 0, 0, time.UTC)
	if !expiresAt.Equal(wantExpiry) {
		t.Fatalf("expiry = %s, want %s", expiresAt, wantExpiry)
	}
	if token, err := manager.Token("dev"); err != nil || token != "sso-secret" {
		t.Fatalf("Token() = %q, %v; want sso-secret", token, err)
	}
	ctx, err := manager.Resolve("dev")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.TokenExpiresAt != wantExpiry.Format(time.RFC3339Nano) {
		t.Fatalf("stored expiry = %q", ctx.TokenExpiresAt)
	}
	recorded, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ARGS=login|-json|-method=corp-oidc|-oidc-callback-addr=127.0.0.1:4650",
		"NOMAD_ADDR=https://dev.nomad.local:4646",
		"NOMAD_TOKEN=",
	} {
		if !strings.Contains(string(recorded), want) {
			t.Errorf("login invocation missing %q:\n%s", want, recorded)
		}
	}
	if strings.Contains(stderr.String(), "sso-secret") {
		t.Fatal("login wrote secret to stderr")
	}
}

func TestPerformNomadLoginAllowsMissingExpiry(t *testing.T) {
	manager := setupLoginManager(t)
	bin, record := buildLoginHelper(t)
	t.Setenv(nomadBinaryEnv, bin)
	t.Setenv("NOMAD_CONTEXT_LOGIN_JSON", `{"SecretID":"no-expiry-secret"}`)
	t.Setenv("NOMAD_CONTEXT_LOGIN_RECORD", record)

	expiresAt, err := performNomadLogin(&cobra.Command{}, &config.Context{
		Name: "dev", Address: "https://dev", AuthMethod: "oidc",
	}, "", manager)
	if err != nil {
		t.Fatalf("performNomadLogin() error = %v", err)
	}
	if !expiresAt.IsZero() {
		t.Fatalf("expiry = %s, want unknown", expiresAt)
	}
	ctx, err := manager.Resolve("dev")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.TokenExpiresAt != "" {
		t.Fatalf("stored expiry = %q, want empty", ctx.TokenExpiresAt)
	}
}

func TestPerformNomadLoginRejectsBadResultsAndKeepsExistingToken(t *testing.T) {
	tests := []struct {
		name   string
		output string
		exit   string
		want   string
	}{
		{name: "invalid JSON", output: "not-json", want: "invalid JSON"},
		{name: "missing token", output: `{"AccessorID":"accessor"}`, want: "returned no token"},
		{name: "nonzero exit", output: `{"SecretID":"new-secret"}`, exit: "9", want: "Nomad login failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := setupLoginManager(t)
			if err := manager.SaveLoginToken("dev", "previous-secret", time.Now().Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			bin, record := buildLoginHelper(t)
			t.Setenv(nomadBinaryEnv, bin)
			t.Setenv("NOMAD_CONTEXT_LOGIN_JSON", tt.output)
			t.Setenv("NOMAD_CONTEXT_LOGIN_EXIT", tt.exit)
			t.Setenv("NOMAD_CONTEXT_LOGIN_RECORD", record)

			_, err := performNomadLogin(&cobra.Command{}, &config.Context{
				Name: "dev", Address: "https://dev", AuthMethod: "oidc",
			}, "", manager)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("performNomadLogin() error = %v, want containing %q", err, tt.want)
			}
			if token, getErr := manager.Token("dev"); getErr != nil || token != "previous-secret" {
				t.Fatalf("failed login replaced existing token: %q, %v", token, getErr)
			}
		})
	}
}

func TestCtxLoginCommandRunsConfiguredNomadLogin(t *testing.T) {
	setupLoginManager(t)
	bin, record := buildLoginHelper(t)
	t.Setenv(nomadBinaryEnv, bin)
	t.Setenv("NOMAD_CONTEXT_LOGIN_JSON", `{"SecretID":"command-secret"}`)
	t.Setenv("NOMAD_CONTEXT_LOGIN_RECORD", record)

	var out bytes.Buffer
	root := NewRootCmd()
	root.SetArgs([]string{"ctx", "login", "dev"})
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), `Logged in to context "dev"`) {
		t.Fatalf("unexpected command output: %s", out.String())
	}
	if strings.Contains(out.String(), "command-secret") {
		t.Fatal("login command exposed token in output")
	}
}

func TestCtxSetCommandPersistsSSOSettings(t *testing.T) {
	t.Setenv("NOMAD_CONTEXT_HOME", t.TempDir())
	keyring.MockInit()
	root := NewRootCmd()
	root.SetArgs([]string{"ctx", "set", "corp", "--addr", "https://corp", "--auth-method", "corp-oidc"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	ctx, err := contexts.NewManager().Resolve("corp")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Address != "https://corp" || ctx.AuthMethod != "corp-oidc" {
		t.Fatalf("unexpected saved context: %+v", ctx)
	}
}

func TestRunNomadRequiresLoginForSSOContextWithoutToken(t *testing.T) {
	setupLoginManager(t)
	_, record := buildLoginHelper(t)
	err := runNomad([]string{"status"}, contexts.NewManager())
	if !errors.Is(err, contexts.ErrLoginRequired) {
		t.Fatalf("runNomad() error = %v, want ErrLoginRequired", err)
	}
	if _, statErr := os.Stat(record); !os.IsNotExist(statErr) {
		t.Fatalf("Nomad command ran without an SSO token (record stat error %v)", statErr)
	}
}

func TestRunNomadRejectsExpiredSSOTokenWithoutExecutingCommand(t *testing.T) {
	record := setupProxyTest(t, "expired-secret")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Contexts["dev"].AuthMethod = "corp-oidc"
	cfg.Contexts["dev"].TokenExpiresAt = time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	err = runNomad([]string{"job", "stop", "dangerous"}, contexts.NewManager())
	if !errors.Is(err, contexts.ErrTokenExpired) {
		t.Fatalf("runNomad() error = %v, want ErrTokenExpired", err)
	}
	if _, statErr := os.Stat(record); !os.IsNotExist(statErr) {
		t.Fatalf("Nomad command ran despite expired token (record stat error %v)", statErr)
	}
}

func setupLoginManager(t *testing.T) *contexts.Manager {
	t.Helper()
	t.Setenv("NOMAD_CONTEXT_HOME", t.TempDir())
	keyring.MockInit()
	mgr := contexts.NewManager()
	if err := mgr.UpsertWithAuth("dev", "https://dev.nomad.local:4646", "corp-oidc", true, ""); err != nil {
		t.Fatal(err)
	}
	return mgr
}

func buildLoginHelper(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	source := `package main
import (
  "fmt"
  "os"
  "strings"
)
func main() {
	  record := fmt.Sprintf("ARGS=%s\nNOMAD_ADDR=%s\nNOMAD_TOKEN=%s\n", strings.Join(os.Args[1:], "|"), os.Getenv("NOMAD_ADDR"), os.Getenv("NOMAD_TOKEN"))
  if err := os.WriteFile(os.Getenv("NOMAD_CONTEXT_LOGIN_RECORD"), []byte(record), 0600); err != nil { panic(err) }
  fmt.Print(os.Getenv("NOMAD_CONTEXT_LOGIN_JSON"))
  if exit := os.Getenv("NOMAD_CONTEXT_LOGIN_EXIT"); exit != "" { code := 1; fmt.Sscanf(exit, "%d", &code); os.Exit(code) }
}`
	sourcePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "nomad-login-helper")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, sourcePath)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build login helper: %v\n%s", err, output)
	}
	record := filepath.Join(dir, "record.txt")
	return bin, record
}
