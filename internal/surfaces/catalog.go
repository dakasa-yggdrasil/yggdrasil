package surfaces

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
	Version  string    `json:"version"`
	Surfaces []Surface `json:"surfaces"`
}

type Surface struct {
	Slug        string `json:"slug"`
	RepoName    string `json:"repo_name"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
	RepoURL     string `json:"repo_url"`
	Description string `json:"description"`
}

type SurfaceStatus struct {
	Surface
	Installed bool
	Active    bool
	Source    string
	Path      string
}

func loadCatalog(root string) (*Catalog, error) {
	path := filepath.Join(root, "catalog", "surfaces.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var catalog Catalog
	if err := json.Unmarshal(content, &catalog); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	sort.Slice(catalog.Surfaces, func(i, j int) bool {
		if catalog.Surfaces[i].Category == catalog.Surfaces[j].Category {
			return catalog.Surfaces[i].Slug < catalog.Surfaces[j].Slug
		}
		return catalog.Surfaces[i].Category < catalog.Surfaces[j].Category
	})

	return &catalog, nil
}

func RenderTable(entries []SurfaceStatus) string {
	var builder strings.Builder
	writer := tabwriter.NewWriter(&builder, 0, 2, 2, ' ', 0)
	fmt.Fprintln(writer, "SLUG\tCATEGORY\tSTATUS\tACTIVE\tSOURCE")
	for _, entry := range entries {
		status := "available"
		if entry.Installed {
			status = "installed"
		}
		active := "no"
		if entry.Active {
			active = "yes"
		}
		source := entry.Source
		if source == "" {
			source = "-"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", entry.Slug, entry.Category, status, active, source)
	}
	_ = writer.Flush()
	return builder.String()
}
