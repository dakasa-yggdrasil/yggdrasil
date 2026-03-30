import type {
  CatalogDiscoveryResponse,
  CollaboratorsResponse,
  IntegrationCatalogEntryDetailResponse,
  IntegrationCatalogResponse,
  ManifestRecord,
  ManifestsResponse,
  SurfacesResponse,
  TeamMembershipsResponse,
  TeamsResponse,
} from "../types";

const now = "2026-03-28T12:00:00Z";

const manifest = (id: string, name: string, namespace: string, spec: unknown): ManifestRecord => ({
  id,
  apiVersion: "yggdrasil.io/v1alpha1",
  kind: "integration_type",
  metadata: {
    name,
    namespace,
    description: `${name} integration type`,
    active: true,
  },
  version: 1,
  checksum: "mock-checksum",
  spec,
  created_at: now,
  updated_at: now,
});

const typedManifest = (
  kind: "product" | "workflow" | "surface",
  id: string,
  name: string,
  namespace: string,
  description: string,
  spec: unknown,
  labels?: Record<string, string>,
): ManifestRecord => ({
  id,
  apiVersion: "yggdrasil.io/v1alpha1",
  kind,
  metadata: {
    name,
    namespace,
    description,
    labels,
    active: true,
  },
  version: 1,
  checksum: `mock-${kind}-${name}`,
  spec,
  created_at: now,
  updated_at: now,
});

export const mockCatalogResponse: IntegrationCatalogResponse = {
  domains: [
    {
      domain: "rabbitmq",
      sections: [
        {
          name: "operations",
          entries: [
            {
              domain: "rabbitmq",
              section: "operations",
              entry: "api",
              plugin_name: "rabbitmq",
              description: "RabbitMQ runtime governance via the Management API.",
              provider: "rabbitmq",
              adapter_version: "1.0.0",
              status: "healthy",
              capabilities: ["describe", "execute"],
              integration_type: {
                id: "it-rabbitmq",
                kind: "integration_type",
                namespace: "global",
                name: "rabbitmq",
                version: 1,
              },
              runtime_state: {
                integration_type: {
                  id: "it-rabbitmq",
                  kind: "integration_type",
                  namespace: "global",
                  name: "rabbitmq",
                  version: 1,
                },
                check_kind: "describe_handshake",
                status: "healthy",
                message: "mocked healthy adapter",
                last_checked_at: now,
                created_at: now,
                updated_at: now,
              },
              instances: [
                {
                  integration_instance: {
                    id: "ii-rabbitmq",
                    kind: "integration_instance",
                    namespace: "global",
                    name: "rabbitmq-platform-api",
                    version: 1,
                  },
                  description: "Primary broker runtime for platform automation.",
                  owners: ["team:platform"],
                  declared_status: "active",
                  status: "healthy",
                },
              ],
            },
          ],
        },
        {
          name: "installations",
          entries: [
            {
              domain: "rabbitmq",
              section: "installations",
              entry: "kubernetes",
              plugin_name: "rabbitmq-on-kubernetes",
              description: "Installs RabbitMQ clusters on Kubernetes.",
              provider: "rabbitmq",
              adapter_version: "1.0.0",
              status: "healthy",
              capabilities: ["describe", "execute"],
              integration_type: {
                id: "it-rabbitmq-k8s",
                kind: "integration_type",
                namespace: "global",
                name: "rabbitmq-on-kubernetes",
                version: 1,
              },
            },
          ],
        },
      ],
    },
    {
      domain: "grafana",
      sections: [
        {
          name: "operations",
          entries: [
            {
              domain: "grafana",
              section: "operations",
              entry: "api",
              plugin_name: "grafana",
              description: "Grafana runtime governance for folders, datasources, and dashboards.",
              provider: "grafana",
              adapter_version: "1.0.0",
              status: "contract_mismatch",
              capabilities: ["describe", "execute"],
              integration_type: {
                id: "it-grafana",
                kind: "integration_type",
                namespace: "global",
                name: "grafana",
                version: 1,
              },
              instances: [
                {
                  integration_instance: {
                    id: "ii-grafana",
                    kind: "integration_instance",
                    namespace: "global",
                    name: "grafana-platform-api",
                    version: 1,
                  },
                  description: "Primary Grafana runtime API.",
                  owners: ["team:platform"],
                  declared_status: "active",
                  status: "contract_mismatch",
                },
              ],
            },
          ],
        },
      ],
    },
    {
      domain: "github",
      sections: [
        {
          name: "operations",
          entries: [
            {
              domain: "github",
              section: "operations",
              entry: "api",
              plugin_name: "github",
              description: "GitHub repositories, environments, teams, and workflow dispatch.",
              provider: "github",
              adapter_version: "1.0.0",
              status: "healthy",
              capabilities: ["describe", "execute"],
              integration_type: {
                id: "it-github",
                kind: "integration_type",
                namespace: "global",
                name: "github",
                version: 1,
              },
            },
          ],
        },
      ],
    },
  ],
};

