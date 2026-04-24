// Package scaffoldcli implements `yggdrasil new integration <name>` and
// `yggdrasil new surface <name>` — one-command scaffolding of a fresh
// Yggdrasil plugin from the official templates.
//
// The flow mirrors `npx @backstage/create-app` but in Go: fetch the
// template repository (shallow clone), strip its git history, rename
// every reference to the template's module path / project name, init a
// new git repo, and print the next steps. Adopters go from "I have an
// idea" to a compilable, testable, publishable skeleton in one call.
package scaffoldcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Kind enumerates the scaffold types. Kept small on purpose — anything
// new (e.g. workflow templates) goes here explicitly so the CLI surface
// stays opinionated.
type Kind string

const (
	KindIntegration Kind = "integration"
	KindSurface     Kind = "surface"
)

// Options is the full input to Run. Sensible defaults are filled when
// callers leave fields empty — Name is the only required input.
type Options struct {
	// Kind is integration or surface.
	Kind Kind

	// Name is the bare plugin name (without the `integration-` or
	// `surface-` prefix). Lowercase kebab-case.
	Name string

	// Module is the Go module path for the new plugin. When empty we
	// derive `github.com/<adopter>/<kind>-<name>` from GitHubOwner.
	Module string

	// GitHubOwner is the target GitHub owner (personal or org) used
	// when Module is empty. Also used to write a sane install hint at
	// the end of the scaffold.
	GitHubOwner string

	// Dir is where the new directory lands. Defaults to `./<kind>-<name>`
	// relative to cwd.
	Dir string

	// TemplateRepo overrides the upstream template (owner/repo@ref).
	// Default "dakasa-yggdrasil/integration-template" or
	// "dakasa-yggdrasil/surface-template" based on Kind.
	TemplateRepo string

	// SkipGitInit skips `git init` in the scaffolded directory. Useful
	// when the caller is going to `git clone` into an existing place.
	SkipGitInit bool
}

// Result carries the final paths + derived values so the CLI can
// print a tailored success banner.
type Result struct {
	Dir         string
	Module      string
	ProjectName string
	InstallHint string
}

// Run executes the scaffold. Non-fatal errors (e.g. missing git) bubble
// up with actionable messages so the operator knows what to install.
func Run(ctx context.Context, opts Options) (Result, error) {
	if err := validate(&opts); err != nil {
		return Result{}, err
	}
	defaults(&opts)

	projectName := fmt.Sprintf("%s-%s", opts.Kind, opts.Name)

	result := Result{
		Dir:         opts.Dir,
		Module:      opts.Module,
		ProjectName: projectName,
		InstallHint: installHint(opts.Kind, opts.GitHubOwner, projectName),
	}

	if err := ensureGitAvailable(); err != nil {
		return result, err
	}

	absDir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return result, fmt.Errorf("resolve target dir: %w", err)
	}
	if _, err := os.Stat(absDir); err == nil {
		return result, fmt.Errorf("directory already exists: %s", absDir)
	}
	result.Dir = absDir

	if err := cloneTemplate(ctx, opts.TemplateRepo, absDir); err != nil {
		return result, err
	}

	// Strip the template's git history — we want a fresh repo.
	if err := os.RemoveAll(filepath.Join(absDir, ".git")); err != nil {
		return result, fmt.Errorf("remove template .git: %w", err)
	}

	rewrites := buildRewrites(opts.Kind, opts.Name, opts.Module)
	if err := applyRewrites(absDir, rewrites); err != nil {
		return result, err
	}

	if !opts.SkipGitInit {
		if err := runIn(ctx, absDir, "git", "init", "-q"); err != nil {
			return result, fmt.Errorf("git init: %w", err)
		}
		if err := runIn(ctx, absDir, "git", "add", "-A"); err != nil {
			return result, fmt.Errorf("git add: %w", err)
		}
	}

	return result, nil
}

