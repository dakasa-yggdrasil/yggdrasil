import { useDeferredValue, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createSurface, fetchSurfaces, type DataSourceMode } from "../lib/api";
import { StatusPill } from "../components/StatusPill";
import type {
  CreateManifestPayload,
  ManifestRecord,
  SurfaceCapabilitySpec,
  SurfaceSpec,
} from "../types";

interface SurfacesPageProps {
  source: DataSourceMode;
}

interface SurfaceCapabilityFormState {
  name: string;
  kind: string;
  audience: string;
  path: string;
  methods: string;
}

interface SurfaceFormState {
  name: string;
  namespace: string;
  description: string;
  labels: string;
  category: string;
  owners: string;
  replaces: string;
  integrationBinding: string;
  runtimeKind: string;
  exposure: string;
  port: string;
  basePath: string;
  healthPath: string;
  coreContracts: string;
  capabilities: SurfaceCapabilityFormState[];
}

const defaultSurfaceFormState = (): SurfaceFormState => ({
  name: "my-domain-api",
  namespace: "global",
  description: "Custom operator-facing surface authored from the console.",
  labels: "yggdrasil.io/surface-reference=false\nyggdrasil.io/surface-category=api",
  category: "api",
  owners: "team:platform",
  replaces: "api",
  integrationBinding: "core_only",
  runtimeKind: "http_api",
  exposure: "internal",
  port: "9090",
  basePath: "/",
  healthPath: "/healthz",
  coreContracts: "authorization,workflow,product,integration_catalog",
  capabilities: [
    {
      name: "root-endpoint",
      kind: "endpoint",
      audience: "internal",
      path: "/",
      methods: "GET",
    },
  ],
});

export function SurfacesPage({ source }: SurfacesPageProps) {
  const queryClient = useQueryClient();
  const [filter, setFilter] = useState("");
  const [editingManifest, setEditingManifest] = useState<ManifestRecord | null>(null);
  const deferredFilter = useDeferredValue(filter.trim().toLowerCase());

  const surfacesQuery = useQuery({
    queryKey: ["surfaces"],
    queryFn: fetchSurfaces,
  });

  const surfaceMutation = useMutation({
    mutationFn: createSurface,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["surfaces"] });
      setEditingManifest(null);
    },
  });

  const surfaces = useMemo(() => {
    const manifests = surfacesQuery.data?.manifests ?? [];
    if (!deferredFilter) {
      return manifests;
    }
    return manifests.filter((manifest) =>
      [manifest.metadata.name, manifest.metadata.namespace, manifest.metadata.description]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(deferredFilter)),
    );
  }, [surfacesQuery.data?.manifests, deferredFilter]);

  const metrics = [
    { label: "Surfaces", value: surfacesQuery.data?.manifests.length ?? 0 },
    {
      label: "Reference",
      value:
        surfacesQuery.data?.manifests.filter(
          (manifest) => manifest.metadata.labels?.["yggdrasil.io/surface-reference"] === "true",
        ).length ?? 0,
    },
    {
      label: "Custom",
      value:
        surfacesQuery.data?.manifests.filter(
          (manifest) => manifest.metadata.labels?.["yggdrasil.io/surface-reference"] !== "true",
        ).length ?? 0,
    },
    {
      label: "Current view",
      value: surfaces.length,
    },
  ];

  return (
    <div className="page-stack">
      <section className="hero-panel">
        <div>
          <span className="eyebrow">Surface control</span>
          <h1>Register and evolve replaceable APIs, auth flows, and consoles straight in the core.</h1>
          <p>
            Surfaces are edge runtimes, not the heart of the product. This page keeps the authoring
            structured, so teams can replace or split surfaces without turning them into hidden
            one-off code paths.
          </p>
        </div>
        <div className="hero-panel__badge">
          <span>Integration rule</span>
          <strong>Surface → Core → Integration</strong>
        </div>
      </section>

      {source === "mock" ? (
        <div className="note-banner">
          Read operations are using mock data. Creating or editing surfaces requires a live
          yggdrasil-core.
        </div>
      ) : null}

      <section className="metric-grid">
        {metrics.map((metric) => (
          <article className="metric-card" key={metric.label}>
            <span className="eyebrow">{metric.label}</span>
            <strong>{metric.value}</strong>
          </article>
        ))}
      </section>

      <section className="section-card">
        <div className="section-card__header">
          <div>
            <span className="eyebrow">Surface index</span>
            <h2>Reference and custom edge runtimes</h2>
            <p>
              Existing surfaces stay visible and editable, while the composer on the right publishes
              new manifest versions without dropping into raw JSON.
            </p>
          </div>
          <label className="search-field">
            <span>Filter</span>
            <input
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              placeholder="api, console, auth..."
            />
          </label>
        </div>

        {surfacesQuery.isLoading ? (
          <div className="state-card">Loading surfaces from yggdrasil-core...</div>
        ) : surfacesQuery.isError ? (
          <div className="state-card state-card--danger">
            Failed to load surfaces from yggdrasil-core.
          </div>
        ) : (
          <div className="workspace-grid workspace-grid--two">
            <section className="section-panel">
              <div className="section-panel__header">
                <div>
                  <span className="eyebrow">Registered surfaces</span>
                  <h3>Edge runtimes in the core</h3>
                </div>
              </div>
              <div className="entity-list">
                {surfaces.map((manifest) => (
                  <SurfaceCard
                    key={manifest.id}
                    manifest={manifest}
                    onEdit={() => setEditingManifest(manifest)}
                  />
                ))}
              </div>
            </section>

            <section className="section-panel">
              <SurfaceComposer
                manifest={editingManifest}
                pending={surfaceMutation.isPending}
                onCancelEdit={() => setEditingManifest(null)}
                onSubmit={(payload) => surfaceMutation.mutateAsync(payload)}
              />
            </section>
          </div>
        )}
      </section>
    </div>
  );
}

