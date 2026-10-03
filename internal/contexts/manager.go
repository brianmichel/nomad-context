package contexts

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/brianmichel/nomad-context/internal/config"
)

const keyringService = "nomad-context"

var (
	ErrContextNotFound = errors.New("context not found")
	ErrNoCurrent       = errors.New("no current context configured")
	ErrTokenNotFound   = errors.New("token not found for context")
	ErrLoginRequired   = errors.New("SSO login required for context")
	ErrTokenExpired    = errors.New("stored Nomad token has expired")
)

type Manager struct {
	service string
}

func NewManager() *Manager {
	return &Manager{service: keyringService}
}

func (m *Manager) List() ([]*config.Context, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, "", err
	}

	contexts := make([]*config.Context, 0, len(cfg.Contexts))
	for _, c := range cfg.Contexts {
		contexts = append(contexts, c)
	}

	sort.Slice(contexts, func(i, j int) bool {
		return contexts[i].Name < contexts[j].Name
	})

	return contexts, cfg.Current, nil
}

func (m *Manager) Upsert(name, address, token string) error {
	return m.UpsertWithAuth(name, address, "", false, token)
}

func (m *Manager) UpsertWithAuth(name, address, authMethod string, authMethodSet bool, token string) error {
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)
	authMethod = strings.TrimSpace(authMethod)

	if name == "" {
		return errors.New("context name is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	existing, exists := cfg.Contexts[name]
	if address == "" {
		if exists {
			address = existing.Address
		} else {
			return errors.New("address is required")
		}
	}
	if exists && !authMethodSet {
		authMethod = existing.AuthMethod
	}
	authChanged := exists && authMethodSet && existing.AuthMethod != authMethod
	addressChanged := exists && existing.Address != address
	clearToken := authChanged || (addressChanged && authMethod != "")
	var oldToken string
	if clearToken {
		var err error
		oldToken, err = m.Token(name)
		if err != nil && !errors.Is(err, ErrTokenNotFound) {
			return err
		}
		if err := m.deleteToken(name); err != nil {
			return err
		}
	}

	cfg.Contexts[name] = &config.Context{
		Name:           name,
		Address:        address,
		AuthMethod:     authMethod,
		TokenExpiresAt: existingTokenExpiry(existing, token != "" || authChanged || (addressChanged && authMethod != "")),
	}

	if cfg.Current == "" {
		cfg.Current = name
	}

	if err := config.Save(cfg); err != nil {
		if oldToken != "" {
			if restoreErr := m.saveToken(name, oldToken); restoreErr != nil {
				return errors.Join(err, fmt.Errorf("restore existing token: %w", restoreErr))
			}
		}
		return err
	}

	if token != "" {
		if err := m.saveToken(name, token); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) Delete(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("%w: %s", ErrContextNotFound, name)
	}

	delete(cfg.Contexts, name)

	if cfg.Current == name {
		cfg.Current = pickNewCurrent(cfg.Contexts)
	}

	if err := config.Save(cfg); err != nil {
		return err
	}

	if err := m.deleteToken(name); err != nil {
		return err
	}

	return nil
}

func (m *Manager) Use(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("%w: %s", ErrContextNotFound, name)
	}

	cfg.Current = name
	return config.Save(cfg)
}

func (m *Manager) Current() (*config.Context, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	if cfg.Current == "" {
		return nil, ErrNoCurrent
	}

	current, ok := cfg.Contexts[cfg.Current]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrContextNotFound, cfg.Current)
	}

	return current, nil
}

func (m *Manager) Resolve(name string) (*config.Context, error) {
	if name == "" {
		return m.Current()
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	ctx, ok := cfg.Contexts[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrContextNotFound, name)
	}

	return ctx, nil
}

func (m *Manager) SaveToken(name, token string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("context name is required for token storage")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("token is empty")
	}

	if err := m.saveToken(name, token); err != nil {
		return err
	}
	return m.saveTokenExpiry(name, "")
}

// SaveLoginToken stores an SSO-issued Nomad token and its optional expiry.
func (m *Manager) SaveLoginToken(name, token string, expiresAt time.Time) error {
	name = strings.TrimSpace(name)
	token = strings.TrimSpace(token)
	if name == "" {
		return errors.New("context name is required for token storage")
	}
	if token == "" {
		return errors.New("token is empty")
	}
	if err := m.saveToken(name, token); err != nil {
		return err
	}
	expiry := ""
	if !expiresAt.IsZero() {
		expiry = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	return m.saveTokenExpiry(name, expiry)
}

func (m *Manager) Token(name string) (string, error) {
	token, err := keyring.Get(m.service, name)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", fmt.Errorf("%w: %s", ErrTokenNotFound, name)
		}
		return "", err
	}
	return token, nil
}

func (m *Manager) saveToken(name, token string) error {
	return keyring.Set(m.service, name, token)
}

func (m *Manager) deleteToken(name string) error {
	if err := keyring.Delete(m.service, name); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

func (m *Manager) saveTokenExpiry(name, expiry string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, ok := cfg.Contexts[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrContextNotFound, name)
	}
	ctx.TokenExpiresAt = expiry
	return config.Save(cfg)
}

func existingTokenExpiry(existing *config.Context, clear bool) string {
	if existing == nil || clear {
		return ""
	}
	return existing.TokenExpiresAt
}

func pickNewCurrent(contexts map[string]*config.Context) string {
	if len(contexts) == 0 {
		return ""
	}

	names := make([]string, 0, len(contexts))
	for name := range contexts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names[0]
}
