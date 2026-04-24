package quickstartcli

import (
	"testing"
)

func TestIsOCIRef(t *testing.T) {
	cases := map[string]bool{
		"oci://ghcr.io/foo/bar":          true,
		"oci://ghcr.io/foo/bar:v1":       true,
		"  oci://ghcr.io/foo/bar  ":      true,
		"dakasa-yggdrasil/integration":   false,
		"oci:ghcr.io/foo/bar":            false,
		"":                               false,
		"https://oci://ghcr.io/foo/bar":  false,
	}
	for input, want := range cases {
		if got := IsOCIRef(input); got != want {
			t.Errorf("IsOCIRef(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseOCIRef(t *testing.T) {
	t.Run("with tag", func(t *testing.T) {
		ref, err := ParseOCIRef("oci://ghcr.io/dakasa-yggdrasil/integration-kubernetes:v1.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref.Registry != "ghcr.io" {
			t.Errorf("Registry = %q, want ghcr.io", ref.Registry)
		}
		if ref.Path != "dakasa-yggdrasil/integration-kubernetes" {
			t.Errorf("Path = %q", ref.Path)
		}
		if ref.Tag != "v1.0.0" {
			t.Errorf("Tag = %q, want v1.0.0", ref.Tag)
		}
	})
	t.Run("default tag", func(t *testing.T) {
		ref, err := ParseOCIRef("oci://ghcr.io/org/thing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref.Tag != "latest" {
			t.Errorf("Tag = %q, want latest (default)", ref.Tag)
		}
	})
	t.Run("registry with port", func(t *testing.T) {
		// A host:port registry must not be confused for a tag — the
		// ":" in the port is before the path's first slash.
		ref, err := ParseOCIRef("oci://registry.local:5000/org/thing:v2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref.Registry != "registry.local:5000" {
			t.Errorf("Registry = %q", ref.Registry)
		}
		if ref.Path != "org/thing" {
			t.Errorf("Path = %q", ref.Path)
		}
		if ref.Tag != "v2" {
			t.Errorf("Tag = %q", ref.Tag)
		}
	})
	t.Run("errors", func(t *testing.T) {
		for _, bad := range []string{
			"",
			"oci://",
			"oci://ghcr.io",
			"oci://ghcr.io/",
			"ghcr.io/foo/bar",
		} {
			if _, err := ParseOCIRef(bad); err == nil {
				t.Errorf("ParseOCIRef(%q) expected error, got nil", bad)
			}
		}
	})
}

func TestOCIRef_String(t *testing.T) {
	// String is canonical — default tag is omitted so operators
	// don't eyeball an extra ":latest" they didn't type.
	ref := OCIRef{Registry: "ghcr.io", Path: "org/thing", Tag: "latest"}
	if got, want := ref.String(), "oci://ghcr.io/org/thing"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	ref.Tag = "v1.0.0"
	if got, want := ref.String(), "oci://ghcr.io/org/thing:v1.0.0"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestParseRepoRef_DispatchesOCI(t *testing.T) {
	// Entry point: the same function CLI callers use must accept
	// both oci:// and owner/repo forms.
	gh, err := ParseRepoRef("dakasa-yggdrasil/integration-kubernetes@v1.0.0")
	if err != nil {
		t.Fatalf("github form: %v", err)
	}
	if gh.OCI != nil {
		t.Error("GitHub ref should not set OCI field")
	}
	if gh.Owner != "dakasa-yggdrasil" || gh.Repo != "integration-kubernetes" {
		t.Errorf("GitHub parse: %+v", gh)
	}

	oci, err := ParseRepoRef("oci://ghcr.io/dakasa-yggdrasil/integration-kubernetes:v1.0.0")
	if err != nil {
		t.Fatalf("oci form: %v", err)
	}
	if oci.OCI == nil {
		t.Fatal("OCI ref should set OCI field")
	}
	if oci.OCI.Registry != "ghcr.io" || oci.OCI.Tag != "v1.0.0" {
		t.Errorf("OCI parse: %+v", *oci.OCI)
	}
}

func TestPickQuickstartLayer(t *testing.T) {
	// Preferred media type wins when present.
	m := ociManifest{Layers: []ociLayer{
		{MediaType: "application/octet-stream", Digest: "sha256:aaa"},
		{MediaType: ociQuickstartMediaTp, Digest: "sha256:bbb"},
	}}
	if d, err := pickQuickstartLayer(m); err != nil || d != "sha256:bbb" {
		t.Errorf("pickQuickstartLayer preferred = %q, err=%v, want sha256:bbb", d, err)
	}

	// Fallback to first layer when no preferred media type.
	m = ociManifest{Layers: []ociLayer{
		{MediaType: "application/yaml", Digest: "sha256:fff"},
	}}
	if d, _ := pickQuickstartLayer(m); d != "sha256:fff" {
		t.Errorf("pickQuickstartLayer fallback = %q, want sha256:fff", d)
	}

	// Empty layers is an error.
	if _, err := pickQuickstartLayer(ociManifest{}); err == nil {
		t.Error("pickQuickstartLayer empty expected error")
	}
}

func TestParseBearerChallenge(t *testing.T) {
	params := parseBearerChallenge(`realm="https://ghcr.io/token",service="ghcr.io",scope="repository:org/thing:pull"`)
	if params["realm"] != "https://ghcr.io/token" {
		t.Errorf("realm = %q", params["realm"])
	}
	if params["service"] != "ghcr.io" {
		t.Errorf("service = %q", params["service"])
	}
}
