package initcli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// initStackEnv is the YGGDRASIL_ENV value the init stack declares. Core
// allows credential-free machine calls (workflow dispatch, event publishes,
// deploy, bootstrap and integration install) only while YGGDRASIL_ENV names
// a development environment explicitly (yggdrasil-core ADR-0022), so a stack
// that loses this value answers 401 to those calls once it runs such a Core.
const initStackEnv = "development"

// coreDevelopmentEnvs mirrors the values Core's machineAnonymousAllowed
// accepts (trimmed and lowercased).
var coreDevelopmentEnvs = map[string]bool{
	"dev":         true,
	"development": true,
	"local":       true,
	"test":        true,
}

// composeInterpolation matches the whole-value shapes docker compose
// interpolates: ${NAME}, ${NAME-default} and ${NAME:-default}.
var composeInterpolation = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?-)(.*))?\}$`)

// resolveComposeValue resolves one compose environment value against the
// variables compose reads from .env, with compose's semantics: ":-" falls
// back when the variable is unset or empty, "-" only when it is unset.
func resolveComposeValue(t *testing.T, raw string, dotenv map[string]string) string {
	t.Helper()
	m := composeInterpolation.FindStringSubmatch(raw)
	if m == nil {
		if strings.Contains(raw, "$") {
			t.Fatalf("unsupported compose interpolation %q; extend resolveComposeValue before changing the asset", raw)
		}
		return raw
	}
	value, present := dotenv[m[1]]
	switch m[2] {
	case ":-":
		if !present || value == "" {
			return m[3]
		}
	case "-":
		if !present {
			return m[3]
		}
	}
	return value
}

// renderInitAssets writes the compose file and .env exactly as `yggdrasil
// init` does for default options and returns the directory.
func renderInitAssets(t *testing.T) string {
	t.Helper()
	opts := Options{}
	defaults(&opts)
	opts.AdminPassword = "test-admin-password"
	dir := t.TempDir()
	if err := writeAssets(dir, opts); err != nil {
		t.Fatalf("writeAssets: %v", err)
	}
	return dir
}

// coreComposeEnv returns the yggdrasil-core service environment from the
// written compose file, as raw (uninterpolated) strings.
func coreComposeEnv(t *testing.T, dir string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}
	var compose struct {
		Services map[string]struct {
			Environment map[string]any `json:"environment"`
		} `json:"services"`
	}
	if err := yaml.Unmarshal(raw, &compose); err != nil {
		t.Fatalf("parse docker-compose.yml: %v", err)
	}
	core, ok := compose.Services["yggdrasil-core"]
	if !ok {
		t.Fatal("docker-compose.yml has no yggdrasil-core service")
	}
	env := make(map[string]string, len(core.Environment))
	for key, value := range core.Environment {
		s, ok := value.(string)
		if !ok {
			t.Fatalf("yggdrasil-core environment %s = %v (%T); quote it so compose passes a string", key, value, value)
		}
		env[key] = s
	}
	return env
}

// dotenvEntries parses the written .env the way compose does for plain
// KEY=value lines, failing on a key declared twice.
func dotenvEntries(t *testing.T, dir string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	entries := map[string]string{}
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf(".env line %d is not KEY=value: %q", i+1, line)
		}
		key = strings.TrimSpace(key)
		if _, dup := entries[key]; dup {
			t.Fatalf(".env declares %s twice", key)
		}
		entries[key] = strings.TrimSpace(value)
	}
	return entries
}

func TestInitStackDeclaresExplicitDevelopmentEnv(t *testing.T) {
	dir := renderInitAssets(t)
	coreEnv := coreComposeEnv(t, dir)
	dotenv := dotenvEntries(t, dir)

	raw, ok := coreEnv["YGGDRASIL_ENV"]
	if !ok {
		t.Fatal("yggdrasil-core environment does not declare YGGDRASIL_ENV; Core would treat the init stack as not development")
	}

	if got := dotenv["YGGDRASIL_ENV"]; got != initStackEnv {
		t.Fatalf(".env YGGDRASIL_ENV = %q, want %q", got, initStackEnv)
	}

	cases := []struct {
		name   string
		dotenv map[string]string
	}{
		{name: "rendered .env", dotenv: dotenv},
		{name: ".env written before the key existed", dotenv: map[string]string{}},
		{name: "blank .env value", dotenv: map[string]string{"YGGDRASIL_ENV": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveComposeValue(t, raw, tc.dotenv)
			if got != initStackEnv {
				t.Fatalf("yggdrasil-core YGGDRASIL_ENV resolves to %q, want %q", got, initStackEnv)
			}
			if !coreDevelopmentEnvs[strings.ToLower(strings.TrimSpace(got))] {
				t.Fatalf("yggdrasil-core YGGDRASIL_ENV %q is not a value Core treats as development", got)
			}
		})
	}
}

// TestInitStackDotenvDrivesCoreEnv proves the .env entry is the knob and
// not dead configuration: compose must read YGGDRASIL_ENV from .env.
func TestInitStackDotenvDrivesCoreEnv(t *testing.T) {
	dir := renderInitAssets(t)
	raw := coreComposeEnv(t, dir)["YGGDRASIL_ENV"]
	got := resolveComposeValue(t, raw, map[string]string{"YGGDRASIL_ENV": "local"})
	if got != "local" {
		t.Fatalf("yggdrasil-core YGGDRASIL_ENV ignores the .env value: got %q, want %q", got, "local")
	}
}

// TestInitStackForwardsDeployToken proves YGGDRASIL_DEPLOY_TOKEN is an
// operator knob in .env: compose forwards it to yggdrasil-core, and the
// rendered .env leaves it empty so the default stack behaves as before.
func TestInitStackForwardsDeployToken(t *testing.T) {
	dir := renderInitAssets(t)
	raw, ok := coreComposeEnv(t, dir)["YGGDRASIL_DEPLOY_TOKEN"]
	if !ok {
		t.Fatal("yggdrasil-core environment does not forward YGGDRASIL_DEPLOY_TOKEN; the .env knob would never reach Core")
	}
	if got := resolveComposeValue(t, raw, dotenvEntries(t, dir)); got != "" {
		t.Fatalf("rendered stack sets YGGDRASIL_DEPLOY_TOKEN to %q, want it empty by default", got)
	}
	if got := resolveComposeValue(t, raw, map[string]string{"YGGDRASIL_DEPLOY_TOKEN": "operator-value"}); got != "operator-value" {
		t.Fatalf("yggdrasil-core YGGDRASIL_DEPLOY_TOKEN ignores the .env value: got %q", got)
	}
}

func TestResolveComposeValue(t *testing.T) {
	cases := []struct {
		raw    string
		dotenv map[string]string
		want   string
	}{
		{raw: "plain", want: "plain"},
		{raw: "${A}", dotenv: map[string]string{"A": "x"}, want: "x"},
		{raw: "${A}", want: ""},
		{raw: "${A:-d}", want: "d"},
		{raw: "${A:-d}", dotenv: map[string]string{"A": ""}, want: "d"},
		{raw: "${A:-d}", dotenv: map[string]string{"A": "x"}, want: "x"},
		{raw: "${A-d}", want: "d"},
		{raw: "${A-d}", dotenv: map[string]string{"A": ""}, want: ""},
	}
	for _, tc := range cases {
		if got := resolveComposeValue(t, tc.raw, tc.dotenv); got != tc.want {
			t.Errorf("resolveComposeValue(%q, %v) = %q, want %q", tc.raw, tc.dotenv, got, tc.want)
		}
	}
}
