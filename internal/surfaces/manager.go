package surfaces

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	templateDirName      = "templates/surface-go"
	defaultSurfacePort   = "9090"
	defaultSurfaceModule = "github.com/dakasa-yggdrasil/%s"
	activeCatalogPath    = "catalog/surfaces.active"
)

type Manager struct {
	root    string
	catalog *Catalog
}

func NewManager(root string) *Manager {
	catalog, err := loadCatalog(root)
	if err != nil {
		return &Manager{root: root}
	}
	return &Manager{root: root, catalog: catalog}
}

func (m *Manager) List() ([]SurfaceStatus, error) {
	active, err := m.loadActive()
	if err != nil {
		return nil, err
	}
	activeSet := map[string]struct{}{}
	for _, item := range active {
		activeSet[item] = struct{}{}
	}

	entries := make([]SurfaceStatus, 0, len(m.catalog.Surfaces))
	for _, surface := range m.catalog.Surfaces {
		path := filepath.Join(m.root, "surfaces", surface.RepoName)
		installed := hasFile(filepath.Join(path, "docker-compose.yml"))
		source := ""
		if installed {
			source = "remote"
			if local := m.localSourcePath(surface); local != "" {
				source = "local"
			}
		}
		_, active := activeSet[surface.Slug]
		entries = append(entries, SurfaceStatus{
			Surface:   surface,
			Installed: installed,
			Active:    active,
			Source:    source,
			Path:      path,
		})
	}

	return entries, nil
}

func (m *Manager) Active() ([]string, error) {
	return m.loadActive()
}

func (m *Manager) ComposeFiles() ([]string, error) {
	active, err := m.loadActive()
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(active))
	for _, slug := range active {
		entry, err := m.find(slug)
		if err != nil {
			continue
		}
		file := filepath.Join(m.root, "surfaces", entry.RepoName, "docker-compose.yml")
		if hasFile(file) {
			files = append(files, file)
		}
	}
	sort.Strings(files)
	return files, nil
}

func (m *Manager) Install(slug string) (Surface, string, error) {
	entry, err := m.find(slug)
	if err != nil {
		return Surface{}, "", err
	}

	rel := filepath.Join("surfaces", entry.RepoName)
	target := filepath.Join(m.root, rel)
	if hasFile(filepath.Join(target, "docker-compose.yml")) {
		return entry, "installed", nil
	}
	if m.isRegisteredSubmodule(rel) {
		if err := run("git", "-C", m.root, "submodule", "update", "--init", "--recursive", rel); err != nil {
			return Surface{}, "", err
		}
		if err := initEnv(filepath.Join(target, ".env.example"), filepath.Join(target, ".env")); err != nil {
			return Surface{}, "", err
		}
		return entry, "registered", nil
	}
	if pathExists(target) {
		return Surface{}, "", fmt.Errorf("target path already exists and is not a managed surface: %s", target)
	}

	source, sourceLabel, err := m.installSource(entry)
	if err != nil {
		return Surface{}, "", err
	}

	args := []string{"-C", m.root}
	if sourceLabel == "local" {
		args = append(args, "-c", "protocol.file.allow=always")
	}
	args = append(args, "submodule", "add", source, rel)
	if err := run("git", args...); err != nil {
		return Surface{}, "", err
	}

	if err := run("git", "-C", m.root, "submodule", "update", "--init", "--recursive", rel); err != nil {
		return Surface{}, "", err
	}

	if err := initEnv(filepath.Join(target, ".env.example"), filepath.Join(target, ".env")); err != nil {
		return Surface{}, "", err
	}

	return entry, sourceLabel, nil
}

func (m *Manager) Remove(slug string) (Surface, error) {
	entry, err := m.find(slug)
	if err != nil {
		return Surface{}, err
	}

	_ = m.Deactivate(slug)

	rel := filepath.Join("surfaces", entry.RepoName)
	target := filepath.Join(m.root, rel)
	if !pathExists(target) {
		return entry, nil
	}

	if err := run("git", "-C", m.root, "submodule", "deinit", "-f", "--", rel); err != nil {
		return Surface{}, err
	}
	if err := run("git", "-C", m.root, "rm", "-f", rel); err != nil {
		return Surface{}, err
	}
	_ = os.RemoveAll(filepath.Join(m.root, ".git", "modules", rel))
	return entry, nil
}

