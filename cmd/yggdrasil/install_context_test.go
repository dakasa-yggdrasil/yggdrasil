package main

import (
	"path/filepath"
	"testing"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// install previously resolved the core URL/token only from env vars
// (YGGDRASIL_URL / YGGDRASIL_WORKFLOW_RUN_TOKEN), so a logged-in user had
// to repeat --server and hit a 401. It must now fall back to the active
// context like get/apply/deploy.

func TestInstallCoreTarget_FallsBackToContext(t *testing.T) {
	t.Setenv("YGGDRASIL_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	cfg := &corecli.Config{
		CurrentContext: "prod",
		Contexts: map[string]*corecli.Context{
			"prod": {Server: "https://core.example", Token: "ys_session"},
		},
	}
	if err := corecli.Save(cfg); err != nil {
		t.Fatal(err)
	}

	base, tok, err := installCoreTarget("", "")
	if err != nil {
		t.Fatalf("installCoreTarget: %v", err)
	}
	if base != "https://core.example" || tok != "ys_session" {
		t.Errorf("got server=%q token=%q, want the active context's values", base, tok)
	}
}

func TestInstallCoreTarget_FlagWinsOverContext(t *testing.T) {
	t.Setenv("YGGDRASIL_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	cfg := &corecli.Config{
		CurrentContext: "prod",
		Contexts:       map[string]*corecli.Context{"prod": {Server: "https://core.example", Token: "ctx"}},
	}
	if err := corecli.Save(cfg); err != nil {
		t.Fatal(err)
	}

	base, tok, err := installCoreTarget("https://override.example", "flagtok")
	if err != nil {
		t.Fatalf("installCoreTarget: %v", err)
	}
	if base != "https://override.example" || tok != "flagtok" {
		t.Errorf("explicit server/token must win, got %q/%q", base, tok)
	}
}

func TestInstallCoreTarget_NoContextErrors(t *testing.T) {
	t.Setenv("YGGDRASIL_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	if _, _, err := installCoreTarget("", ""); err == nil {
		t.Fatal("expected an error when no server is configured anywhere")
	}
}
