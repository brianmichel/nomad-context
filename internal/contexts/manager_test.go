package contexts_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/brianmichel/nomad-context/internal/config"
	"github.com/brianmichel/nomad-context/internal/contexts"
)

func TestManagerUpsertAndToken(t *testing.T) {
	mgr := newTestManager(t)

	if err := mgr.Upsert("dev", "https://nomad.dev:4646", "secret"); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	ctx, err := mgr.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if ctx.Name != "dev" || ctx.Address != "https://nomad.dev:4646" {
		t.Fatalf("unexpected current context: %+v", ctx)
	}

	token, err := mgr.Token("dev")
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token != "secret" {
		t.Fatalf("Token() = %q, want %q", token, "secret")
	}
}

func TestManagerUpsertWithoutToken(t *testing.T) {
	mgr := newTestManager(t)

	if err := mgr.Upsert("dev", "https://nomad.dev:4646", ""); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	if _, err := mgr.Token("dev"); !errors.Is(err, contexts.ErrTokenNotFound) {
		t.Fatalf("Token() error = %v, want ErrTokenNotFound", err)
	}
}

func TestManagerUseAndResolve(t *testing.T) {
	mgr := newTestManager(t)

	if err := mgr.Upsert("dev", "https://dev", ""); err != nil {
		t.Fatalf("Upsert(dev) error = %v", err)
	}
	if err := mgr.Upsert("prod", "https://prod", "prod-token"); err != nil {
		t.Fatalf("Upsert(prod) error = %v", err)
	}

	if err := mgr.Use("prod"); err != nil {
		t.Fatalf("Use(prod) error = %v", err)
	}

	ctx, err := mgr.Resolve("")
	if err != nil {
		t.Fatalf("Resolve(\"\") error = %v", err)
	}
	if ctx.Name != "prod" {
		t.Fatalf("expected current context to be prod, got %q", ctx.Name)
	}

	ctx, err = mgr.Resolve("dev")
	if err != nil {
		t.Fatalf("Resolve(dev) error = %v", err)
	}
	if ctx.Name != "dev" {
		t.Fatalf("expected Resolve(dev) to return dev, got %q", ctx.Name)
	}
}

func TestManagerDeleteUpdatesCurrent(t *testing.T) {
	mgr := newTestManager(t)

	if err := mgr.Upsert("alpha", "https://alpha", ""); err != nil {
		t.Fatalf("Upsert(alpha) error = %v", err)
	}
	if err := mgr.Upsert("beta", "https://beta", "beta-token"); err != nil {
		t.Fatalf("Upsert(beta) error = %v", err)
	}

	if err := mgr.Use("beta"); err != nil {
		t.Fatalf("Use(beta) error = %v", err)
	}

	if err := mgr.Delete("beta"); err != nil {
		t.Fatalf("Delete(beta) error = %v", err)
	}

	ctx, err := mgr.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if ctx.Name != "alpha" {
		t.Fatalf("expected fallback current to be alpha, got %q", ctx.Name)
	}

	if _, err := mgr.Token("beta"); !errors.Is(err, contexts.ErrTokenNotFound) {
		t.Fatalf("expected beta token to be removed, got %v", err)
	}
}

func TestManagerResolveMissingContext(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Upsert("dev", "https://dev", ""); err != nil {
		t.Fatalf("Upsert error = %v", err)
	}

	if _, err := mgr.Resolve("missing"); !errors.Is(err, contexts.ErrContextNotFound) {
		t.Fatalf("Resolve(missing) error = %v, want ErrContextNotFound", err)
	}
}

func TestManagerUpsertWithAuthAndSaveLoginToken(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.UpsertWithAuth("dev", "https://dev", "corp-oidc", true, ""); err != nil {
		t.Fatalf("UpsertWithAuth() error = %v", err)
	}
	expiresAt := time.Date(2026, 9, 27, 12, 30, 0, 0, time.UTC)
	if err := mgr.SaveLoginToken("dev", "sso-secret", expiresAt); err != nil {
		t.Fatalf("SaveLoginToken() error = %v", err)
	}

	ctx, err := mgr.Resolve("dev")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if ctx.AuthMethod != "corp-oidc" || ctx.TokenExpiresAt != expiresAt.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected auth metadata: %+v", ctx)
	}
	if token, err := mgr.Token("dev"); err != nil || token != "sso-secret" {
		t.Fatalf("Token() = %q, %v; want sso-secret", token, err)
	}
}

func TestManagerChangingAuthMethodClearsOldToken(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.UpsertWithAuth("dev", "https://dev", "first-method", true, ""); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SaveLoginToken("dev", "old-secret", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := mgr.UpsertWithAuth("dev", "https://dev", "second-method", true, ""); err != nil {
		t.Fatalf("UpsertWithAuth() error = %v", err)
	}
	if _, err := mgr.Token("dev"); !errors.Is(err, contexts.ErrTokenNotFound) {
		t.Fatalf("Token() error = %v, want ErrTokenNotFound", err)
	}
	ctx, err := mgr.Resolve("dev")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.TokenExpiresAt != "" || ctx.AuthMethod != "second-method" {
		t.Fatalf("stale token metadata remains: %+v", ctx)
	}
}

func TestManagerUpsertRestoresTokenWhenConfigSaveFails(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.UpsertWithAuth("dev", "https://dev", "first-method", true, ""); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SaveLoginToken("dev", "old-secret", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Errorf("restore config file permissions: %v", err)
		}
	})

	if err := mgr.UpsertWithAuth("dev", "https://dev", "second-method", true, ""); err == nil {
		t.Fatal("UpsertWithAuth() succeeded with a read-only config directory")
	}
	if token, err := mgr.Token("dev"); err != nil || token != "old-secret" {
		t.Fatalf("Token() = %q, %v; want restored old-secret", token, err)
	}
	ctx, err := mgr.Resolve("dev")
	if err != nil {
		t.Fatal(err)
	}
	if ctx.AuthMethod != "first-method" {
		t.Fatalf("failed update changed auth method to %q", ctx.AuthMethod)
	}
}

func TestManagerLegacyConfigLoadsWithoutAuthFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NOMAD_CONTEXT_HOME", dir)
	legacy := `{"current_context":"dev","contexts":{"dev":{"name":"dev","address":"https://dev"}}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Contexts["dev"].AuthMethod != "" || cfg.Contexts["dev"].TokenExpiresAt != "" {
		t.Fatalf("legacy context got unexpected auth metadata: %+v", cfg.Contexts["dev"])
	}
}

func newTestManager(t *testing.T) *contexts.Manager {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("NOMAD_CONTEXT_HOME", dir)
	keyring.MockInit()
	return contexts.NewManager()
}