func validate(opts *Options) error {
	switch opts.Kind {
	case KindIntegration, KindSurface:
	default:
		return fmt.Errorf("unsupported kind %q (must be integration or surface)", opts.Kind)
	}
	name := strings.ToLower(strings.TrimSpace(opts.Name))
	if !slugPattern.MatchString(name) {
		return fmt.Errorf("name %q is invalid: must be lowercase kebab-case (letters, digits, hyphens, starting with a letter)", opts.Name)
	}
	opts.Name = name
	if opts.Module != "" {
		opts.Module = strings.TrimSpace(opts.Module)
	}
	opts.GitHubOwner = strings.TrimSpace(opts.GitHubOwner)
	return nil
}

func defaults(opts *Options) {
	projectName := fmt.Sprintf("%s-%s", opts.Kind, opts.Name)
	if opts.Dir == "" {
		opts.Dir = "./" + projectName
	}
	if opts.TemplateRepo == "" {
		switch opts.Kind {
		case KindIntegration:
			opts.TemplateRepo = "dakasa-yggdrasil/integration-template"
		case KindSurface:
			opts.TemplateRepo = "dakasa-yggdrasil/surface-template"
		}
	}
	if opts.Module == "" {
		owner := opts.GitHubOwner
		if owner == "" {
			owner = "your-org"
		}
		opts.Module = fmt.Sprintf("github.com/%s/%s", owner, projectName)
	}
}

// slugPattern is stricter than the Yggdrasil integration name pattern
// on purpose — scaffolding a plugin named `42` is a bad idea.
var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

func ensureGitAvailable() error {
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git is required for `yggdrasil new`; install it and re-run")
	}
	return nil
}

// cloneTemplate does a shallow clone of the upstream template into dst.
// Shallow is deliberate: we only need the tree at HEAD, not years of
// template history the adopter's new repo should not inherit.
func cloneTemplate(ctx context.Context, repoRef, dst string) error {
	url := parseRepoURL(repoRef)
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--quiet", url, dst)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", url, err)
	}
	return nil
}

func parseRepoURL(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "git@") {
		return ref
	}
	// Accept "owner/repo" or "owner/repo@ref"; we ignore the ref (shallow)
	// since the caller almost always wants HEAD of the default branch.
	if at := strings.IndexByte(ref, '@'); at >= 0 {
		ref = ref[:at]
	}
	return "https://github.com/" + ref + ".git"
}

// buildRewrites returns the ordered list of find/replace pairs applied
// to every text file in the scaffold. Ordering matters — longer, more
// specific patterns run first so they don't get partially replaced by
// shorter ones.
func buildRewrites(kind Kind, name, module string) []rewrite {
	templateProject := fmt.Sprintf("%s-template", kind)
	newProject := fmt.Sprintf("%s-%s", kind, name)

	templateModule := fmt.Sprintf("github.com/dakasa-yggdrasil/%s", templateProject)

	return []rewrite{
		{templateModule, module},
		{templateProject, newProject},
	}
}

type rewrite struct {
	from string
	to   string
}

// applyRewrites walks the tree and rewrites every text file that
// contains the template references. Binary files are skipped — the
// heuristic is "contains a NUL byte in the first 8 KiB".
func applyRewrites(root string, rewrites []rewrite) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			// The .git directory was already removed; skip any vendor
			// or node_modules trees for speed.
			name := entry.Name()
			if name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		return rewriteFile(path, rewrites)
	})
}

func rewriteFile(path string, rewrites []rewrite) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if isBinary(raw) {
		return nil
	}
	content := string(raw)
	changed := false
	for _, r := range rewrites {
		if strings.Contains(content, r.from) {
			content = strings.ReplaceAll(content, r.from, r.to)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	info, _ := os.Stat(path)
	mode := os.FileMode(0o644)
	if info != nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func isBinary(raw []byte) bool {
	limit := len(raw)
	if limit > 8192 {
		limit = 8192
	}
	for i := 0; i < limit; i++ {
		if raw[i] == 0 {
			return true
		}
	}
	return false
}

func runIn(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func installHint(kind Kind, owner, project string) string {
	if owner == "" {
		owner = "your-org"
	}
	return fmt.Sprintf("yggdrasil install %s/%s", owner, project)
}
