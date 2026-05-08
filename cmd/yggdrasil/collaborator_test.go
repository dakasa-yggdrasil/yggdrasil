package main

import (
	"strings"
	"testing"
)

// runCollaborator dispatches by verb. We don't have an injected core client
// in these subcommands — they read config via FromContext — so end-to-end
// CLI execution is covered by the corecli package tests. Here we focus on
// argument parsing/validation since that's the layer unique to CLI.

func TestRunCollaborator_RequiresSubcommand(t *testing.T) {
	if err := runCollaborator(nil); err == nil {
		t.Fatal("expected error when no subcommand provided")
	}
}

func TestRunCollaborator_HelpExits0(t *testing.T) {
	for _, verb := range []string{"help", "-h", "--help"} {
		if err := runCollaborator([]string{verb}); err != nil {
			t.Errorf("%s should not error: %v", verb, err)
		}
	}
}

func TestRunCollaborator_UnknownVerb(t *testing.T) {
	err := runCollaborator([]string{"frobnicate"})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected 'unknown' error, got %v", err)
	}
}

// --- Argument validation (no network) -------------------------------------
//
// These tests assert the per-verb required-flag checks fire BEFORE any HTTP
// call. They don't need a fake server because the failure path returns the
// validation error first.

func TestRunCollaboratorCreate_RequiresSlugAndDisplayName(t *testing.T) {
	cases := [][]string{
		{},
		{"--slug", "alice"},
		{"--display-name", "Alice"},
	}
	for _, args := range cases {
		err := runCollaboratorCreate(args)
		if err == nil || !strings.Contains(err.Error(), "required") {
			t.Errorf("args=%v: expected required error, got %v", args, err)
		}
	}
}

func TestRunCollaboratorOffboard_RequiresIDAndReason(t *testing.T) {
	if err := runCollaboratorOffboard([]string{"--reason", "voluntary"}); err == nil {
		t.Error("expected error when ID is missing")
	}
	if err := runCollaboratorOffboard([]string{"alice"}); err == nil {
		t.Error("expected error when --reason is missing")
	}
}

func TestRunCollaboratorRoleChange_RequiresIDAndNewRole(t *testing.T) {
	if err := runCollaboratorRoleChange(nil); err == nil {
		t.Error("expected error when args empty")
	}
	if err := runCollaboratorRoleChange([]string{"alice"}); err == nil {
		t.Error("expected error when --new-role missing")
	}
}

func TestRunCollaboratorTeamAdd_RequiresIDAndTeam(t *testing.T) {
	if err := runCollaboratorTeamAdd([]string{"alice"}); err == nil {
		t.Error("expected error when --team missing")
	}
	if err := runCollaboratorTeamAdd([]string{"--team", "team-1"}); err == nil {
		t.Error("expected error when ID missing")
	}
}

func TestRunCollaboratorTeamRemove_RequiresIDAndTeam(t *testing.T) {
	if err := runCollaboratorTeamRemove(nil); err == nil {
		t.Error("expected error when args empty")
	}
	if err := runCollaboratorTeamRemove([]string{"alice"}); err == nil {
		t.Error("expected error when --team missing")
	}
}

func TestRunCollaboratorAttributeSet_RequiresIDKeyValue(t *testing.T) {
	cases := [][]string{
		nil,
		{"alice"},
		{"alice", "--key", "level"},
	}
	for _, args := range cases {
		if err := runCollaboratorAttributeSet(args); err == nil {
			t.Errorf("args=%v: expected error for missing required flags", args)
		}
	}
}

func TestRunCollaboratorManagerChange_RequiresIDAndNewManager(t *testing.T) {
	if err := runCollaboratorManagerChange([]string{"alice"}); err == nil {
		t.Error("expected error when --new-manager missing")
	}
	if err := runCollaboratorManagerChange([]string{"--new-manager", "m-1"}); err == nil {
		t.Error("expected error when ID missing")
	}
}

func TestRunCollaboratorAbsenceStart_RequiresIDTypeFromTo(t *testing.T) {
	cases := [][]string{
		{"alice"},
		{"alice", "--type", "vacation"},
		{"alice", "--type", "vacation", "--from", "2026-07-01"},
	}
	for _, args := range cases {
		if err := runCollaboratorAbsenceStart(args); err == nil {
			t.Errorf("args=%v: expected required error", args)
		}
	}
}

func TestRunCollaboratorGet_RequiresID(t *testing.T) {
	if err := runCollaboratorGet(nil); err == nil {
		t.Error("expected error when ID missing")
	}
}

func TestRunCollaboratorUpdate_RequiresID(t *testing.T) {
	if err := runCollaboratorUpdate(nil); err == nil {
		t.Error("expected error when ID missing")
	}
}

func TestRunCollaboratorSuspend_RequiresID(t *testing.T) {
	if err := runCollaboratorSuspend(nil); err == nil {
		t.Error("expected error when ID missing")
	}
}

func TestRunCollaboratorUnsuspend_RequiresID(t *testing.T) {
	if err := runCollaboratorUnsuspend(nil); err == nil {
		t.Error("expected error when ID missing")
	}
}

func TestRunCollaboratorReOnboard_RequiresID(t *testing.T) {
	if err := runCollaboratorReOnboard(nil); err == nil {
		t.Error("expected error when ID missing")
	}
}

func TestRunCollaboratorAbsenceEnd_RequiresID(t *testing.T) {
	if err := runCollaboratorAbsenceEnd(nil); err == nil {
		t.Error("expected error when ID missing")
	}
}

func TestRunCollaboratorLifecycleEvents_RequiresID(t *testing.T) {
	if err := runCollaboratorLifecycleEvents(nil); err == nil {
		t.Error("expected error when ID missing")
	}
}

func TestRunCollaboratorProviderState_RequiresID(t *testing.T) {
	if err := runCollaboratorProviderState(nil); err == nil {
		t.Error("expected error when ID missing")
	}
}
