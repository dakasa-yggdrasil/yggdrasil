// Package initcli implements the `yggdrasil init` command: bring up a
// self-hosted yggdrasil-core stack with one command, from empty host to
// a running control plane with a usable admin login.
//
// Two modes:
//
//   - Standalone (default): write docker-compose.yml + .env into a
//     target directory, run `docker compose up -d`, poll /readyz, log
//     in, persist a CLI context.
//
//   - Existing cluster (`--server <url>`): skip the compose step and
//     only run the login + context persistence half, for adopters who
//     already brought up yggdrasil-core through helm or kustomize.
package initcli

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

//go:embed assets/docker-compose.yml assets/env.template
var assets embed.FS

// DefaultCoreImage is the image tag baked into the compose file when
// the operator does not override YGGDRASIL_CORE_IMAGE. Pinned to a
// concrete tag because `latest` makes upgrades surprising.
const DefaultCoreImage = "ghcr.io/dakasa-yggdrasil/yggdrasil-core:latest"

// Options is everything an operator can tweak when calling Run.
// Zero values pick safe defaults (random passwords, ./yggdrasil dir).
type Options struct {
	// Dir is where the compose file + .env land. Relative paths are
	// resolved against the process cwd. Defaults to "./yggdrasil".
	Dir string

	// Server, when set, skips docker-compose entirely and only performs
	// the login + context persistence half against that yggdrasil-core.
	Server string

	// AdminUsername and AdminPassword control the first-run bootstrap.
	// Password defaults to a random 24-char value printed at the end.
	AdminUsername    string
	AdminPassword    string
	AdminEmail       string
	AdminDisplayName string

	// CoreImage overrides the yggdrasil-core container image. Empty
	// means DefaultCoreImage.
	CoreImage string

	// HTTPPort is the host port the container stack binds to. 9080
	// default matches the server's advertised port.
	HTTPPort int

	// ContextName is how this deployment is recorded in ~/.yggdrasil/
	// config.yaml. Defaults to "local" for standalone mode, or the
	// host from --server when in existing-cluster mode.
	ContextName string

	// Yes skips the final confirmation prompt (implied by noninteractive
	// CI runs that pipe the command).
	Yes bool
}

// Result is what Run hands back so the caller can print a tailored
// success banner. All fields are populated on success; some may be
// empty when running in existing-cluster mode.
type Result struct {
	Dir              string
	Server           string
	AdminUsername    string
	AdminPassword    string
	AdminPasswordGenerated bool
	Token            string
	ContextName      string
	ConfigPath       string
	SkippedCompose   bool
}

// Run orchestrates the full init flow. Every step is idempotent
// enough that re-running on a half-finished directory finishes the
// remaining work (except docker compose up, which is always safe).
func Run(ctx context.Context, opts Options) (Result, error) {
	defaults(&opts)

	result := Result{
		Server:        opts.Server,
		AdminUsername: opts.AdminUsername,
		ContextName:   opts.ContextName,
	}

	if opts.AdminPassword == "" {
		pw, err := randomPassword(24)
		if err != nil {
			return result, fmt.Errorf("generate admin password: %w", err)
		}
		opts.AdminPassword = pw
		result.AdminPasswordGenerated = true
	}
	result.AdminPassword = opts.AdminPassword

	if opts.Server != "" {
		result.SkippedCompose = true
		return finalizeExistingCluster(ctx, opts, result)
	}

	if err := ensureDockerAvailable(); err != nil {
		return result, err
	}

	resolvedDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return result, fmt.Errorf("resolve directory: %w", err)
	}
	if err := os.MkdirAll(resolvedDir, 0o755); err != nil {
		return result, fmt.Errorf("create %s: %w", resolvedDir, err)
	}
	result.Dir = resolvedDir

	if err := writeAssets(resolvedDir, opts); err != nil {
		return result, err
	}

	if err := composeUp(ctx, resolvedDir); err != nil {
		return result, err
	}

	serverURL := fmt.Sprintf("http://localhost:%d", opts.HTTPPort)
	result.Server = serverURL

	if err := waitForReady(ctx, serverURL); err != nil {
		return result, fmt.Errorf("yggdrasil-core did not become ready: %w", err)
	}

	result, err = finalizeLoginAndConfig(ctx, opts, result)
	if err != nil {
		return result, err
	}

	// Seed the topology-specific integration_instance manifests that
	// point at the adapter services baked into the standalone compose.
	// These cannot live in yggdrasil-core's shipped seeds (URLs are
	// deployment-specific) but are essential for workflows like
	// yggdrasil-deploy-control-plane to find an adapter to dispatch
	// to. Standalone-mode only — existing-cluster deployments bring
	// their own instances.
	if err := applyTopologyInstances(ctx, result.Server, result.Token); err != nil {
		return result, fmt.Errorf("register adapter instances: %w", err)
	}

	return result, nil
}

func finalizeExistingCluster(ctx context.Context, opts Options, result Result) (Result, error) {
	if err := waitForReady(ctx, opts.Server); err != nil {
		return result, fmt.Errorf("server %s is not reachable: %w", opts.Server, err)
	}
	return finalizeLoginAndConfig(ctx, opts, result)
}

