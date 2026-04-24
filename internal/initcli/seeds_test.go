package initcli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestDefaultTopologyInstances_Shape guards the structural contract
// the standalone compose relies on: HTTP transport endpoints, service
// names that match what docker-compose.standalone.yml exposes, and
// the two required integration_type references by name. These are the
// values a future compose rename would invalidate — that rename needs
// to update both the compose file and this helper in lockstep.
func TestDefaultTopologyInstances_Shape(t *testing.T) {
	instances := defaultTopologyInstances()
	if got, want := len(instances), 2; got != want {
		t.Fatalf("instance count = %d, want %d", got, want)
	}

	expected := map[string]struct {
		typeRef string
		baseURL string
	}{
		"yggdrasil-core-kubernetes": {
			typeRef: "kubernetes",
			baseURL: "http://integration-kubernetes:8081",
		},
		"yggdrasil-core-schema-migrations": {
			typeRef: "schema-migrations-goose-postgres",
			baseURL: "http://integration-schema-migrations:8082",
		},
	}

	for _, inst := range instances {
		metadata, _ := inst["metadata"].(map[string]any)
		name, _ := metadata["name"].(string)
		exp, ok := expected[name]
		if !ok {
			t.Errorf("unexpected instance name %q", name)
			continue
		}
		if ns, _ := metadata["namespace"].(string); ns != "global" {
			t.Errorf("%s namespace = %q, want global", name, ns)
		}
		spec, _ := inst["spec"].(map[string]any)
		config, _ := spec["config"].(map[string]any)
		baseURL, _ := config["base_url"].(string)
		if baseURL != exp.baseURL {
			t.Errorf("%s config.base_url = %q, want %q", name, baseURL, exp.baseURL)
		}
		typeRef, _ := spec["type_ref"].(map[string]any)
		refName, _ := typeRef["name"].(string)
		if refName != exp.typeRef {
			t.Errorf("%s type_ref.name = %q, want %q", name, refName, exp.typeRef)
		}
	}
}

// TestApplyTopologyInstances_PostsExpectedManifests exercises the
// full apply path against a fake HTTP server that mimics
// POST /api/v1/manifests?kind=integration_instance. We capture the
// bodies posted and assert both instances are pushed with the right
// kind query param and required fields. No real core needed.
func TestApplyTopologyInstances_PostsExpectedManifests(t *testing.T) {
	var (
		mu      sync.Mutex
		posted  []map[string]any
		posts   int
		kinds   []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/manifests" && r.Method == http.MethodPost:
			kind := r.URL.Query().Get("kind")
			body, _ := io.ReadAll(r.Body)
			var parsed map[string]any
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Errorf("decode posted body: %v", err)
			}
			mu.Lock()
			posts++
			posted = append(posted, parsed)
			kinds = append(kinds, kind)
			mu.Unlock()
			// Core returns the created manifest — shape doesn't
			// matter for this test, we only care that POST succeeds.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"00000000-0000-0000-0000-000000000000"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	if err := applyTopologyInstances(context.Background(), srv.URL, "test-token"); err != nil {
		t.Fatalf("applyTopologyInstances error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if posts != 2 {
		t.Fatalf("POST count = %d, want 2", posts)
	}
	for _, kind := range kinds {
		if kind != "integration_instance" {
			t.Errorf("kind query param = %q, want integration_instance", kind)
		}
	}
	gotNames := map[string]bool{}
	for _, body := range posted {
		// The core's POST /api/v1/manifests endpoint takes a flat
		// payload (name/namespace/spec at top level), not a nested
		// manifest envelope.
		name, _ := body["name"].(string)
		gotNames[name] = true
	}
	for _, want := range []string{"yggdrasil-core-kubernetes", "yggdrasil-core-schema-migrations"} {
		if !gotNames[want] {
			t.Errorf("missing posted manifest for %q", want)
		}
	}
}

// TestApplyTopologyInstances_FailsFastOnCoreError verifies the caller
// sees the first apply failure, not a silent partial success. This is
// the behavior the top-level init banner relies on for error
// surfacing.
func TestApplyTopologyInstances_FailsFastOnCoreError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"fabricated validation failure"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	err := applyTopologyInstances(context.Background(), srv.URL, "test-token")
	if err == nil {
		t.Fatal("expected error when core rejects the apply")
	}
	if !strings.Contains(err.Error(), "yggdrasil-core-kubernetes") {
		t.Errorf("error does not mention the failing instance name: %v", err)
	}
}
