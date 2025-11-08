package contexts_test

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"

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

func newTestManager(t *testing.T) *contexts.Manager {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("NOMAD_CONTEXT_HOME", dir)
	keyring.MockInit()
	return contexts.NewManager()
}
