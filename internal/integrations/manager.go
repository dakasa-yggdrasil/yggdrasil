package integrations

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

type Manager struct {
	root    string
	catalog *Catalog
}

func NewManager(root string) (*Manager, error) {
	catalog, err := loadCatalog(root)
	if err != nil {
		return nil, err
	}
	return &Manager{root: root, catalog: catalog}, nil
}

func (m *Manager) List() ([]IntegrationStatus, error) {
	entries := make([]IntegrationStatus, 0, len(m.catalog.Integrations))
	for _, integration := range m.catalog.Integrations {
		path := filepath.Join(m.root, "integrations", integration.RepoName)
		installed := hasFile(filepath.Join(path, "docker-compose.yml"))
		source := "remote"
		if local := m.localSourcePath(integration); local != "" {
			source = "local"
		}
		if !installed {
			source = ""
		}
		entries = append(entries, IntegrationStatus{
			Integration: integration,
			Installed:   installed,
			Source:      source,
			Path:        path,
		})
	}
	return entries, nil
}

func (m *Manager) ComposeFiles() ([]string, error) {
	pattern := filepath.Join(m.root, "integrations", "*", "docker-compose.yml")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func (m *Manager) Install(slug string) (Integration, string, error) {
	entry, err := m.find(slug)
	if err != nil {
		return Integration{}, "", err
	}

	target := filepath.Join(m.root, "integrations", entry.RepoName)
	if hasFile(filepath.Join(target, "docker-compose.yml")) {
		return entry, "installed", nil
	}
	if pathExists(target) {
		return Integration{}, "", fmt.Errorf("target path already exists and is not a managed integration: %s", target)
	}

	source, sourceLabel, err := m.installSource(entry)
	if err != nil {
		return Integration{}, "", err
	}

	args := []string{"-C", m.root}
	if sourceLabel == "local" {
		args = append(args, "-c", "protocol.file.allow=always")
	}
	args = append(args, "submodule", "add", source, filepath.Join("integrations", entry.RepoName))
	if err := run("git", args...); err != nil {
		return Integration{}, "", err
	}

	if err := run("git", "-C", m.root, "submodule", "update", "--init", "--recursive", filepath.Join("integrations", entry.RepoName)); err != nil {
		return Integration{}, "", err
	}

	if err := initEnv(filepath.Join(target, ".env.example"), filepath.Join(target, ".env")); err != nil {
		return Integration{}, "", err
	}

	return entry, sourceLabel, nil
}

func (m *Manager) Remove(slug string) (Integration, error) {
	entry, err := m.find(slug)
	if err != nil {
		return Integration{}, err
	}

	rel := filepath.Join("integrations", entry.RepoName)
	target := filepath.Join(m.root, rel)
	if !pathExists(target) {
		return entry, nil
	}

	if err := run("git", "-C", m.root, "submodule", "deinit", "-f", "--", rel); err != nil {
		return Integration{}, err
	}
	if err := run("git", "-C", m.root, "rm", "-f", rel); err != nil {
		return Integration{}, err
	}
	_ = os.RemoveAll(filepath.Join(m.root, ".git", "modules", rel))
	return entry, nil
}

func (m *Manager) find(slug string) (Integration, error) {
	for _, integration := range m.catalog.Integrations {
		if integration.Slug == slug {
			return integration, nil
		}
	}
	return Integration{}, fmt.Errorf("integration %q not found in catalog", slug)
}

func (m *Manager) installSource(entry Integration) (string, string, error) {
	if local := m.localSourcePath(entry); local != "" {
		return local, "local", nil
	}
	if entry.RepoURL != "" {
		return entry.RepoURL, "remote", nil
	}
	return "", "", errors.New("no install source available")
}

func (m *Manager) localSourcePath(entry Integration) string {
	if devDir := os.Getenv("YGGDRASIL_INTEGRATIONS_DEV_DIR"); devDir != "" {
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

func initEnv(examplePath, envPath string) error {
	if !hasFile(examplePath) || pathExists(envPath) {
		return nil
	}

	content, err := os.ReadFile(examplePath)
	if err != nil {
		return err
	}
	return os.WriteFile(envPath, content, 0o644)
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && (info.IsDir() || !info.IsDir())
}
