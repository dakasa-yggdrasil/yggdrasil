package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// Synthetic sentinel only. Failure messages must never format it, request
// bodies, returned provider errors, or workflow result objects.
const privateInputFixture = "synthetic-private-input-fixture"
const privateRunFixtureID = "f1000000-0000-4000-8000-000000000001"

func privateWorkflowFixture() map[string]any {
	return map[string]any{
		"id":       "f2000000-0000-4000-8000-000000000001",
		"kind":     "workflow",
		"version":  float64(3),
		"metadata": map[string]any{"name": "private-writer", "namespace": "ops", "active": true},
		"spec": map[string]any{"input_schema": map[string]any{"properties": map[string]any{
			"credential": map[string]any{"type": "string", "sensitive": true},
			"second":     map[string]any{"type": "string", "secret": true},
			"confirm":    map[string]any{"type": "string"},
		}}},
	}
}

func privateJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if json.NewEncoder(w).Encode(payload) != nil {
		t.Error("could not write synthetic response")
	}
}

func privateFakeIO(out *bytes.Buffer, prompt func(context.Context, string) (string, error)) privateInputIO {
	return privateInputIO{terminal: func() bool { return true }, prompt: prompt, out: out}
}

func TestRunPrivateRejectsKeysAndOrdinaryConflictsBeforeNetworkOrPrompt(t *testing.T) {
	for _, args := range [][]string{
		{"--input-secret", "credential=" + privateInputFixture},
		{"--input-secret", "credential", "--input-secret", "credential"},
		{"--input-secret", "credential", "--input", "credential=ordinary"},
		{"--input-secret", " "},
		{"--input-secret", "credential\n"},
	} {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
		var out bytes.Buffer
		prompted := false
		io := privateFakeIO(&out, func(context.Context, string) (string, error) { prompted = true; return privateInputFixture, nil })
		command := append([]string{"private-writer", "--server", server.URL}, args...)
		err := runRunWithPrivateIO(command, io)
		server.Close()
		if !errors.Is(err, errPrivateInputKeys) || prompted || calls.Load() != 0 || out.Len() != 0 {
			t.Fatal("invalid private keys must fail before any network request or prompt")
		}
		if strings.Contains(err.Error(), privateInputFixture) {
			t.Fatal("private argument validation exposed its rejected value")
		}
	}
}

func TestPrivateManifestRequiresExactActiveWorkflowAndExplicitSecretSchema(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		accept bool
	}{
		{name: "sensitive string", accept: true},
		{name: "missing record ID", mutate: func(m map[string]any) { delete(m, "id") }},
		{name: "missing version", mutate: func(m map[string]any) { delete(m, "version") }},
		{name: "fractional version", mutate: func(m map[string]any) { m["version"] = float64(1.5) }},
		{name: "secret string", mutate: func(m map[string]any) {
			m["spec"].(map[string]any)["input_schema"].(map[string]any)["properties"].(map[string]any)["credential"] = map[string]any{"type": "string", "secret": true}
		}, accept: true},
		{name: "missing active", mutate: func(m map[string]any) { delete(m["metadata"].(map[string]any), "active") }},
		{name: "inactive", mutate: func(m map[string]any) { m["metadata"].(map[string]any)["active"] = false }},
		{name: "different name", mutate: func(m map[string]any) { m["metadata"].(map[string]any)["name"] = "different" }},
		{name: "different namespace", mutate: func(m map[string]any) { m["metadata"].(map[string]any)["namespace"] = "different" }},
		{name: "different kind", mutate: func(m map[string]any) { m["kind"] = "integration_type" }},
		{name: "missing schema", mutate: func(m map[string]any) { m["spec"] = map[string]any{} }},
		{name: "undeclared", mutate: func(m map[string]any) {
			delete(m["spec"].(map[string]any)["input_schema"].(map[string]any)["properties"].(map[string]any), "credential")
		}},
		{name: "ordinary property", mutate: func(m map[string]any) {
			m["spec"].(map[string]any)["input_schema"].(map[string]any)["properties"].(map[string]any)["credential"] = map[string]any{"type": "string"}
		}},
		{name: "nonboolean marker", mutate: func(m map[string]any) {
			m["spec"].(map[string]any)["input_schema"].(map[string]any)["properties"].(map[string]any)["credential"] = map[string]any{"type": "string", "sensitive": "true"}
		}},
		{name: "nonstring property", mutate: func(m map[string]any) {
			m["spec"].(map[string]any)["input_schema"].(map[string]any)["properties"].(map[string]any)["credential"] = map[string]any{"type": "object", "secret": true}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := privateWorkflowFixture()
			if tc.mutate != nil {
				tc.mutate(manifest)
			}
			if verifyPrivateInputManifest([]map[string]any{manifest}, "ops", "private-writer", []string{"credential"}) != tc.accept {
				t.Fatal("unexpected private schema validation result")
			}
		})
	}
	valid := privateWorkflowFixture()
	if verifyPrivateInputManifest(nil, "ops", "private-writer", []string{"credential"}) || verifyPrivateInputManifest([]map[string]any{valid, valid}, "ops", "private-writer", []string{"credential"}) {
		t.Fatal("missing or ambiguous workflows must be refused")
	}
}