func finalizeLoginAndConfig(ctx context.Context, opts Options, result Result) (Result, error) {
	client := corecli.NewClient(result.Server, "")
	token, _, err := client.Login(ctx, opts.AdminUsername, opts.AdminPassword)
	if err != nil {
		return result, fmt.Errorf("log in as %s: %w", opts.AdminUsername, err)
	}
	result.Token = token

	cfg, path, err := corecli.Load()
	if err != nil {
		return result, err
	}
	cfg.SetContext(result.ContextName, &corecli.Context{
		Server:       result.Server,
		Token:        result.Token,
		Collaborator: opts.AdminUsername,
	}, true)
	if err := corecli.Save(cfg); err != nil {
		return result, err
	}
	result.ConfigPath = path
	return result, nil
}

func defaults(opts *Options) {
	if opts.Dir == "" {
		opts.Dir = "./yggdrasil"
	}
	if opts.AdminUsername == "" {
		opts.AdminUsername = "admin"
	}
	if opts.AdminDisplayName == "" {
		opts.AdminDisplayName = "Admin"
	}
	if opts.CoreImage == "" {
		opts.CoreImage = DefaultCoreImage
	}
	if opts.HTTPPort == 0 {
		opts.HTTPPort = 9080
	}
	if opts.ContextName == "" {
		opts.ContextName = "local"
		if opts.Server != "" {
			opts.ContextName = contextNameFromServer(opts.Server)
		}
	}
}

// contextNameFromServer derives a sensible context name from the
// server URL (host stripped of scheme and ports). Used in
// existing-cluster mode to avoid clobbering "local" when someone
// initializes multiple remotes.
func contextNameFromServer(server string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(server, "https://"), "http://")
	if idx := strings.IndexAny(s, "/:"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.ReplaceAll(s, ".", "-")
	if s == "" {
		s = "remote"
	}
	return s
}

func writeAssets(dir string, opts Options) error {
	composeContent, err := fs.ReadFile(assets, "assets/docker-compose.yml")
	if err != nil {
		return fmt.Errorf("read embedded compose: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), composeContent, 0o644); err != nil {
		return fmt.Errorf("write docker-compose.yml: %w", err)
	}

	envTmplBytes, err := fs.ReadFile(assets, "assets/env.template")
	if err != nil {
		return fmt.Errorf("read embedded env template: %w", err)
	}
	tmpl, err := template.New("env").Parse(string(envTmplBytes))
	if err != nil {
		return fmt.Errorf("parse env template: %w", err)
	}

	pgPass, err := randomPassword(20)
	if err != nil {
		return err
	}
	rmqPass, err := randomPassword(20)
	if err != nil {
		return err
	}

	data := struct {
		GeneratedAt      string
		CoreImage        string
		HTTPPort         int
		PostgresUser     string
		PostgresPassword string
		PostgresDB       string
		RabbitMQUser     string
		RabbitMQPassword string
		AdminUsername    string
		AdminPassword    string
		AdminEmail       string
		AdminDisplayName string
	}{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		CoreImage:        opts.CoreImage,
		HTTPPort:         opts.HTTPPort,
		PostgresUser:     "yggdrasil",
		PostgresPassword: pgPass,
		PostgresDB:       "yggdrasil",
		RabbitMQUser:     "yggdrasil",
		RabbitMQPassword: rmqPass,
		AdminUsername:    opts.AdminUsername,
		AdminPassword:    opts.AdminPassword,
		AdminEmail:       opts.AdminEmail,
		AdminDisplayName: opts.AdminDisplayName,
	}

	envFile, err := os.Create(filepath.Join(dir, ".env"))
	if err != nil {
		return fmt.Errorf("create .env: %w", err)
	}
	defer envFile.Close()
	if err := envFile.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod .env: %w", err)
	}
	if err := tmpl.Execute(envFile, data); err != nil {
		return fmt.Errorf("render .env: %w", err)
	}
	return nil
}

func ensureDockerAvailable() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker is not installed or not in PATH; install Docker Desktop or the docker engine first")
	}
	// `docker compose version` returns non-zero when the compose v2
	// plugin is missing. We rely on compose v2 (space, not dash).
	cmd := exec.Command("docker", "compose", "version")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose v2 plugin is required: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func composeUp(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "up", "-d")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	return nil
}

// waitForReady polls /readyz until the server reports ready or the
// timeout fires. /readyz is preferred over /healthz because it asserts
// the DB is reachable, which is what tells us bootstrap finished.
func waitForReady(ctx context.Context, server string) error {
	client := corecli.NewClient(server, "")
	deadline := time.Now().Add(120 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := client.Readyz(reqCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timed out")
	}
	return lastErr
}

// randomPassword produces an URL-safe random string of the requested
// character length. Base64-URL is chosen so the value is shell-safe in
// .env files without quoting (no '/', no '=' once padding is stripped).
func randomPassword(length int) (string, error) {
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) < length {
		return encoded, nil
	}
	return encoded[:length], nil
}