func (m *Manager) Scaffold(name, module string) (string, string, error) {
	slug := normalizeSurfaceName(name)
	if slug == "" {
		return "", "", fmt.Errorf("surface name is required")
	}

	templateRoot := filepath.Join(m.root, templateDirName)
	if info, err := os.Stat(templateRoot); err != nil || !info.IsDir() {
		return "", "", fmt.Errorf("surface template not found at %s", templateRoot)
	}

	target := filepath.Join(m.root, "surfaces", slug)
	if _, err := os.Stat(target); err == nil {
		return "", "", fmt.Errorf("surface already exists at %s", target)
	}

	if strings.TrimSpace(module) == "" {
		module = fmt.Sprintf(defaultSurfaceModule, slug)
	}

	replacements := map[string]string{
		"__SURFACE_NAME__":         slug,
		"__SURFACE_DISPLAY_NAME__": humanizeSurfaceName(slug),
		"__MODULE_PATH__":          strings.TrimSpace(module),
		"__SURFACE_PORT__":         defaultSurfacePort,
	}

	if err := copyTemplateDir(templateRoot, target, replacements); err != nil {
		return "", "", err
	}

	if err := initEnv(filepath.Join(target, ".env.example"), filepath.Join(target, ".env")); err != nil {
		return "", "", err
	}

	return slug, target, nil
}

func (m *Manager) Activate(name string) error {
	entry, err := m.find(name)
	if err != nil {
		return err
	}

	target := filepath.Join(m.root, "surfaces", entry.RepoName)
	if !hasFile(filepath.Join(target, "docker-compose.yml")) {
		return fmt.Errorf("surface %q is not installed at %s", entry.Slug, target)
	}

	active, err := m.loadActive()
	if err != nil {
		return err
	}
	for _, item := range active {
		if item == entry.Slug {
			return nil
		}
	}

	active = append(active, entry.Slug)
	sort.Strings(active)
	return m.writeActive(active)
}

func (m *Manager) Deactivate(name string) error {
	slug := normalizeSurfaceName(name)
	if slug == "" {
		return fmt.Errorf("surface name is required")
	}

	active, err := m.loadActive()
	if err != nil {
		return err
	}

	filtered := make([]string, 0, len(active))
	for _, item := range active {
		if item == slug {
			continue
		}
		filtered = append(filtered, item)
	}
	return m.writeActive(filtered)
}

func (m *Manager) find(slug string) (Surface, error) {
	slug = normalizeSurfaceName(slug)
	if m.catalog == nil {
		return Surface{}, fmt.Errorf("surface catalog is unavailable")
	}
	for _, surface := range m.catalog.Surfaces {
		if surface.Slug == slug {
			return surface, nil
		}
	}
	return Surface{}, fmt.Errorf("surface %q not found in catalog", slug)
}

func (m *Manager) installSource(entry Surface) (string, string, error) {
	if local := m.localSourcePath(entry); local != "" {
		return local, "local", nil
	}
	if entry.RepoURL != "" {
		return entry.RepoURL, "remote", nil
	}
	return "", "", errors.New("no install source available")
}

func (m *Manager) localSourcePath(entry Surface) string {
	if devDir := os.Getenv("YGGDRASIL_SURFACES_DEV_DIR"); devDir != "" {
		path := filepath.Join(devDir, entry.RepoName)
		if isGitRepo(path) {
			return path
		}
	}

	sibling := filepath.Join(filepath.Dir(m.root), entry.RepoName)
	if isGitRepo(sibling) {
		return sibling
	}

	return ""
}

func copyTemplateDir(sourceRoot, targetRoot string, replacements map[string]string) error {
	return filepath.WalkDir(sourceRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(targetRoot, 0o755)
		}

		targetPath := filepath.Join(targetRoot, rel)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		rendered := string(content)
		for from, to := range replacements {
			rendered = strings.ReplaceAll(rendered, from, to)
		}

		mode := fs.FileMode(0o644)
		if info, err := entry.Info(); err == nil {
			mode = info.Mode()
		}

		return os.WriteFile(targetPath, []byte(rendered), mode)
	})
}

func normalizeSurfaceName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func humanizeSurfaceName(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "-")
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func initEnv(examplePath, envPath string) error {
	if !hasFile(examplePath) {
		return nil
	}
	if _, err := os.Stat(envPath); err == nil {
		return nil
	}

	content, err := os.ReadFile(examplePath)
	if err != nil {
		return err
	}
	return os.WriteFile(envPath, content, 0o644)
}

func hasFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && (info.IsDir() || !info.IsDir())
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *Manager) isRegisteredSubmodule(rel string) bool {
	cmd := exec.Command("git", "-C", m.root, "config", "-f", ".gitmodules", "--get-regexp", `^submodule\..*\.path$`)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		if fields[1] == rel {
			return true
		}
	}
	return false
}

func (m *Manager) loadActive() ([]string, error) {
	path := filepath.Join(m.root, activeCatalogPath)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	active := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, exists := seen[line]; exists {
			continue
		}
		seen[line] = struct{}{}
		active = append(active, line)
	}
	sort.Strings(active)
	return active, nil
}

func (m *Manager) writeActive(items []string) error {
	path := filepath.Join(m.root, activeCatalogPath)
	content := ""
	if len(items) > 0 {
		content = strings.Join(items, "\n") + "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
