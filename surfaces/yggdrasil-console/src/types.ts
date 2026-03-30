export type RuntimeStatus =
  | "healthy"
  | "contract_mismatch"
  | "invalid_response"
  | "unreachable"
  | "unknown"
  | "active"
  | "draft"
  | "disabled"
  | "unconfigured"
  | string;

export interface ManifestReference {
  id: string;
  kind: string;
  namespace: string;
  name: string;
  version: number;
}

export interface IntegrationRuntimeState {
  integration_type: ManifestReference;
  check_kind: string;
  status: RuntimeStatus;
  message?: string;
  details?: Record<string, unknown>;
  last_checked_at: string;
  last_success_at?: string | null;
  last_failure_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface IntegrationCatalogInstance {
  integration_instance: ManifestReference;
  description?: string;
  owners?: string[];
  declared_status: string;
  status: RuntimeStatus;
  runtime_state?: IntegrationRuntimeState | null;
}

export interface IntegrationCatalogEntry {
  domain: string;
  section: string;
  entry: string;
  plugin_name: string;
  description?: string;
  provider: string;
  adapter_version?: string;
  status: RuntimeStatus;
  labels?: Record<string, string>;
  capabilities?: string[];
  integration_type: ManifestReference;
  runtime_state?: IntegrationRuntimeState | null;
  instances?: IntegrationCatalogInstance[];
}

export interface IntegrationCatalogSection {
  name: string;
  entries: IntegrationCatalogEntry[];
}

export interface IntegrationCatalogDomain {
  domain: string;
  sections: IntegrationCatalogSection[];
}

export interface IntegrationCatalogResponse {
  domains: IntegrationCatalogDomain[];
}

export interface CatalogDiscoverySource {
  integration_instance: ManifestReference;
  integration_type: ManifestReference;
  provider: string;
  plugin_name: string;
  domain?: string;
  section?: string;
  entry?: string;
  health_status?: RuntimeStatus;
  discovery_status?: string;
  message?: string;
}

export interface CatalogDiscoveryItem {
  source: CatalogDiscoverySource;
  kind: "integration" | "surface" | string;
  name: string;
  namespace?: string;
  display_name?: string;
  description?: string;
  domain?: string;
  section?: string;
  entry?: string;
  repository?: string;
  labels?: Record<string, string>;
  metadata?: Record<string, unknown>;
  registered_manifest?: ManifestReference | null;
  registration_status: "registered" | "unregistered" | string;
}

export interface CatalogDiscoveryResponse {
  sources: CatalogDiscoverySource[];
  items: CatalogDiscoveryItem[];
}

export interface ManifestMetadata {
  name: string;
  namespace: string;
  description?: string;
  labels?: Record<string, string>;
  active: boolean;
}

export interface ManifestRecord {
  id: string;
  apiVersion: string;
  kind: string;
  metadata: ManifestMetadata;
  version: number;
  checksum: string;
  spec: unknown;
  created_at: string;
  updated_at: string;
}

export interface SurfaceRuntimeSpec {
  kind: string;
  exposure?: string;
  port?: number;
  base_path?: string;
  health_path?: string;
}

export interface SurfaceCapabilitySpec {
  name: string;
  kind: string;
  audience?: string;
  path?: string;
  methods?: string[];
}

export interface SurfaceSpec {
  category: string;
  owners?: string[];
  replaces?: string[];
  integration_binding?: string;
  runtime: SurfaceRuntimeSpec;
  core_contracts?: string[];
  capabilities?: SurfaceCapabilitySpec[];
}

export interface IntegrationCatalogEntryDetailResponse {
  entry: IntegrationCatalogEntry;
  integrationTypeManifest: ManifestRecord;
}

export interface IntegrationSchemaProperty {
  type: string;
  description?: string;
  secret?: boolean;
  enum?: unknown[];
  default?: unknown;
}

export interface IntegrationSchemaSpec {
  mode: string;
  required?: string[];
  properties?: Record<string, IntegrationSchemaProperty>;
}

export interface IntegrationResourceType {
  name: string;
  canonical_prefix: string;
  identity_template: string;
  discoverable: boolean;
  default_actions: string[];
}

export interface IntegrationActionDefinition {
  name: string;
  description?: string;
  resource_types?: string[];
  idempotent?: boolean;
}

export interface IntegrationTypeSpec {
  provider: string;
  adapter: {
    transport: string;
    version: string;
    queues: Record<string, string>;
    timeout_seconds?: number;
  };
  capabilities: string[];
  credential_schema: IntegrationSchemaSpec;
  instance_schema: IntegrationSchemaSpec;
  resource_types: IntegrationResourceType[];
  action_catalog?: IntegrationActionDefinition[];
  discovery: {
    mode: string;
    cursor?: string;
    supports_webhooks?: boolean;
  };
  normalization: {
    external_id_path: string;
    name_path?: string;
    owner_path?: string;
    fallback_resource_prefix: string;
  };
  execution: {
    supports_dry_run?: boolean;
    idempotent_actions?: string[];
  };
  extensions: {
    allow_custom_resource_types?: boolean;
    allow_custom_actions?: boolean;
    preserve_raw_payload?: boolean;
  };
}

export interface CreateIntegrationInstancePayload {
  name: string;
  namespace?: string;
  description?: string;
  labels?: Record<string, string>;
  type_ref: {
    name: string;
    namespace?: string;
  };
  status?: string;
  owners?: string[];
  credentials?: Record<string, unknown>;
  config?: Record<string, unknown>;
  discovery?: {
    enabled: boolean;
    mode?: string;
    sync_interval_seconds?: number;
  };
  execution?: {
    default_dry_run?: boolean;
    max_batch_size?: number;
  };
}

export interface CollaboratorRecord {
  id: string;
  slug: string;
  status: string;
  display_name: string;
  primary_email?: string;
  manager_id?: string | null;
  primary_team_id?: string | null;
  personal_data?: Record<string, unknown>;
  employment_data?: Record<string, unknown>;
  third_party_identities?: Record<string, unknown>;
  traits?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface TeamRecord {
  id: string;
  slug: string;
  name: string;
  type?: string;
  status: string;
  parent_team_id?: string | null;
  owners?: string[];
  traits?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface TeamMembershipRecord {
  id: string;
  team_id: string;
  team_slug: string;
  collaborator_id: string;
  collaborator_slug: string;
  role: string;
  active: boolean;
  source: string;
  starts_at?: string | null;
  ends_at?: string | null;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface CollaboratorsResponse {
  collaborators: CollaboratorRecord[];
}

export interface TeamsResponse {
  teams: TeamRecord[];
}

export interface TeamMembershipsResponse {
  memberships: TeamMembershipRecord[];
}

export interface ManifestsResponse {
  manifests: ManifestRecord[];
}

export interface SurfacesResponse {
  manifests: ManifestRecord[];
}

export interface CreateCollaboratorPayload {
  slug: string;
  status?: string;
  display_name: string;
  primary_email?: string;
  manager_id?: string;
  primary_team_id?: string;
  personal_data?: Record<string, unknown>;
  employment_data?: Record<string, unknown>;
  third_party_identities?: Record<string, unknown>;
  traits?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface CreateTeamPayload {
  slug: string;
  name: string;
  type?: string;
  status?: string;
  parent_team_id?: string;
  owners?: string[];
  traits?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface UpsertTeamMembershipPayload {
  team_id: string;
  collaborator_id: string;
  role?: string;
  active?: boolean;
  source?: string;
  starts_at?: string;
  ends_at?: string;
  metadata?: Record<string, unknown>;
}

export interface CreateManifestPayload {
  name: string;
  namespace?: string;
  description?: string;
  labels?: Record<string, string>;
  spec: unknown;
}
