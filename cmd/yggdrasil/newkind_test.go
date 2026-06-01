package main

import (
	"strings"
	"testing"
)

func TestIsManifestTemplateKind(t *testing.T) {
	if !isManifestTemplateKind("workflow") {
		t.Error("workflow should be a manifest-template kind")
	}
	if isManifestTemplateKind("integration") {
		t.Error("integration is a repo-scaffold kind, not a manifest template")
	}
}

func TestRenderManifestTemplate(t *testing.T) {
	out, err := renderManifestTemplate("workflow", "hello", "prod")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"kind: workflow", "name: hello", "namespace: prod"} {
		if !strings.Contains(out, want) {
			t.Errorf("template missing %q:\n%s", want, out)
		}
	}
}

func TestRenderManifestTemplate_UnknownKind(t *testing.T) {
	if _, err := renderManifestTemplate("nope", "x", "y"); err == nil {
		t.Error("expected error for unknown kind")
	}
}
