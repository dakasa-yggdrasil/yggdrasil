package main

import (
	"path/filepath"
	"testing"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

func TestRunConfig_RequiresSubcommand(t *testing.T) {
	if err := runConfig(nil); err == nil {
		t.Fatal("expected error when no subcommand given")
	}
}

func TestRunConfig_UnknownSubcommand(t *testing.T) {
	if err := runConfig([]string{"frobnicate"}); err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
}

func TestConfig_UseContext_RequiresName(t *testing.T) {
	if err := runConfig([]string{"use-context"}); err == nil {
		t.Fatal("expected error when context name is missing")
	}
}

func TestConfig_UseContext_RejectsUnknownContext(t *testing.T) {
	t.Setenv("YGGDRASIL_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	if err := runConfig([]string{"use-context", "ghost"}); err == nil {
		t.Fatal("expected error for a context that does not exist")
	}
}

func TestConfig_SetUseRoundtrip(t *testing.T) {
	t.Setenv("YGGDRASIL_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))

	if err := runConfig([]string{"set-context", "prod", "--server", "https://prod.example", "--token", "ys_prod"}); err != nil {
		t.Fatalf("set-context prod: %v", err)
	}
	if err := runConfig([]string{"set-context", "dev", "--server", "http://localhost:9080"}); err != nil {
		t.Fatalf("set-context dev: %v", err)
	}
	if err := runConfig([]string{"use-context", "prod"}); err != nil {
		t.Fatalf("use-context prod: %v", err)
	}

	cfg, _, err := corecli.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.CurrentContext != "prod" {
		t.Errorf("current context = %q, want prod", cfg.CurrentContext)
	}
	if cfg.Contexts["prod"] == nil || cfg.Contexts["prod"].Server != "https://prod.example" {
		t.Errorf("prod server not persisted: %+v", cfg.Contexts["prod"])
	}
	if cfg.Contexts["dev"] == nil || cfg.Contexts["dev"].Server != "http://localhost:9080" {
		t.Errorf("dev server not persisted: %+v", cfg.Contexts["dev"])
	}
}