const typeSpecs: Record<string, IntegrationCatalogEntryDetailResponse> = {
  "rabbitmq:operations:api": {
    entry: requireEntry(0, 0, 0),
    integrationTypeManifest: manifest("it-rabbitmq", "rabbitmq", "global", {
      provider: "rabbitmq",
      credential_schema: {
        mode: "inline",
        required: ["username", "password"],
        properties: {
          username: { type: "string", description: "Management API username." },
          password: { type: "string", description: "Management API password.", secret: true },
        },
      },
      instance_schema: {
        mode: "inline",
        properties: {
          management_url: { type: "string", default: "https://rabbitmq.example.com/api" },
          default_vhost: { type: "string", default: "/" },
          insecure_skip_verify: { type: "boolean", default: false },
        },
      },
    }),
  },
  "grafana:operations:api": {
    entry: requireEntry(1, 0, 0),
    integrationTypeManifest: manifest("it-grafana", "grafana", "global", {
      provider: "grafana",
      credential_schema: {
        mode: "inline",
        properties: {
          token: { type: "string", secret: true },
          username: { type: "string" },
          password: { type: "string", secret: true },
        },
      },
      instance_schema: {
        mode: "inline",
        properties: {
          base_url: { type: "string", default: "https://grafana.example.com/api" },
          default_folder_uid: { type: "string", default: "platform" },
          default_folder_title: { type: "string", default: "Platform" },
          org_id: { type: "string", default: "1" },
        },
      },
    }),
  },
  "github:operations:api": {
    entry: requireEntry(2, 0, 0),
    integrationTypeManifest: manifest("it-github", "github", "global", {
      provider: "github",
      credential_schema: {
        mode: "inline",
        properties: {
          token: { type: "string", secret: true },
        },
      },
      instance_schema: {
        mode: "inline",
        properties: {
          default_owner: { type: "string" },
          default_ref: { type: "string", default: "main" },
          default_workflow: { type: "string" },
          default_visibility: { type: "string", default: "private", enum: ["private", "public", "internal"] },
          api_base_url: { type: "string", default: "https://api.github.com" },
        },
      },
    }),
  },
};

export function mockIntegrationCatalog(): IntegrationCatalogResponse {
  return mockCatalogResponse;
}

export function mockCatalogDiscovery(): CatalogDiscoveryResponse {
  return {
    sources: [
      {
        integration_instance: {
          id: "ii-github-discovery",
          kind: "integration_instance",
          namespace: "global",
          name: "github-discovery",
          version: 1,
        },
        integration_type: {
          id: "it-github",
          kind: "integration_type",
          namespace: "global",
          name: "github",
          version: 1,
        },
        provider: "github",
        plugin_name: "github",
        domain: "github",
        section: "operations",
        entry: "api",
        health_status: "healthy",
        discovery_status: "succeeded",
      },
    ],
    items: [
      {
        source: {
          integration_instance: {
            id: "ii-github-discovery",
            kind: "integration_instance",
            namespace: "global",
            name: "github-discovery",
            version: 1,
          },
          integration_type: {
            id: "it-github",
            kind: "integration_type",
            namespace: "global",
            name: "github",
            version: 1,
          },
          provider: "github",
          plugin_name: "github",
          health_status: "healthy",
          discovery_status: "succeeded",
        },
        kind: "integration",
        name: "rabbitmq",
        display_name: "RabbitMQ",
        description: "Runtime and governance plugin discovered from the enterprise org.",
        domain: "rabbitmq",
        section: "operations",
        entry: "api",
        repository: "https://github.com/dakasa-yggdrasil/integration-rabbitmq",
        registration_status: "registered",
        registered_manifest: {
          id: "it-rabbitmq",
          kind: "integration_type",
          namespace: "global",
          name: "rabbitmq",
          version: 1,
        },
        metadata: {
          source: "github",
          topics: ["integration", "yggdrasil-plugin"],
        },
      },
      {
        source: {
          integration_instance: {
            id: "ii-github-discovery",
            kind: "integration_instance",
            namespace: "global",
            name: "github-discovery",
            version: 1,
          },
          integration_type: {
            id: "it-github",
            kind: "integration_type",
            namespace: "global",
            name: "github",
            version: 1,
          },
          provider: "github",
          plugin_name: "github",
          health_status: "healthy",
          discovery_status: "succeeded",
        },
        kind: "surface",
        name: "payments-api",
        namespace: "global",
        display_name: "Payments API",
        description: "A niche operator-facing API surface discovered from the enterprise org.",
        repository: "https://github.com/dakasa-yggdrasil/payments-api",
        registration_status: "unregistered",
        metadata: {
          category: "api",
          source: "github",
        },
      },
    ],
  };
}