interface SurfaceComposerProps {
  manifest: ManifestRecord | null;
  pending: boolean;
  onCancelEdit: () => void;
  onSubmit: (payload: CreateManifestPayload) => Promise<unknown>;
}

function SurfaceComposer({ manifest, pending, onCancelEdit, onSubmit }: SurfaceComposerProps) {
  const [form, setForm] = useState<SurfaceFormState>(defaultSurfaceFormState);
  const [submissionError, setSubmissionError] = useState<string | null>(null);

  useEffect(() => {
    setForm(manifest ? surfaceFormStateFromManifest(manifest) : defaultSurfaceFormState());
    setSubmissionError(null);
  }, [manifest]);

  async function submit() {
    try {
      setSubmissionError(null);
      await onSubmit(surfacePayloadFromFormState(form));
      setForm(defaultSurfaceFormState());
    } catch (error) {
      setSubmissionError(error instanceof Error ? error.message : "Failed to save surface.");
    }
  }

  function updateCapability(
    index: number,
    patch: Partial<SurfaceCapabilityFormState>,
  ) {
    setForm((current) => ({
      ...current,
      capabilities: current.capabilities.map((capability, capabilityIndex) =>
        capabilityIndex === index ? { ...capability, ...patch } : capability,
      ),
    }));
  }

  function addCapability() {
    setForm((current) => ({
      ...current,
      capabilities: [
        ...current.capabilities,
        {
          name: "",
          kind: "endpoint",
          audience: "internal",
          path: "/",
          methods: "GET",
        },
      ],
    }));
  }

  function removeCapability(index: number) {
    setForm((current) => ({
      ...current,
      capabilities: current.capabilities.filter((_, capabilityIndex) => capabilityIndex !== index),
    }));
  }

  const editing = manifest !== null;

  return (
    <div className="form-stack">
      <div className="section-panel__header">
        <div>
          <span className="eyebrow">{editing ? "Edit surface" : "Create surface"}</span>
          <h3>{editing ? "Publish a new surface version" : "Structured surface authoring"}</h3>
          <p>
            This form stays close to the `surface` manifest, but keeps the common fields explicit
            so we do not need raw JSON for normal authoring.
          </p>
        </div>
        {editing ? (
          <button className="button button--ghost" onClick={onCancelEdit}>
            New surface
          </button>
        ) : null}
      </div>

      {editing ? (
        <div className="note-banner">
          Saving this form creates a new manifest version for{" "}
          <strong>
            {form.namespace}/{form.name}
          </strong>
          .
        </div>
      ) : null}

      <div className="form-grid">
        <label className="field">
          <span>Name</span>
          <input
            value={form.name}
            onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
          />
        </label>
        <label className="field">
          <span>Namespace</span>
          <input
            value={form.namespace}
            onChange={(event) =>
              setForm((current) => ({ ...current, namespace: event.target.value }))
            }
          />
        </label>
      </div>

      <label className="field">
        <span>Description</span>
        <textarea
          rows={3}
          value={form.description}
          onChange={(event) =>
            setForm((current) => ({ ...current, description: event.target.value }))
          }
        />
      </label>

      <div className="form-grid">
        <label className="field">
          <span>Category</span>
          <select
            value={form.category}
            onChange={(event) =>
              setForm((current) => ({ ...current, category: event.target.value }))
            }
          >
            {["api", "auth", "console", "bff", "webhook", "gateway"].map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>Integration binding</span>
          <select
            value={form.integrationBinding}
            onChange={(event) =>
              setForm((current) => ({ ...current, integrationBinding: event.target.value }))
            }
          >
            <option value="core_only">core_only</option>
          </select>
        </label>
      </div>

      <div className="form-grid">
        <label className="field">
          <span>Owners</span>
          <input
            value={form.owners}
            onChange={(event) => setForm((current) => ({ ...current, owners: event.target.value }))}
            placeholder="team:platform, team:security"
          />
        </label>
        <label className="field">
          <span>Replaces</span>
          <input
            value={form.replaces}
            onChange={(event) =>
              setForm((current) => ({ ...current, replaces: event.target.value }))
            }
            placeholder="api, auth, console"
          />
        </label>
      </div>

      <div className="form-grid">
        <label className="field">
          <span>Runtime kind</span>
          <select
            value={form.runtimeKind}
            onChange={(event) =>
              setForm((current) => ({ ...current, runtimeKind: event.target.value }))
            }
          >
            {["http_api", "web_ui", "worker", "hybrid"].map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>Exposure</span>
          <select
            value={form.exposure}
            onChange={(event) =>
              setForm((current) => ({ ...current, exposure: event.target.value }))
            }
          >
            {["internal", "operator", "collaborator", "public"].map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="form-grid">
        <label className="field">
          <span>Port</span>
          <input
            type="number"
            value={form.port}
            onChange={(event) => setForm((current) => ({ ...current, port: event.target.value }))}
          />
        </label>
        <label className="field">
          <span>Core contracts</span>
          <input
            value={form.coreContracts}
            onChange={(event) =>
              setForm((current) => ({ ...current, coreContracts: event.target.value }))
            }
            placeholder="authorization, workflow, product"
          />
        </label>
      </div>

      <div className="form-grid">
        <label className="field">
          <span>Base path</span>
          <input
            value={form.basePath}
            onChange={(event) =>
              setForm((current) => ({ ...current, basePath: event.target.value }))
            }
            placeholder="/api/v1"
          />
        </label>
        <label className="field">
          <span>Health path</span>
          <input
            value={form.healthPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, healthPath: event.target.value }))
            }
            placeholder="/healthz"
          />
        </label>
      </div>

      <label className="field">
        <span>Labels</span>
        <textarea
          rows={4}
          value={form.labels}
          onChange={(event) => setForm((current) => ({ ...current, labels: event.target.value }))}
          placeholder={"key=value\nanother=value"}
        />
      </label>

      <div className="subform-list">
        <div className="section-panel__header">
          <div>
            <span className="eyebrow">Capabilities</span>
            <h4>What this surface exposes</h4>
          </div>
          <button className="button button--ghost" onClick={addCapability}>
            Add capability
          </button>
        </div>

        {form.capabilities.map((capability, index) => (
          <div className="subform-card" key={`${capability.name}-${index}`}>
            <div className="subform-card__header">
              <strong>{capability.name || `Capability ${index + 1}`}</strong>
              {form.capabilities.length > 1 ? (
                <button className="button button--ghost" onClick={() => removeCapability(index)}>
                  Remove
                </button>
              ) : null}
            </div>
            <div className="form-grid">
              <label className="field">
                <span>Name</span>
                <input
                  value={capability.name}
                  onChange={(event) => updateCapability(index, { name: event.target.value })}
                />
              </label>
              <label className="field">
                <span>Kind</span>
                <select
                  value={capability.kind}
                  onChange={(event) => updateCapability(index, { kind: event.target.value })}
                >
                  {["endpoint", "auth_flow", "ui_area", "webhook", "rpc"].map((option) => (
                    <option key={option} value={option}>
                      {option}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <div className="form-grid">
              <label className="field">
                <span>Audience</span>
                <select
                  value={capability.audience}
                  onChange={(event) => updateCapability(index, { audience: event.target.value })}
                >
                  {["internal", "operator", "collaborator", "service", "public"].map((option) => (
                    <option key={option} value={option}>
                      {option}
                    </option>
                  ))}
                </select>
              </label>
              <label className="field">
                <span>Methods</span>
                <input
                  value={capability.methods}
                  onChange={(event) => updateCapability(index, { methods: event.target.value })}
                  placeholder="GET, POST"
                />
              </label>
            </div>
            <label className="field">
              <span>Path</span>
              <input
                value={capability.path}
                onChange={(event) => updateCapability(index, { path: event.target.value })}
                placeholder="/api/v1/example"
              />
            </label>
          </div>
        ))}
      </div>

      {submissionError ? <div className="state-card state-card--danger">{submissionError}</div> : null}

      <div className="form-actions">
        <button className="button" onClick={submit} disabled={pending || !form.name.trim()}>
          {pending ? "Saving..." : editing ? "Publish new version" : "Create surface"}
        </button>
      </div>
    </div>
  );
}

function SurfaceCard({
  manifest,
  onEdit,
}: {
  manifest: ManifestRecord;
  onEdit: () => void;
}) {
  const spec = surfaceSpecFromUnknown(manifest.spec);
  const isReference = manifest.metadata.labels?.["yggdrasil.io/surface-reference"] === "true";

  return (
    <article className="entity-card">
      <div className="entity-card__header">
        <div>
          <h3>{manifest.metadata.name}</h3>
          <p>
            {spec.category || "surface"} / {manifest.metadata.namespace}
          </p>
        </div>
        <div className="entity-card__actions">
          <StatusPill status={manifest.metadata.active ? "active" : "disabled"} />
          <button className="button button--ghost" onClick={onEdit}>
            Edit
          </button>
        </div>
      </div>

      <div className="entity-card__meta">
        <span>{manifest.metadata.description || "No description yet"}</span>
        <span>
          {spec.runtime.kind} / {spec.runtime.exposure || "internal"} / port{" "}
          {spec.runtime.port ?? "n/a"}
        </span>
      </div>

      <div className="token-row">
        <span className="token">{isReference ? "reference" : "custom"}</span>
        <span className="token">{spec.integration_binding || "core_only"}</span>
        {(spec.core_contracts ?? []).map((contract) => (
          <span className="token" key={contract}>
            {contract}
          </span>
        ))}
      </div>

      <div className="entity-card__meta">
        <span>{(spec.capabilities ?? []).length} capabilities</span>
        <span>v{manifest.version}</span>
      </div>
    </article>
  );
}

function surfaceFormStateFromManifest(manifest: ManifestRecord): SurfaceFormState {
  const spec = surfaceSpecFromUnknown(manifest.spec);

  return {
    name: manifest.metadata.name,
    namespace: manifest.metadata.namespace,
    description: manifest.metadata.description ?? "",
    labels: stringifyLabels(manifest.metadata.labels),
    category: spec.category || "api",
    owners: joinList(spec.owners),
    replaces: joinList(spec.replaces),
    integrationBinding: spec.integration_binding || "core_only",
    runtimeKind: spec.runtime.kind || "http_api",
    exposure: spec.runtime.exposure || "internal",
    port: spec.runtime.port ? String(spec.runtime.port) : "",
    basePath: spec.runtime.base_path || "",
    healthPath: spec.runtime.health_path || "",
    coreContracts: joinList(spec.core_contracts),
    capabilities: (spec.capabilities ?? []).length
      ? spec.capabilities!.map((capability) => ({
          name: capability.name || "",
          kind: capability.kind || "endpoint",
          audience: capability.audience || "internal",
          path: capability.path || "",
          methods: joinList(capability.methods),
        }))
      : defaultSurfaceFormState().capabilities,
  };
}

function surfacePayloadFromFormState(form: SurfaceFormState): CreateManifestPayload {
  const port = Number(form.port);
  if (!Number.isFinite(port) || port <= 0) {
    throw new Error("Port must be a positive number.");
  }

  const capabilities: SurfaceCapabilitySpec[] = form.capabilities.map((capability, index) => {
    const name = capability.name.trim();
    if (!name) {
      throw new Error(`Capability ${index + 1} requires a name.`);
    }

    const kind = capability.kind.trim();
    const methods = splitList(capability.methods).map((method) => method.toUpperCase());
    const path = capability.path.trim();

    return {
      name,
      kind,
      audience: capability.audience.trim() || undefined,
      path: path || undefined,
      methods: methods.length > 0 ? methods : undefined,
    };
  });

  const spec: SurfaceSpec = {
    category: form.category.trim(),
    owners: splitList(form.owners),
    replaces: splitList(form.replaces),
    integration_binding: form.integrationBinding.trim() || "core_only",
    runtime: {
      kind: form.runtimeKind.trim(),
      exposure: form.exposure.trim() || undefined,
      port,
      base_path: form.basePath.trim() || undefined,
      health_path: form.healthPath.trim() || undefined,
    },
    core_contracts: splitList(form.coreContracts),
    capabilities,
  };

  return {
    name: form.name.trim(),
    namespace: form.namespace.trim(),
    description: form.description.trim(),
    labels: parseLabelsInput(form.labels),
    spec,
  };
}

function surfaceSpecFromUnknown(value: unknown): SurfaceSpec {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return { category: "", runtime: { kind: "" } };
  }

  const spec = value as Record<string, unknown>;
  const runtime = (spec.runtime ?? {}) as Record<string, unknown>;
  const capabilities = Array.isArray(spec.capabilities) ? spec.capabilities : [];

  return {
    category: asString(spec.category),
    owners: asStringArray(spec.owners),
    replaces: asStringArray(spec.replaces),
    integration_binding: asString(spec.integration_binding),
    runtime: {
      kind: asString(runtime.kind),
      exposure: asString(runtime.exposure),
      port: typeof runtime.port === "number" ? runtime.port : undefined,
      base_path: asString(runtime.base_path),
      health_path: asString(runtime.health_path),
    },
    core_contracts: asStringArray(spec.core_contracts),
    capabilities: capabilities.map((entry) => {
      const capability = entry as Record<string, unknown>;
      return {
        name: asString(capability.name),
        kind: asString(capability.kind),
        audience: asString(capability.audience),
        path: asString(capability.path),
        methods: asStringArray(capability.methods),
      };
    }),
  };
}

function parseLabelsInput(value: string): Record<string, string> {
  const output: Record<string, string> = {};

  for (const line of value.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) {
      continue;
    }

    const separatorIndex = trimmed.indexOf("=");
    if (separatorIndex <= 0) {
      throw new Error("Labels must use one key=value pair per line.");
    }

    const key = trimmed.slice(0, separatorIndex).trim();
    const entryValue = trimmed.slice(separatorIndex + 1).trim();
    if (!key || !entryValue) {
      throw new Error("Labels must use one key=value pair per line.");
    }
    output[key] = entryValue;
  }

  return output;
}

function stringifyLabels(labels?: Record<string, string>) {
  if (!labels || Object.keys(labels).length === 0) {
    return "";
  }

  return Object.entries(labels)
    .map(([key, value]) => `${key}=${value}`)
    .join("\n");
}

function splitList(value: string) {
  return value
    .split(",")
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function joinList(value?: string[]) {
  return (value ?? []).join(", ");
}

function asString(value: unknown) {
  return typeof value === "string" ? value : "";
}

function asStringArray(value: unknown) {
  return Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === "string") : [];
}
