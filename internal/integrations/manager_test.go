package integrations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalSourcePathRequiresExplicitDevDir(t *testing.T) {
	root := t.TempDir()
	devDir := filepath.Join(t.TempDir(), "integrations-dev")
	repoDir := filepath.Join(devDir, "integration-aws")

	t.Setenv("YGGDRASIL_INTEGRATIONS_DEV_DIR", "")
	t.Setenv("YGGDRASIL_SURFACES_DEV_DIR", "")
	if err := ensureFakeGitRepo(repoDir); err != nil {
		t.Fatal(err)
	}

	manager := &Manager{root: root}
	entry := Integration{RepoName: "integration-aws"}

	if got := manager.localSourcePath(entry); got != "" {
		t.Fatalf("expected no local source without explicit dev dir, got %q", got)
	}

	t.Setenv("YGGDRASIL_INTEGRATIONS_DEV_DIR", devDir)
	if got := manager.localSourcePath(entry); got != repoDir {
		t.Fatalf("expected local source %q, got %q", repoDir, got)
	}
}

func TestInstallSourceDefaultsToRemote(t *testing.T) {
	manager := &Manager{root: t.TempDir()}
	entry := Integration{
		RepoName: "integration-aws",
		RepoURL:  "https://github.com/dakasa-yggdrasil/integration-aws.git",
	}

	t.Setenv("YGGDRASIL_INTEGRATIONS_DEV_DIR", "")

	source, label, err := manager.installSource(entry)
	if err != nil {
		t.Fatalf("installSource returned error: %v", err)
	}
	if source != entry.RepoURL {
		t.Fatalf("expected remote source %q, got %q", entry.RepoURL, source)
	}
	if label != "remote" {
		t.Fatalf("expected remote label, got %q", label)
	}
}

func ensureFakeGitRepo(path string) error {
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
		return err
	}
	return nil
}
