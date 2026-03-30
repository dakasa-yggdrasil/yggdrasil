package surfaces

import (
	"fmt"
	"io/fs"
	"os"
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
	root string
}

func NewManager(root string) *Manager {
	return &Manager{root: root}
}

func (m *Manager) List() ([]string, error) {
	return m.loadActive()
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
	slug := normalizeSurfaceName(name)
	if slug == "" {
		return fmt.Errorf("surface name is required")
	}

	target := filepath.Join(m.root, "surfaces", slug)
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		return fmt.Errorf("surface not found at %s", target)
	}

	active, err := m.loadActive()
	if err != nil {
		return err
	}
	for _, item := range active {
		if item == slug {
			return nil
		}
	}

	active = append(active, slug)
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
