// Package corecli holds the pieces shared across every yggdrasil
// subcommand that talks to a running yggdrasil-core: the on-disk
// configuration file, the HTTP client, and the data shapes exchanged
// with the server.
//
// The design mirrors kubectl's split: a persistent config at
// ~/.yggdrasil/config.yaml resolves "which server, which token" so
// every subcommand works without flag gymnastics, while flags remain
// available for one-off overrides.
package corecli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// ConfigFileName is the canonical location of the on-disk config
// (resolved relative to $HOME unless YGGDRASIL_CONFIG overrides).
const ConfigFileName = "config.yaml"

// Config captures everything a CLI subcommand needs to reach a
// yggdrasil-core and authenticate. Multi-context support (dev, staging,
// prod against the same laptop) is modeled after kubectl.
type Config struct {
	CurrentContext string               `json:"current_context,omitempty"`
	Contexts       map[string]*Context  `json:"contexts,omitempty"`
}

// Context is one named (server + credentials) pair.
type Context struct {
	Server       string `json:"server"`
	Token        string `json:"token,omitempty"`
	Collaborator string `json:"collaborator,omitempty"`
}

// Path returns the config file path, honoring $YGGDRASIL_CONFIG first
// and falling back to $HOME/.yggdrasil/config.yaml. It does not ensure
// the file exists — callers use Load for that.
func Path() (string, error) {
	if override := strings.TrimSpace(os.Getenv("YGGDRASIL_CONFIG")); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".yggdrasil", ConfigFileName), nil
}

// Load reads the config from disk. A missing file returns an empty
// Config (and no error) so `yggdrasil init` on a fresh machine does
// not require a pre-existing file.
func Load() (*Config, string, error) {
	path, err := Path()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Contexts: map[string]*Context{}}, path, nil
		}
		return nil, path, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, path, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Contexts == nil {
		cfg.Contexts = map[string]*Context{}
	}
	return &cfg, path, nil
}

// Save writes the config back to disk, creating the parent directory
// when needed. The file is chmod'd to 0600 because it holds the
// bearer token — same reasoning as kubectl.
func Save(cfg *Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Active resolves the selected context, honoring (in order):
//  1. $YGGDRASIL_CONTEXT
//  2. cfg.CurrentContext
//  3. The only context when exactly one exists
//
// Returns an empty Context and nil error when no contexts exist yet —
// callers decide whether "no config" is fatal (e.g. `get`, `apply`)
// or recoverable (e.g. `init`, `version`).
func (c *Config) Active() (*Context, string) {
	if c == nil {
		return nil, ""
	}
	if override := strings.TrimSpace(os.Getenv("YGGDRASIL_CONTEXT")); override != "" {
		if ctx, ok := c.Contexts[override]; ok {
			return ctx, override
		}
		return nil, override
	}
	if c.CurrentContext != "" {
		if ctx, ok := c.Contexts[c.CurrentContext]; ok {
			return ctx, c.CurrentContext
		}
	}
	if len(c.Contexts) == 1 {
		for name, ctx := range c.Contexts {
			return ctx, name
		}
	}
	return nil, ""
}

// SetContext upserts one context and makes it current. Useful for
// `yggdrasil init` (creates "local") and `yggdrasil login` (creates
// or refreshes any named context).
func (c *Config) SetContext(name string, ctx *Context, makeCurrent bool) {
	if c.Contexts == nil {
		c.Contexts = map[string]*Context{}
	}
	c.Contexts[name] = ctx
	if makeCurrent || c.CurrentContext == "" {
		c.CurrentContext = name
	}
}
