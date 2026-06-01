package main

import (
	"strings"
	"testing"
)

func TestRunDiff_RequiresFile(t *testing.T) {
	if err := runDiff(nil); err == nil {
		t.Fatal("expected error when -f is missing")
	}
}

func TestLineDiff_Changes(t *testing.T) {
	out := lineDiff([]string{"a", "b", "c"}, []string{"a", "x", "c"})
	if !strings.Contains(out, "-b") || !strings.Contains(out, "+x") {
		t.Errorf("expected -b and +x in diff:\n%s", out)
	}
	if !strings.Contains(out, " a") {
		t.Errorf("expected unchanged context line for 'a':\n%s", out)
	}
}

func TestLineDiff_Identical(t *testing.T) {
	if got := lineDiff([]string{"a", "b"}, []string{"a", "b"}); got != "" {
		t.Errorf("identical inputs should diff empty, got %q", got)
	}
}

func TestSpecToYAML_CanonicalisesKeyOrder(t *testing.T) {
	// Two specs with the same content but different key order must
	// normalise to the same YAML so the diff doesn't show phantom changes.
	a, err := specToYAML(map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	b, err := specToYAML(map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("key order should not matter:\n%q\nvs\n%q", a, b)
	}
}
