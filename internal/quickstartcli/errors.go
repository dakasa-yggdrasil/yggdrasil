package quickstartcli

import (
	"fmt"
	"strings"
)

// errProviderNotFound builds a helpful "provider X not found, choose one of
// {a,b,c}" error so the CLI can guide the adopter toward a valid value.
func errProviderNotFound(id string, providers []QuickstartProvider) error {
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.ID)
	}
	if id == "" {
		return fmt.Errorf("manifest declares %d providers (%s); pass --provider <id> to pick one", len(providers), strings.Join(names, ", "))
	}
	return fmt.Errorf("provider %q not found (available: %s)", id, strings.Join(names, ", "))
}
