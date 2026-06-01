package scaffoldcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A trimmed copy of the template's yggdrasil-quickstart.yaml placeholders,
// AFTER the tree-wide rewrite has already turned integration-template into
// integration-datadog (so only the quickstart-specific stubs remain).
const stubQuickstart = `apiVersion: yggdrasil.io/v1alpha1
kind: integration_quickstart
metadata:
  name: template
spec:
  display_name: "TODO: display name"
  providers:
    - id: default
      display_name: "TODO: provider label"
      inputs:
        - id: image
          default: ghcr.io/your-org/integration-datadog:latest
      steps:
        - id: apply-adapter-deployment
          with:
            manifest:
              spec:
                template:
                  metadata:
                    labels:
                      app: "{{ inputs.instance_name }}"
      smoke_test:
        uses:
          family: template
`

func TestCustomizeQuickstart_ReplacesStubs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "yggdrasil-quickstart.yaml")
	if err := os.WriteFile(path, []byte(stubQuickstart), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := customizeQuickstart(dir, "datadog", "acme-eng"); err != nil {
		t.Fatalf("customizeQuickstart: %v", err)
	}
	got, _ := os.ReadFile(path)
	s := string(got)

	for _, want := range []string{
		"name: datadog",
		"family: datadog",
		"id: datadog",
		"ghcr.io/acme-eng/integration-datadog",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in customized quickstart:\n%s", want, s)
		}
	}
	for _, bad := range []string{"name: template", "family: template", "id: default", "your-org", "TODO"} {
		if strings.Contains(s, bad) {
			t.Errorf("leftover stub %q should be gone:\n%s", bad, s)
		}
	}
	// The K8s Deployment podspec `template:` key MUST be preserved — only
	// the quickstart's own placeholders are rewritten.
	if !strings.Contains(s, "template:\n") {
		t.Errorf("the Deployment podspec template: key must be preserved:\n%s", s)
	}
}

func TestCustomizeQuickstart_NoFileIsNoop(t *testing.T) {
	if err := customizeQuickstart(t.TempDir(), "x", "acme"); err != nil {
		t.Errorf("expected no-op when the file is absent, got %v", err)
	}
}
