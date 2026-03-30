package manifest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/dakasa-co/yggdrasil-core/model"
)

var (
	supportedIntegrationCapabilities = []string{
		"describe",
		"discover",
		"read",
		"execute",
		"sync",
		"health",
	}
	supportedIntegrationTransports     = []string{"rabbitmq"}
	supportedIntegrationSchemaModes    = []string{"none", "inline", "secret_ref"}
	supportedIntegrationSchemaTypes    = []string{"string", "number", "integer", "boolean", "object", "array"}
	supportedIntegrationDiscoveryModes = []string{"pull", "push", "hybrid"}
	supportedIntegrationCursorModes    = []string{"none", "full", "incremental"}
	integrationNamePattern             = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]*$`)
)

// ParseIntegrationTypeSpec parses the raw spec payload into the typed integration type manifest.
func ParseIntegrationTypeSpec(raw json.RawMessage) (model.IntegrationTypeManifestSpec, error) {
	var spec model.IntegrationTypeManifestSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return model.IntegrationTypeManifestSpec{}, fmt.Errorf("parse integration_type spec: %w", err)
	}
	return spec, nil
}

// ValidateIntegrationTypeSpec validates the adapter contract exposed through an integration_type manifest.
func ValidateIntegrationTypeSpec(spec model.IntegrationTypeManifestSpec) error {
	provider := normalizeIntegrationName(spec.Provider)
	if provider == "" {
		return fmt.Errorf("integration_type provider is required")
	}
	if !integrationNamePattern.MatchString(provider) {
		return fmt.Errorf("integration_type provider %q is invalid", spec.Provider)
	}

	if err := validateIntegrationAdapter(spec.Adapter, spec.Capabilities); err != nil {
		return err
	}
	if err := validateIntegrationCapabilities(spec.Capabilities); err != nil {
		return err
	}
	if err := validateIntegrationSchema("credential_schema", spec.CredentialSchema); err != nil {
		return err
	}
	if err := validateIntegrationSchema("instance_schema", spec.InstanceSchema); err != nil {
		return err
	}
	if err := validateIntegrationResourceTypes(spec.ResourceTypes); err != nil {
		return err
	}
	if err := validateIntegrationActionCatalog(spec.ActionCatalog, spec.ResourceTypes); err != nil {
		return err
	}
	if err := validateIntegrationDiscovery(spec.Discovery); err != nil {
		return err
	}
	if err := validateIntegrationNormalization(spec.Normalization); err != nil {
		return err
	}
	if err := validateIntegrationExecution(spec.Execution, spec.ActionCatalog, spec.ResourceTypes, spec.Extensions); err != nil {
		return err
	}

	return nil
}

func validateIntegrationAdapter(adapter model.IntegrationAdapterSpec, capabilities []string) error {
	transport := strings.ToLower(strings.TrimSpace(adapter.Transport))
	if !slices.Contains(supportedIntegrationTransports, transport) {
		return fmt.Errorf("integration_type adapter transport %q is unsupported", adapter.Transport)
	}

	if strings.TrimSpace(adapter.Version) == "" {
		return fmt.Errorf("integration_type adapter version is required")
	}
	if adapter.TimeoutSeconds <= 0 {
		return fmt.Errorf("integration_type adapter timeout_seconds must be greater than zero")
	}

	queues := map[string]string{
		"describe": strings.TrimSpace(adapter.Queues.Describe),
		"discover": strings.TrimSpace(adapter.Queues.Discover),
		"read":     strings.TrimSpace(adapter.Queues.Read),
		"execute":  strings.TrimSpace(adapter.Queues.Execute),
		"sync":     strings.TrimSpace(adapter.Queues.Sync),
		"health":   strings.TrimSpace(adapter.Queues.Health),
	}

	capabilitySet := toIntegrationNameSet(capabilities)
	for capability := range capabilitySet {
		if queues[capability] == "" {
			return fmt.Errorf("integration_type adapter queue for capability %q is required", capability)
		}
	}

	return nil
}

func validateIntegrationCapabilities(capabilities []string) error {
	if len(capabilities) == 0 {
		return fmt.Errorf("integration_type capabilities require at least one value")
	}

	seen := map[string]struct{}{}
	for _, capability := range capabilities {
		capability = normalizeIntegrationName(capability)
		if capability == "" {
			return fmt.Errorf("integration_type capabilities cannot contain empty values")
		}
		if !slices.Contains(supportedIntegrationCapabilities, capability) {
			return fmt.Errorf("integration_type capability %q is unsupported", capability)
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("integration_type capability %q is duplicated", capability)
		}
		seen[capability] = struct{}{}
	}

	if _, hasDescribe := seen["describe"]; !hasDescribe {
		return fmt.Errorf("integration_type capabilities must include describe")
	}

	return nil
}

func validateIntegrationSchema(label string, schema model.IntegrationSchemaSpec) error {
	mode := strings.ToLower(strings.TrimSpace(schema.Mode))
	if !slices.Contains(supportedIntegrationSchemaModes, mode) {
		return fmt.Errorf("integration_type %s mode %q is unsupported", label, schema.Mode)
	}

	requiredSeen := map[string]struct{}{}
	for _, field := range schema.Required {
		field = normalizeIntegrationName(field)
		if field == "" {
			return fmt.Errorf("integration_type %s required cannot contain empty values", label)
		}
		if _, exists := requiredSeen[field]; exists {
			return fmt.Errorf("integration_type %s required field %q is duplicated", label, field)
		}
		requiredSeen[field] = struct{}{}
	}

	for name, property := range schema.Properties {
		normalizedName := normalizeIntegrationName(name)
		if normalizedName == "" {
			return fmt.Errorf("integration_type %s property name is required", label)
		}
		propertyType := strings.ToLower(strings.TrimSpace(property.Type))
		if !slices.Contains(supportedIntegrationSchemaTypes, propertyType) {
			return fmt.Errorf("integration_type %s property %q has unsupported type %q", label, name, property.Type)
		}
	}

	if len(schema.Properties) > 0 {
		for field := range requiredSeen {
			if _, exists := schema.Properties[field]; !exists {
				return fmt.Errorf("integration_type %s required field %q must exist in properties", label, field)
			}
		}
	}

	return nil
}

func validateIntegrationResourceTypes(resourceTypes []model.IntegrationResourceType) error {
	if len(resourceTypes) == 0 {
		return fmt.Errorf("integration_type resource_types require at least one value")
	}

	seen := map[string]struct{}{}
	for _, resourceType := range resourceTypes {
		name := normalizeIntegrationName(resourceType.Name)
		if name == "" {
			return fmt.Errorf("integration_type resource_type name is required")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("integration_type resource_type %q is duplicated", name)
		}
		seen[name] = struct{}{}

		if strings.TrimSpace(resourceType.CanonicalPrefix) == "" {
			return fmt.Errorf("integration_type resource_type %q canonical_prefix is required", name)
		}
		if strings.TrimSpace(resourceType.IdentityTemplate) == "" {
			return fmt.Errorf("integration_type resource_type %q identity_template is required", name)
		}
		if len(resourceType.DefaultActions) == 0 {
			return fmt.Errorf("integration_type resource_type %q default_actions require at least one value", name)
		}

		actionSeen := map[string]struct{}{}
		for _, action := range resourceType.DefaultActions {
			action = normalizeIntegrationName(action)
			if action == "" {
				return fmt.Errorf("integration_type resource_type %q default_actions cannot contain empty values", name)
			}
			if _, exists := actionSeen[action]; exists {
				return fmt.Errorf("integration_type resource_type %q default action %q is duplicated", name, action)
			}
			actionSeen[action] = struct{}{}
		}
	}

	return nil
}

func validateIntegrationActionCatalog(actions []model.IntegrationActionDefinition, resourceTypes []model.IntegrationResourceType) error {
	if len(actions) == 0 {
		return nil
	}

	resourceTypeNames := map[string]struct{}{}
	for _, resourceType := range resourceTypes {
		resourceTypeNames[normalizeIntegrationName(resourceType.Name)] = struct{}{}
	}

	seen := map[string]struct{}{}
	for _, action := range actions {
		name := normalizeIntegrationName(action.Name)
		if name == "" {
			return fmt.Errorf("integration_type action_catalog name is required")
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("integration_type action_catalog action %q is duplicated", name)
		}
		seen[name] = struct{}{}

		resourceSet := map[string]struct{}{}
		for _, resourceType := range action.ResourceTypes {
			resourceType = normalizeIntegrationName(resourceType)
			if resourceType == "" {
				return fmt.Errorf("integration_type action_catalog action %q contains an empty resource_types value", name)
			}
			if _, exists := resourceTypeNames[resourceType]; !exists {
				return fmt.Errorf("integration_type action_catalog action %q references unknown resource_type %q", name, resourceType)
			}
			if _, exists := resourceSet[resourceType]; exists {
				return fmt.Errorf("integration_type action_catalog action %q duplicates resource_type %q", name, resourceType)
			}
			resourceSet[resourceType] = struct{}{}
		}
	}

	return nil
}

func validateIntegrationDiscovery(discovery model.IntegrationDiscoverySpec) error {
	mode := strings.ToLower(strings.TrimSpace(discovery.Mode))
	if !slices.Contains(supportedIntegrationDiscoveryModes, mode) {
		return fmt.Errorf("integration_type discovery mode %q is unsupported", discovery.Mode)
	}

	cursor := strings.ToLower(strings.TrimSpace(discovery.Cursor))
	if cursor == "" {
		cursor = "none"
	}
	if !slices.Contains(supportedIntegrationCursorModes, cursor) {
		return fmt.Errorf("integration_type discovery cursor %q is unsupported", discovery.Cursor)
	}
	if (mode == "pull" || mode == "hybrid") && cursor == "none" {
		return fmt.Errorf("integration_type discovery mode %q requires a cursor strategy", mode)
	}

	return nil
}

func validateIntegrationNormalization(normalization model.IntegrationNormalizationSpec) error {
	if strings.TrimSpace(normalization.ExternalIDPath) == "" {
		return fmt.Errorf("integration_type normalization external_id_path is required")
	}
	if strings.TrimSpace(normalization.FallbackResourcePrefix) == "" {
		return fmt.Errorf("integration_type normalization fallback_resource_prefix is required")
	}
	return nil
}

func validateIntegrationExecution(
	execution model.IntegrationExecutionSpec,
	actionCatalog []model.IntegrationActionDefinition,
	resourceTypes []model.IntegrationResourceType,
	extensions model.IntegrationExtensionsSpec,
) error {
	knownActions := map[string]struct{}{}
	for _, action := range actionCatalog {
		knownActions[normalizeIntegrationName(action.Name)] = struct{}{}
	}
	for _, resourceType := range resourceTypes {
		for _, action := range resourceType.DefaultActions {
			knownActions[normalizeIntegrationName(action)] = struct{}{}
		}
	}

	seen := map[string]struct{}{}
	for _, action := range execution.IdempotentActions {
		action = normalizeIntegrationName(action)
		if action == "" {
			return fmt.Errorf("integration_type execution idempotent_actions cannot contain empty values")
		}
		if _, exists := seen[action]; exists {
			return fmt.Errorf("integration_type execution idempotent action %q is duplicated", action)
		}
		seen[action] = struct{}{}

		if _, exists := knownActions[action]; !exists && !extensions.AllowCustomActions {
			return fmt.Errorf("integration_type execution idempotent action %q is unknown", action)
		}
	}

	return nil
}

func normalizeIntegrationName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func toIntegrationNameSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeIntegrationName(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}
