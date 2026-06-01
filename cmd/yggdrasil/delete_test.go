package main

import (
	"strings"
	"testing"
)

func TestRunDelete_RequiresKindAndName(t *testing.T) {
	if err := runDelete(nil); err == nil {
		t.Fatal("expected error with no args")
	}
	if err := runDelete([]string{"workflow"}); err == nil {
		t.Fatal("expected error when name is missing")
	}
}

func TestConfirmYes(t *testing.T) {
	cases := map[string]bool{
		"y\n":   true,
		"yes\n": true,
		"Y\n":   true,
		"YES\n": true,
		"n\n":   false,
		"no\n":  false,
		"\n":    false,
		"":      false,
		"yep\n": false,
	}
	for in, want := range cases {
		if got := confirmYes(strings.NewReader(in)); got != want {
			t.Errorf("confirmYes(%q) = %v, want %v", in, got, want)
		}
	}
}
