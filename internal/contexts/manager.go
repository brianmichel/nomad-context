package contexts

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/brianmichel/nomad-context/internal/config"
)

const keyringService = "nomad-context"

var (
	ErrContextNotFound = errors.New("context not found")
	ErrNoCurrent       = errors.New("no current context configured")
	ErrTokenNotFound   = errors.New("token not found for context")
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
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)

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

	cfg.Contexts[name] = &config.Context{
		Name:    name,
		Address: address,
	}

	if cfg.Current == "" {
		cfg.Current = name
	}

	if err := config.Save(cfg); err != nil {
		return err
	}

	if token != "" {
		return m.saveToken(name, token)
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

	if err := keyring.Delete(m.service, name); err != nil && !errors.Is(err, keyring.ErrNotFound) {
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

	return m.saveToken(name, token)
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