export function mockCatalogEntryDetail(
  domain: string,
  section: string,
  entry: string,
): IntegrationCatalogEntryDetailResponse {
  const key = `${domain}:${section}:${entry}`;
  return typeSpecs[key] ?? requireTypeSpec("github:operations:api");
}

export function mockCollaborators(): CollaboratorsResponse {
  return {
    collaborators: [
      {
        id: "col-ana",
        slug: "ana",
        status: "active",
        display_name: "Ana Platform",
        primary_email: "ana@dakasa.dev",
        primary_team_id: "team-platform",
        employment_data: {
          title: "Platform engineer",
          level: "staff",
        },
        third_party_identities: {
          github: {
            login: "anaplatform",
          },
        },
        traits: {
          focus: "platform",
        },
        metadata: {},
        created_at: now,
        updated_at: now,
      },
      {
        id: "col-otavio",
        slug: "otavio",
        status: "active",
        display_name: "Otavio Identity",
        primary_email: "otavio@dakasa.dev",
        primary_team_id: "team-identity",
        employment_data: {
          title: "Identity engineer",
          level: "senior",
        },
        third_party_identities: {
          github: {
            login: "otavioidentity",
          },
        },
        traits: {
          focus: "identity",
        },
        metadata: {},
        created_at: now,
        updated_at: now,
      },
    ],
  };
}

export function mockTeams(): TeamsResponse {
  return {
    teams: [
      {
        id: "team-platform",
        slug: "platform",
        name: "Platform",
        type: "engineering",
        status: "active",
        owners: ["collaborator:ana"],
        traits: {
          domain: "platform",
        },
        metadata: {},
        created_at: now,
        updated_at: now,
      },
      {
        id: "team-identity",
        slug: "identity",
        name: "Identity",
        type: "engineering",
        status: "active",
        owners: ["collaborator:otavio"],
        traits: {
          domain: "identity",
        },
        metadata: {},
        created_at: now,
        updated_at: now,
      },
    ],
  };
}

export function mockTeamMemberships(): TeamMembershipsResponse {
  return {
    memberships: [
      {
        id: "membership-1",
        team_id: "team-platform",
        team_slug: "platform",
        collaborator_id: "col-ana",
        collaborator_slug: "ana",
        role: "lead",
        active: true,
        source: "console",
        created_at: now,
        updated_at: now,
      },
      {
        id: "membership-2",
        team_id: "team-identity",
        team_slug: "identity",
        collaborator_id: "col-otavio",
        collaborator_slug: "otavio",
        role: "member",
        active: true,
        source: "console",
        created_at: now,
        updated_at: now,
      },
    ],
  };
}

export function mockProducts(): ManifestsResponse {
  return {
    manifests: [
      typedManifest(
        "product",
        "product-rabbitmq",
        "rabbitmq-platform",
        "global",
        "Platform RabbitMQ baseline installation.",
        {
          category: "messaging",
          class: "platform",
          owners: ["team:platform"],
          lifecycle: {
            stage: "production",
            tier: "critical",
          },
          components: [
            {
              name: "broker",
              source: {
                kind: "integration",
                integration_instance_ref: {
                  namespace: "global",
                  name: "rabbitmq-platform-kubernetes",
                },
                operation: "generate_installation",
                input: {
                  blueprint: "shared-broker",
                  namespace: "messaging",
                },
              },
              renderer: {
                kind: "raw_k8s",
              },
              target: {
                kind: "kubernetes",
                integration_instance_ref: {
                  namespace: "global",
                  name: "kubernetes-platform-prod",
                },
                namespace: "messaging",
              },
            },
          ],
        },
        {
          surface: "console",
        },
      ),
      typedManifest(
        "product",
        "product-grafana",
        "grafana-platform",
        "global",
        "Platform Grafana baseline installation.",
        {
          category: "observability",
          class: "platform",
          owners: ["team:platform"],
          components: [
            {
              name: "grafana",
              source: {
                kind: "integration",
                integration_instance_ref: {
                  namespace: "global",
                  name: "grafana-platform-kubernetes",
                },
                operation: "generate_installation",
                input: {
                  blueprint: "single-instance",
                  namespace: "observability",
                },
              },
              renderer: {
                kind: "raw_k8s",
              },
              target: {
                kind: "kubernetes",
                integration_instance_ref: {
                  namespace: "global",
                  name: "kubernetes-platform-prod",
                },
                namespace: "observability",
              },
            },
          ],
        },
      ),
    ],
  };
}

