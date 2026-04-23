package corecli

import (
	"os"
	"path/filepath"
	"testing"
)

// withConfigEnv runs fn with YGGDRASIL_CONFIG pointed at a temp file
// so each test gets its own isolated state.
func withConfigEnv(t *testing.T, fn func(path string)) {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	t.Setenv("YGGDRASIL_CONFIG", path)
	fn(path)
}

func TestLoad_MissingFileReturnsEmptyConfig(t *testing.T) {
	withConfigEnv(t, func(path string) {
		cfg, gotPath, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if gotPath != path {
			t.Errorf("path = %q, want %q", gotPath, path)
		}
		if cfg == nil {
			t.Fatal("cfg is nil")
		}
		if len(cfg.Contexts) != 0 {
			t.Errorf("Contexts = %v, want empty", cfg.Contexts)
		}
	})
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withConfigEnv(t, func(_ string) {
		cfg := &Config{
			CurrentContext: "local",
			Contexts: map[string]*Context{
				"local": {
					Server:       "http://localhost:9080",
					Token:        "secret-token",
					Collaborator: "admin",
				},
			},
		}
		if err := Save(cfg); err != nil {
			t.Fatalf("Save error: %v", err)
		}

		loaded, _, err := Load()
		if err != nil {
			t.Fatalf("Load error: %v", err)
		}
		if loaded.CurrentContext != "local" {
			t.Errorf("CurrentContext = %q", loaded.CurrentContext)
		}
		ctx, ok := loaded.Contexts["local"]
		if !ok {
			t.Fatal("local context missing after roundtrip")
		}
		if ctx.Server != "http://localhost:9080" || ctx.Token != "secret-token" || ctx.Collaborator != "admin" {
			t.Errorf("context fields lost: %+v", ctx)
		}
	})
}

func TestSave_ChmodsConfigTo0600(t *testing.T) {
	withConfigEnv(t, func(path string) {
		cfg := &Config{Contexts: map[string]*Context{"x": {Server: "http://h", Token: "t"}}}
		if err := Save(cfg); err != nil {
			t.Fatalf("Save error: %v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		// On non-windows expect 0600. Skip on windows where mode bits
		// behave differently.
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Errorf("file mode = %o, want 0600", mode)
		}
	})
}

func TestActive_ResolvesByPriority(t *testing.T) {
	cases := []struct {
		name      string
		ctxOverride string
		current   string
		contexts  map[string]*Context
		want      string
	}{
		{
			name:    "no contexts → empty",
			contexts: map[string]*Context{},
			want:    "",
		},
		{
			name:    "single context auto-selected",
			contexts: map[string]*Context{"only": {Server: "http://only"}},
			want:    "only",
		},
		{
			name:    "current_context wins over single",
			current: "prod",
			contexts: map[string]*Context{
				"prod": {Server: "http://prod"},
				"dev":  {Server: "http://dev"},
			},
			want: "prod",
		},
		{
			name:        "env override wins over current_context",
			ctxOverride: "dev",
			current:     "prod",
			contexts: map[string]*Context{
				"prod": {Server: "http://prod"},
				"dev":  {Server: "http://dev"},
			},
			want: "dev",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ctxOverride != "" {
				t.Setenv("YGGDRASIL_CONTEXT", tc.ctxOverride)
			} else {
				t.Setenv("YGGDRASIL_CONTEXT", "")
			}
			cfg := &Config{CurrentContext: tc.current, Contexts: tc.contexts}
			_, name := cfg.Active()
			if name != tc.want {
				t.Errorf("name = %q, want %q", name, tc.want)
			}
		})
	}
}

func TestSetContext_PreservesOtherContexts(t *testing.T) {
	cfg := &Config{
		CurrentContext: "dev",
		Contexts: map[string]*Context{
			"dev": {Server: "http://dev"},
		},
	}
	cfg.SetContext("prod", &Context{Server: "http://prod", Token: "p"}, false)

	if cfg.CurrentContext != "dev" {
		t.Errorf("CurrentContext should not flip when makeCurrent=false; got %q", cfg.CurrentContext)
	}
	if _, ok := cfg.Contexts["dev"]; !ok {
		t.Error("dev context lost after SetContext")
	}
	if _, ok := cfg.Contexts["prod"]; !ok {
		t.Error("prod context not added")
	}
}
