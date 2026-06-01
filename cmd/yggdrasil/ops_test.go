package main

import (
	"strings"
	"testing"
)

func TestRunOps_RequiresSubcommand(t *testing.T) {
	if err := runOps(nil); err == nil {
		t.Fatal("expected error when no subcommand given")
	}
}

func TestRunOps_UnknownSubcommand(t *testing.T) {
	err := runOps([]string{"frobnicate"})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected 'unknown' error, got %v", err)
	}
}

func TestRunOps_HelpExits0(t *testing.T) {
	for _, h := range []string{"help", "-h", "--help"} {
		if err := runOps([]string{h}); err != nil {
			t.Errorf("%s should not error: %v", h, err)
		}
	}
}

func TestRunOpsGet_RequiresRunID(t *testing.T) {
	if err := runOps([]string{"get"}); err == nil {
		t.Fatal("expected error when run id is missing")
	}
}

func TestRunOpsActions_RequireRunID(t *testing.T) {
	for _, verb := range []string{"retry", "abort", "replay"} {
		if err := runOps([]string{verb}); err == nil {
			t.Errorf("%s: expected error when run id is missing", verb)
		}
	}
}