func TestRunPrivateChecksActualManifestThenDispatchesOnceAsyncWithoutResultContent(t *testing.T) {
	var gets, posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets.Add(1)
			q := r.URL.Query()
			if r.URL.Path != "/api/v1/manifests" || q.Get("kind") != "workflow" || q.Get("namespace") != "ops" || q.Get("name") != "private-writer" || q.Get("active_only") != "true" {
				t.Error("private preflight must use the canonical filtered active workflow GET")
			}
			privateJSON(t, w, map[string]any{"manifests": []map[string]any{privateWorkflowFixture()}})
		case http.MethodPost:
			posts.Add(1)
			if r.URL.Path != "/api/v1/workflow-runs" || r.URL.Query().Get("async") != "true" {
				t.Error("private input must force async on the existing run endpoint")
			}
			var body struct {
				Workflow map[string]any `json:"workflow"`
				Inputs   map[string]any `json:"inputs"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.Inputs["credential"] != privateInputFixture || body.Inputs["second"] != privateInputFixture || body.Inputs["confirm"] != "REGISTER" {
				t.Error("private and ordinary inputs must reach the one dispatch unchanged")
			}
			if body.Workflow["namespace"] != "ops" || body.Workflow["name"] != "private-writer" || body.Workflow["version"] != float64(3) {
				t.Error("private dispatch must pin the exact preflight workflow version")
			}
			if _, present := body.Workflow["manifest_id"]; present {
				t.Error("private dispatch must retain active-version resolution")
			}
			privateJSON(t, w, map[string]any{"run_id": privateRunFixtureID, "status": "pending", "workflow": map[string]any{"echo": privateInputFixture}, "metadata": map[string]any{"echo": privateInputFixture}, "steps": []map[string]any{{"error": privateInputFixture}}})
		default:
			t.Error("unexpected request")
		}
	}))
	defer server.Close()
	var out bytes.Buffer
	prompts := 0
	io := privateFakeIO(&out, func(context.Context, string) (string, error) {
		if gets.Load() != 1 || posts.Load() != 0 {
			t.Error("secrets must be collected only after manifest validation and before dispatch")
		}
		prompts++
		return privateInputFixture, nil
	})
	err := runRunWithPrivateIO([]string{"private-writer", "-n", "ops", "--server", server.URL, "--input", "confirm=REGISTER", "--input-secret", "credential", "--input-secret", "second", "--async=false"}, io)
	if err != nil || gets.Load() != 1 || posts.Load() != 1 || prompts != 2 {
		t.Fatal("private dispatch must collect validated keys and make one async request")
	}
	if out.String() != "run "+privateRunFixtureID+"\nstatus pending\n" || strings.Contains(out.String(), privateInputFixture) {
		t.Fatal("private dispatch output must contain only the validated run ID and status")
	}
}

func TestRunPrivateRefusesNonTTYAndInvalidManifestWithoutPrompt(t *testing.T) {
	for _, tty := range []bool{false, true} {
		var gets, posts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				posts.Add(1)
			}
			gets.Add(1)
			privateJSON(t, w, map[string]any{"manifests": []map[string]any{}})
		}))
		var out bytes.Buffer
		prompted := false
		io := privateFakeIO(&out, func(context.Context, string) (string, error) { prompted = true; return privateInputFixture, nil })
		io.terminal = func() bool { return tty }
		err := runPrivateWorkflow(context.Background(), corecli.NewClient(server.URL, ""), "ops", "private-writer", map[string]any{}, []string{"credential"}, io)
		server.Close()
		want := errPrivateTerminal
		if tty {
			want = errPrivateManifest
		}
		if !errors.Is(err, want) || prompted || posts.Load() != 0 || out.Len() != 0 || !tty && gets.Load() != 0 {
			t.Fatal("non-TTY or unverified workflow must not collect or dispatch private input")
		}
	}
}

func TestRunPrivateRejectsCanceledOrInvalidEntryAndErasesCollectedMapValues(t *testing.T) {
	for _, value := range []string{"", " ", " leading", "trailing ", "line\nbreak", "control\x00", string([]byte{0xff}), strings.Repeat("x", 64*1024+1)} {
		if validPrivateValue(value) {
			t.Fatal("invalid private value accepted")
		}
	}
	for _, canceled := range []bool{false, true} {
		var posts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				posts.Add(1)
			}
			privateJSON(t, w, map[string]any{"manifests": []map[string]any{privateWorkflowFixture()}})
		}))
		var out bytes.Buffer
		prompt := 0
		io := privateFakeIO(&out, func(context.Context, string) (string, error) {
			prompt++
			if prompt == 1 {
				return privateInputFixture, nil
			}
			if canceled {
				return "", errors.New(privateInputFixture)
			}
			return " ", nil
		})
		inputs := map[string]any{"confirm": "REGISTER"}
		err := runPrivateWorkflow(context.Background(), corecli.NewClient(server.URL, ""), "ops", "private-writer", inputs, []string{"credential", "second"}, io)
		server.Close()
		if err == nil || strings.Contains(err.Error(), privateInputFixture) || posts.Load() != 0 || out.Len() != 0 || len(inputs) != 1 || inputs["confirm"] != "REGISTER" {
			t.Fatal("failed private collection must send nothing, hide values and clear the input map")
		}
	}
}

func TestRunPrivateDispatchErrorsAndRedirectsNeverLeakOrRetry(t *testing.T) {
	for _, scenario := range []string{"api", "malformed", "missing ID", "invalid status", "redirect", "transport"} {
		t.Run(scenario, func(t *testing.T) {
			var posts, redirected atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					privateJSON(t, w, map[string]any{"manifests": []map[string]any{privateWorkflowFixture()}})
					return
				}
				posts.Add(1)
				switch scenario {
				case "api":
					w.WriteHeader(http.StatusBadRequest)
					privateJSON(t, w, map[string]any{"error": privateInputFixture})
				case "malformed":
					_, _ = w.Write([]byte(`{"echo":"` + privateInputFixture + `",broken}`))
				case "missing ID":
					privateJSON(t, w, map[string]any{"status": "pending", "metadata": privateInputFixture})
				case "invalid status":
					privateJSON(t, w, map[string]any{"run_id": privateRunFixtureID, "status": privateInputFixture})
				case "redirect":
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
				case "transport":
					connection, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error("could not simulate an unconfirmed transport result")
						return
					}
					_ = connection.Close()
				}
			}))
			defer server.Close()
			var out bytes.Buffer
			inputs := map[string]any{}
			err := runPrivateWorkflow(context.Background(), corecli.NewClient(server.URL, "machine-fixture"), "ops", "private-writer", inputs, []string{"credential"}, privateFakeIO(&out, func(context.Context, string) (string, error) { return privateInputFixture, nil }))
			if !errors.Is(err, errPrivateDispatch) || strings.Contains(err.Error(), privateInputFixture) || out.Len() != 0 || posts.Load() != 1 || redirected.Load() != 0 || len(inputs) != 0 {
				t.Fatal("private dispatch failure must remain opaque, single-call and without redirected replay")
			}
		})
	}
}

func TestRunPrivateContextCancellationAfterEntryDoesNotSendOrExposeValue(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
		privateJSON(t, w, map[string]any{"manifests": []map[string]any{privateWorkflowFixture()}})
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out bytes.Buffer
	inputs := map[string]any{}
	err := runPrivateWorkflow(ctx, corecli.NewClient(server.URL, ""), "ops", "private-writer", inputs, []string{"credential"}, privateFakeIO(&out, func(context.Context, string) (string, error) {
		cancel()
		return privateInputFixture, nil
	}))
	if err == nil || strings.Contains(err.Error(), privateInputFixture) || posts.Load() != 0 || out.Len() != 0 || len(inputs) != 0 {
		t.Fatal("canceled private request must send nothing, expose nothing and release map references")
	}
}

func TestPrivateRunResponseRejectsEvenValidLookingEchoes(t *testing.T) {
	result := corecli.WorkflowRunResult{RunID: privateRunFixtureID, Status: "pending"}
	for _, value := range []string{privateRunFixtureID, "pending"} {
		if validPrivateRunResponse(result, []string{"credential"}, map[string]any{"credential": value}) {
			t.Fatal("a syntactically valid response field must not echo the collected input")
		}
	}
}

func TestOrdinaryRunInputsKeepTheirExistingStringAndSyncBehavior(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Inputs map[string]any `json:"inputs"`
		}
		if r.Method != http.MethodPost || r.URL.RawQuery != "" || json.NewDecoder(r.Body).Decode(&body) != nil || body.Inputs["count"] != "008" {
			t.Error("ordinary duplicate --input keys must retain last-string-value and sync behavior")
		}
		privateJSON(t, w, map[string]any{"status": "committed"})
	}))
	defer server.Close()
	err := runRunWithPrivateIO([]string{"ordinary", "--server", server.URL, "--input", "count=007", "--input", "count=008"}, privateInputIO{terminal: func() bool { t.Fatal("ordinary run must not require a TTY"); return false }})
	if err != nil || calls.Load() != 1 {
		t.Fatal("ordinary --input workflow behavior changed")
	}
}
