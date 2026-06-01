package main

import "testing"

// Like the other subcommands, runRun reads config via FromContext, so we
// cover the CLI-unique layer here: positional/flag parsing and the input
// validation that must fire before any network call.

func TestRunRun_RequiresWorkflowName(t *testing.T) {
	if err := runRun(nil); err == nil {
		t.Fatal("expected error when no workflow name is given")
	}
	if err := runRun([]string{"-n", "global"}); err == nil {
		t.Fatal("expected error when only flags are given (no workflow name)")
	}
}

func TestParseInputPairs_KeyValue(t *testing.T) {
	m, err := parseInputPairs([]string{"region=us-east-1", "msg=hello world"})
	if err != nil {
		t.Fatalf("parseInputPairs: %v", err)
	}
	if m["region"] != "us-east-1" || m["msg"] != "hello world" {
		t.Errorf("unexpected inputs: %+v", m)
	}
}

func TestParseInputPairs_RejectsMissingEquals(t *testing.T) {
	for _, bad := range []string{"noequals", "=novalue"} {
		if _, err := parseInputPairs([]string{bad}); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestRunRun_RejectsBadInputBeforeNetwork(t *testing.T) {
	// A malformed --input must error out during arg validation, never
	// reaching the (here unreachable) server.
	if err := runRun([]string{"my-workflow", "--input", "noequals"}); err == nil {
		t.Fatal("expected error for malformed --input")
	}
}
