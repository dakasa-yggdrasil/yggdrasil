package scaffoldcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidate_AcceptsKebabCaseName(t *testing.T) {
	cases := []struct {
		name  string
		ok    bool
	}{
		{"my-thing", true},
		{"thing", true},
		{"acme-k8s-helper", true},
		{"My-Thing", true},          // gets lowercased
		{"42-thing", false},         // must start with letter
		{"", false},
		{"ends-with-hyphen-", false},
		{"with spaces", false},
		{"with_underscore", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := Options{Kind: KindIntegration, Name: tc.name}
			err := validate(&opts)
			gotOk := err == nil
			if gotOk != tc.ok {
				t.Errorf("validate(%q) err=%v, want ok=%v", tc.name, err, tc.ok)
			}
		})
	}
}

func TestValidate_RejectsUnknownKind(t *testing.T) {
	opts := Options{Kind: Kind("workflow"), Name: "ok"}
	if err := validate(&opts); err == nil {
		t.Fatal("expected error for unsupported kind")
	}
}

func TestDefaults_DerivesModuleFromOwner(t *testing.T) {
	opts := Options{Kind: KindIntegration, Name: "my-thing", GitHubOwner: "acme"}
	defaults(&opts)
	want := "github.com/acme/integration-my-thing"
	if opts.Module != want {
		t.Errorf("module = %q, want %q", opts.Module, want)
	}
	if opts.Dir != "./integration-my-thing" {
		t.Errorf("dir = %q", opts.Dir)
	}
	if opts.TemplateRepo != "dakasa-yggdrasil/integration-template" {
		t.Errorf("template = %q", opts.TemplateRepo)
	}
}

func TestDefaults_SurfaceTemplate(t *testing.T) {
	opts := Options{Kind: KindSurface, Name: "cool-console", GitHubOwner: "acme"}
	defaults(&opts)
	if opts.TemplateRepo != "dakasa-yggdrasil/surface-template" {
		t.Errorf("template = %q, want surface template", opts.TemplateRepo)
	}
	if !strings.HasPrefix(opts.Module, "github.com/acme/surface-") {
		t.Errorf("module = %q, want surface module", opts.Module)
	}
}

func TestParseRepoURL(t *testing.T) {
	cases := map[string]string{
		"acme/tool":                        "https://github.com/acme/tool.git",
		"acme/tool@v1":                     "https://github.com/acme/tool.git",
		"https://github.com/acme/tool.git": "https://github.com/acme/tool.git",
		"git@github.com:acme/tool.git":     "git@github.com:acme/tool.git",
	}
	for in, want := range cases {
		if got := parseRepoURL(in); got != want {
			t.Errorf("parseRepoURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRewriteFile_ReplacesOnlyWhenPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	content := `package main
// import github.com/dakasa-yggdrasil/integration-template/controllers/message
const project = "integration-template"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	rewrites := buildRewrites(KindIntegration, "my-thing", "github.com/acme/integration-my-thing")
	if err := rewriteFile(path, rewrites); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	gotStr := string(got)
	if strings.Contains(gotStr, "integration-template") {
		t.Errorf("template references survived: %s", gotStr)
	}
	if !strings.Contains(gotStr, "integration-my-thing") {
		t.Errorf("new name missing: %s", gotStr)
	}
	if !strings.Contains(gotStr, "github.com/acme/integration-my-thing") {
		t.Errorf("new module path missing: %s", gotStr)
	}
}

func TestRewriteFile_SkipsBinary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.bin")
	content := append([]byte("dakasa-yggdrasil/integration-template"), 0x00, 0x01, 0x02)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	rewrites := buildRewrites(KindIntegration, "x", "github.com/o/integration-x")
	if err := rewriteFile(path, rewrites); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("binary file was modified; scaffold should skip it")
	}
}

// TestRun_RejectsExistingDir guards the contract that we never overwrite
// an existing directory — the adopter's work is precious, and scaffold
// should be one-shot explicit.
func TestRun_RejectsExistingDir(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "integration-already-here")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), Options{
		Kind: KindIntegration,
		Name: "already-here",
		Dir:  existing,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %v, want contains 'already exists'", err)
	}
}

func TestInstallHint_UsesOwner(t *testing.T) {
	hint := installHint(KindIntegration, "acme", "integration-my-thing")
	want := "yggdrasil install acme/integration-my-thing"
	if hint != want {
		t.Errorf("hint = %q, want %q", hint, want)
	}
}
