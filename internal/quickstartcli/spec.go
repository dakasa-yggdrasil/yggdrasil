// Package quickstartcli implements the adopter-side CLI flow for installing
// integrations through a yggdrasil-core's POST /api/v1/integrations/install
// endpoint. The package fetches the integration's yggdrasil-quickstart.yaml
// manifest, walks the user through the declared inputs (interactively via a
// huh-powered TUI or non-interactively via flags), and then submits the
// install request to a remote yggdrasil-core.
//
// The schema mirrors the subset of yggdrasil-core's
// model.IntegrationQuickstartManifestSpec the CLI needs to render the form
// and build the request body — we deliberately don't import yggdrasil-core
// because this CLI is meant to ship as a self-contained binary.
package quickstartcli

// QuickstartDocument is the manifest envelope (apiVersion / kind / metadata
// / spec). The CLI only inspects spec; the rest is forwarded to the server.
type QuickstartDocument struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Metadata   QuickstartMetadata     `json:"metadata"`
	Spec       QuickstartSpec         `json:"spec"`
}

type QuickstartMetadata struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace,omitempty"`
	Description string `json:"description,omitempty"`
}

// QuickstartSpec is the body of the integration_quickstart manifest.
type QuickstartSpec struct {
	DisplayName string                `json:"display_name,omitempty"`
	Description string                `json:"description,omitempty"`
	Providers   []QuickstartProvider  `json:"providers"`
}

type QuickstartProvider struct {
	ID               string                  `json:"id"`
	DisplayName      string                  `json:"display_name,omitempty"`
	Description      string                  `json:"description,omitempty"`
	Requires         []QuickstartRequirement `json:"requires,omitempty"`
	Inputs           []QuickstartInput       `json:"inputs,omitempty"`
	PostInstallHints []string                `json:"post_install_hints,omitempty"`
}

type QuickstartRequirement struct {
	Kind       string `json:"kind"`
	Name       string `json:"name,omitempty"`
	Capability string `json:"capability,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type QuickstartInput struct {
	ID          string             `json:"id"`
	Label       string             `json:"label"`
	Description string             `json:"description,omitempty"`
	Help        string             `json:"help,omitempty"`
	Type        string             `json:"type,omitempty"`
	Default     any                `json:"default,omitempty"`
	Required    bool               `json:"required,omitempty"`
	Sensitive   bool               `json:"sensitive,omitempty"`
	Choices     []QuickstartChoice `json:"choices,omitempty"`
	Validate    *QuickstartValidate `json:"validate,omitempty"`
}

type QuickstartChoice struct {
	Value       string `json:"value"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

type QuickstartValidate struct {
	Regex   string `json:"regex,omitempty"`
	Min     *int   `json:"min,omitempty"`
	Max     *int   `json:"max,omitempty"`
	Message string `json:"message,omitempty"`
}

// FindProvider returns the provider matching id, or the only provider when
// id is empty and the manifest declares exactly one.
func (s QuickstartSpec) FindProvider(id string) (QuickstartProvider, error) {
	if id == "" && len(s.Providers) == 1 {
		return s.Providers[0], nil
	}
	for _, p := range s.Providers {
		if p.ID == id {
			return p, nil
		}
	}
	return QuickstartProvider{}, errProviderNotFound(id, s.Providers)
}