export function mockSurfaces(): SurfacesResponse {
  return {
    manifests: [
      typedManifest(
        "surface",
        "surface-auth",
        "yggdrasil-auth-surface",
        "global",
        "Reference collaborator-facing auth surface.",
        {
          category: "auth",
          owners: ["team:platform"],
          replaces: ["auth", "identities"],
          integration_binding: "core_only",
          runtime: {
            kind: "http_api",
            exposure: "collaborator",
            port: 9090,
            base_path: "/",
            health_path: "/healthz",
          },
          core_contracts: ["authorization", "collaborator", "team", "surface"],
          capabilities: [
            {
              name: "collaborator-auth",
              kind: "auth_flow",
              audience: "collaborator",
            },
            {
              name: "auth-home",
              kind: "endpoint",
              audience: "collaborator",
              path: "/",
              methods: ["GET"],
            },
          ],
        },
        {
          "yggdrasil.io/surface-reference": "true",
        },
      ),
      typedManifest(
        "surface",
        "surface-console",
        "yggdrasil-console",
        "global",
        "Reference operator-facing web console.",
        {
          category: "console",
          owners: ["team:platform"],
          replaces: ["console"],
          integration_binding: "core_only",
          runtime: {
            kind: "web_ui",
            exposure: "operator",
            port: 3080,
            base_path: "/",
            health_path: "/",
          },
          core_contracts: ["authorization", "collaborator", "team", "product", "workflow", "integration_catalog"],
          capabilities: [
            {
              name: "catalog-ui",
              kind: "ui_area",
              audience: "operator",
              path: "/catalog",
            },
          ],
        },
        {
          "yggdrasil.io/surface-reference": "true",
        },
      ),
    ],
  };
}

export function mockWorkflows(): ManifestsResponse {
  return {
    manifests: [
      typedManifest(
        "workflow",
        "workflow-github-dispatch",
        "github-dispatch",
        "global",
        "Dispatches one GitHub Actions workflow through the global GitHub integration.",
        {
          trigger: {
            mode: "manual",
          },
          input_schema: {
            required: ["repository", "workflow"],
            properties: {
              repository: {
                type: "string",
              },
              workflow: {
                type: "string",
              },
              ref: {
                type: "string",
              },
            },
          },
          steps: [
            {
              id: "dispatch",
              use: {
                kind: "integration",
                instance_ref: {
                  namespace: "global",
                  name: "github-caller",
                },
                capability: "dispatch_workflow",
              },
              with: {
                repository: "{{ inputs.repository }}",
                workflow: "{{ inputs.workflow }}",
                ref: "{{ inputs.ref }}",
              },
            },
          ],
        },
      ),
      typedManifest(
        "workflow",
        "workflow-product-observe",
        "product-observe",
        "global",
        "Observes one product installation after deployment.",
        {
          trigger: {
            mode: "manual",
          },
          input_schema: {
            required: ["product_name"],
            properties: {
              product_name: {
                type: "string",
              },
            },
          },
          steps: [
            {
              id: "observe",
              use: {
                kind: "integration",
                instance_ref: {
                  namespace: "global",
                  name: "kubernetes-platform-prod",
                },
                capability: "observe_objects",
              },
              with: {
                product_name: "{{ inputs.product_name }}",
              },
            },
          ],
        },
      ),
    ],
  };
}

function requireEntry(domainIndex: number, sectionIndex: number, entryIndex: number) {
  const entry =
    mockCatalogResponse.domains[domainIndex]?.sections[sectionIndex]?.entries[entryIndex];
  if (!entry) {
    throw new Error("mock catalog entry is missing");
  }
  return entry;
}

function requireTypeSpec(key: string) {
  const detail = typeSpecs[key];
  if (!detail) {
    throw new Error(`mock type spec is missing for ${key}`);
  }
  return detail;
}
