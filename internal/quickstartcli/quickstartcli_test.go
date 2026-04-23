package quickstartcli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRepoRefVariants(t *testing.T) {
	cases := []struct {
		in      string
		want    RepoRef
		wantErr bool
	}{
		{
			in:   "owner/repo",
			want: RepoRef{Owner: "owner", Repo: "repo", Ref: "main", Path: DefaultManifestPath},
		},
		{
			in:   "owner/repo@v1.2.3",
			want: RepoRef{Owner: "owner", Repo: "repo", Ref: "v1.2.3", Path: DefaultManifestPath},
		},
		{
			in:   "owner/repo:custom/path.yaml",
			want: RepoRef{Owner: "owner", Repo: "repo", Ref: "main", Path: "custom/path.yaml"},
		},
		{
			in:   "owner/repo@v1:custom/path.yaml",
			want: RepoRef{Owner: "owner", Repo: "repo", Ref: "v1", Path: "custom/path.yaml"},
		},
		{in: "", wantErr: true},
		{in: "missingslash", wantErr: true},
	}
	for _, tc := range cases {
		got, err := ParseRepoRef(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseRepoRef(%q): expected error, got %+v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRepoRef(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseRepoRef(%q): got %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestRepoRefStringRoundTrip(t *testing.T) {
	cases := []string{
		"owner/repo",
		"owner/repo@v1.2.3",
		"owner/repo:custom/path.yaml",
		"owner/repo@v1:custom/path.yaml",
	}
	for _, in := range cases {
		ref, err := ParseRepoRef(in)
		if err != nil {
			t.Fatalf("ParseRepoRef(%q): %v", in, err)
		}
		if ref.String() != in {
			t.Errorf("RoundTrip(%q): got %q", in, ref.String())
		}
	}
}

func TestDecodeManifestRejectsWrongKind(t *testing.T) {
	yamlBytes := []byte(`apiVersion: yggdrasil.io/v1alpha1
kind: workflow
metadata: { name: x }
spec: {}
`)
	if _, err := DecodeManifest(yamlBytes); err == nil {
		t.Fatal("expected wrong kind to fail")
	}
}

func TestDecodeManifestAcceptsYAMLAndJSON(t *testing.T) {
	cases := [][]byte{
		[]byte(`apiVersion: yggdrasil.io/v1alpha1
kind: integration_quickstart
metadata: { name: x, namespace: global }
spec:
  providers:
    - id: only
      steps: [{id: s, uses: {kind: integration}}]
`),
		[]byte(`{"apiVersion":"yggdrasil.io/v1alpha1","kind":"integration_quickstart","metadata":{"name":"x","namespace":"global"},"spec":{"providers":[{"id":"only","steps":[{"id":"s","uses":{"kind":"integration"}}]}]}}`),
	}
	for i, raw := range cases {
		doc, err := DecodeManifest(raw)
		if err != nil {
			t.Fatalf("case %d: DecodeManifest: %v", i, err)
		}
		if doc.Spec.Providers[0].ID != "only" {
			t.Fatalf("case %d: unexpected provider id %q", i, doc.Spec.Providers[0].ID)
		}
	}
}

func TestPickProviderUsesPreselectedAndFallsBackToSingle(t *testing.T) {
	spec := QuickstartSpec{
		Providers: []QuickstartProvider{{ID: "only-one"}},
	}
	got, err := PickProvider(spec, "")
	if err != nil {
		t.Fatalf("PickProvider single: %v", err)
	}
	if got.ID != "only-one" {
		t.Fatalf("expected single provider, got %q", got.ID)
	}

	multi := QuickstartSpec{Providers: []QuickstartProvider{{ID: "a"}, {ID: "b"}}}
	got, err = PickProvider(multi, "b")
	if err != nil {
		t.Fatalf("PickProvider preselected: %v", err)
	}
	if got.ID != "b" {
		t.Fatalf("expected b, got %q", got.ID)
	}
}

func TestCollectInputsHeadlessHappyPath(t *testing.T) {
	provider := QuickstartProvider{
		Inputs: []QuickstartInput{
			{ID: "region", Label: "Region", Type: "string", Required: true,
				Validate: &QuickstartValidate{Regex: `^[a-z]{2}-[a-z]+-\d$`}},
			{ID: "tier", Label: "Tier", Type: "select", Default: "standard",
				Choices: []QuickstartChoice{{Value: "standard"}, {Value: "advanced"}}},
			{ID: "size", Label: "Size", Type: "integer", Required: true},
			{ID: "enabled", Label: "Enabled", Type: "boolean", Default: true},
		},
	}
	out, err := CollectInputsHeadless(provider, map[string]string{
		"region": "us-east-1",
		"size":   "20",
	})
	if err != nil {
		t.Fatalf("CollectInputsHeadless: %v", err)
	}
	if out["region"] != "us-east-1" {
		t.Fatalf("region: %v", out["region"])
	}
	if out["size"].(int) != 20 {
		t.Fatalf("size: %v", out["size"])
	}
	if out["tier"] != "standard" {
		t.Fatalf("expected tier default 'standard', got %v", out["tier"])
	}
	if out["enabled"] != true {
		t.Fatalf("expected enabled default true, got %v", out["enabled"])
	}
}

func TestCollectInputsHeadlessReportsAllErrors(t *testing.T) {
	provider := QuickstartProvider{
		Inputs: []QuickstartInput{
			{ID: "region", Label: "Region", Type: "string", Required: true,
				Validate: &QuickstartValidate{Regex: `^[a-z]{2}-[a-z]+-\d$`, Message: "must look like us-east-1"}},
			{ID: "size", Label: "Size", Type: "integer", Required: true},
		},
	}
	_, err := CollectInputsHeadless(provider, map[string]string{
		"region":  "BAD",
		"unknown": "value",
	})
	if err == nil {
		t.Fatal("expected aggregated validation error")
	}
	msg := err.Error()
	for _, want := range []string{"region", "size", "unknown"} {
		if !contains(msg, want) {
			t.Errorf("expected error to mention %q, got: %s", want, msg)
		}
	}
}

func TestDecodeManifestAgainstPOC(t *testing.T) {
	candidate := filepath.Join("..", "..", "..", "integration-secrets-management", "yggdrasil-quickstart.yaml")
	raw, err := os.ReadFile(candidate)
	if err != nil {
		t.Skipf("POC manifest not available at %s: %v", candidate, err)
	}
	doc, err := DecodeManifest(raw)
	if err != nil {
		t.Fatalf("DecodeManifest POC: %v", err)
	}
	if len(doc.Spec.Providers) != 2 {
		t.Fatalf("expected 2 providers in POC, got %d", len(doc.Spec.Providers))
	}
	for _, p := range doc.Spec.Providers {
		if len(p.Inputs) == 0 {
			t.Errorf("provider %s has no inputs", p.ID)
		}
		if p.SmokeTestExpected() && len(p.PostInstallHints) == 0 {
			t.Errorf("provider %s has no post_install_hints", p.ID)
		}
	}
}

// SmokeTestExpected returns true unconditionally — the POC defines
// smoke_test on all providers so the test would catch a regression where
// the field is silently dropped during decoding. (We don't model SmokeTest
// in the CLI struct on purpose; the server owns it.)
func (p QuickstartProvider) SmokeTestExpected() bool { return true }

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
