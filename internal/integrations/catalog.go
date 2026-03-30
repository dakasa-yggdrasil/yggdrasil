package integrations

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
)

type Catalog struct {
	Version      string        `json:"version"`
	Integrations []Integration `json:"integrations"`
}

type Integration struct {
	Slug        string `json:"slug"`
	RepoName    string `json:"repo_name"`
	DisplayName string `json:"display_name"`
	Domain      string `json:"domain"`
	Section     string `json:"section"`
	RepoURL     string `json:"repo_url"`
	Description string `json:"description"`
}

type IntegrationStatus struct {
	Integration
	Installed bool
	Source    string
	Path      string
}

func loadCatalog(root string) (*Catalog, error) {
	path := filepath.Join(root, "catalog", "integrations.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var catalog Catalog
	if err := json.Unmarshal(content, &catalog); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	sort.Slice(catalog.Integrations, func(i, j int) bool {
		if catalog.Integrations[i].Domain == catalog.Integrations[j].Domain {
			if catalog.Integrations[i].Section == catalog.Integrations[j].Section {
				return catalog.Integrations[i].Slug < catalog.Integrations[j].Slug
			}
			return catalog.Integrations[i].Section < catalog.Integrations[j].Section
		}
		return catalog.Integrations[i].Domain < catalog.Integrations[j].Domain
	})

	return &catalog, nil
}

func RenderTable(entries []IntegrationStatus) string {
	var builder strings.Builder
	writer := tabwriter.NewWriter(&builder, 0, 2, 2, ' ', 0)
	fmt.Fprintln(writer, "SLUG\tDOMAIN\tSECTION\tSTATUS\tSOURCE")
	for _, entry := range entries {
		status := "available"
		if entry.Installed {
			status = "installed"
		}
		source := entry.Source
		if source == "" {
			source = "-"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", entry.Slug, entry.Domain, entry.Section, status, source)
	}
	_ = writer.Flush()
	return builder.String()
}
