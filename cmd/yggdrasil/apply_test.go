package main

import (
	"strings"
	"testing"
)

func TestSplitYAMLDocuments_StripsSeparators(t *testing.T) {
	raw := []byte(`---
kind: workflow
metadata: {name: a, namespace: x}
spec: {}
---
kind: workflow
metadata: {name: b, namespace: x}
spec: {}
`)
	docs, err := splitYAMLDocuments(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 {
		t.Fatalf("got %d docs, want 2", len(docs))
	}
	for i, d := range docs {
		if strings.HasPrefix(string(d), "---") {
			t.Errorf("doc %d still starts with '---': %q", i, string(d))
		}
	}
}

func TestSplitYAMLDocuments_IgnoresEmptyTrailing(t *testing.T) {
	raw := []byte(`kind: workflow
metadata: {name: only, namespace: x}
spec: {}
---
`)
	docs, err := splitYAMLDocuments(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("got %d docs, want 1 (trailing --- should be ignored)", len(docs))
	}
}

func TestExtractKindAndPayload_RejectsMissingFields(t *testing.T) {
	cases := map[string]string{
		"missing kind":     `metadata: {name: x, namespace: y}` + "\n" + `spec: {}`,
		"missing metadata": `kind: workflow` + "\n" + `spec: {}`,
		"missing spec":     `kind: workflow` + "\n" + `metadata: {name: x, namespace: y}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := extractKindAndPayload([]byte(body))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestExtractKindAndPayload_NormalizesKindToLowercase(t *testing.T) {
	body := `kind: Workflow
metadata:
  name: my-flow
  namespace: prod
spec:
  trigger: {mode: manual}
`
	kind, payload, err := extractKindAndPayload([]byte(body))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if kind != "workflow" {
		t.Errorf("kind = %q, want workflow", kind)
	}
	if payload["name"] != "my-flow" || payload["namespace"] != "prod" {
		t.Errorf("metadata not extracted: %+v", payload)
	}
	if payload["spec"] == nil {
		t.Error("spec dropped")
	}
}
