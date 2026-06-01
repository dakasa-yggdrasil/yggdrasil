package main

import "testing"

func TestRunRollback_RequiresKindNameAndVersion(t *testing.T) {
	if err := runRollback(nil); err == nil {
		t.Fatal("expected error with no args")
	}
	if err := runRollback([]string{"workflow"}); err == nil {
		t.Fatal("expected error when name is missing")
	}
	if err := runRollback([]string{"workflow", "hello"}); err == nil {
		t.Fatal("expected error when --to is missing")
	}
	if err := runRollback([]string{"workflow", "hello", "--to", "0"}); err == nil {
		t.Fatal("expected error when --to is not a positive version")
	}
}

func TestManifestVersion(t *testing.T) {
	// JSON numbers decode to float64 in a map[string]any.
	if v, ok := manifestVersion(map[string]any{"version": float64(7)}); !ok || v != 7 {
		t.Errorf("manifestVersion = %d,%v want 7,true", v, ok)
	}
	if _, ok := manifestVersion(map[string]any{}); ok {
		t.Error("expected ok=false when version absent")
	}
}
