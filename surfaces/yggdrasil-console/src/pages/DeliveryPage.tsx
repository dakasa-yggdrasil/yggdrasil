import { useDeferredValue, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createProduct,
  createWorkflow,
  fetchProducts,
  fetchWorkflows,
  type DataSourceMode,
} from "../lib/api";
import { parseObjectInput, parseStringMapInput, prettyJSON } from "../lib/json";
import type { CreateManifestPayload, ManifestRecord } from "../types";

interface DeliveryPageProps {
  source: DataSourceMode;
}

const defaultProductSpec = {
  category: "platform",
  class: "service",
  owners: ["team:platform"],
  lifecycle: {
    stage: "development",
  },
  components: [
    {
      name: "sample-component",
      source: {
        kind: "inline",
        objects: [
          {
            apiVersion: "v1",
            kind: "Namespace",
            metadata: {
              name: "sample-product",
            },
          },
        ],
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
        namespace: "sample-product",
      },
      reconcile: {
        strategy: "apply",
        prune: true,
      },
    },
  ],
};

const defaultWorkflowSpec = {
  trigger: {
    mode: "manual",
  },
  input_schema: {
    required: ["repository", "workflow"],
    properties: {
      repository: {
        type: "string",
        description: "GitHub repository in owner/name form.",
      },
      workflow: {
        type: "string",
        description: "GitHub Actions workflow file name.",
      },
      ref: {
        type: "string",
        description: "Git reference used for dispatch.",
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
};

export function DeliveryPage({ source }: DeliveryPageProps) {
  const queryClient = useQueryClient();
  const [filter, setFilter] = useState("");
  const deferredFilter = useDeferredValue(filter.trim().toLowerCase());

  const productsQuery = useQuery({
    queryKey: ["products"],
    queryFn: fetchProducts,
  });

  const workflowsQuery = useQuery({
    queryKey: ["workflows"],
    queryFn: fetchWorkflows,
  });

  const productMutation = useMutation({
    mutationFn: createProduct,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["products"] });
    },
  });

  const workflowMutation = useMutation({
    mutationFn: createWorkflow,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["workflows"] });
    },
  });

  const products = useMemo(() => {
    const manifests = productsQuery.data?.manifests ?? [];
    if (!deferredFilter) {
      return manifests;
    }
    return manifests.filter((manifest) =>
      [manifest.metadata.name, manifest.metadata.namespace, manifest.metadata.description]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(deferredFilter)),
    );
  }, [productsQuery.data?.manifests, deferredFilter]);

  const workflows = useMemo(() => {
    const manifests = workflowsQuery.data?.manifests ?? [];
    if (!deferredFilter) {
      return manifests;
    }
    return manifests.filter((manifest) =>
      [manifest.metadata.name, manifest.metadata.namespace, manifest.metadata.description]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(deferredFilter)),
    );
  }, [workflowsQuery.data?.manifests, deferredFilter]);

  const loading = productsQuery.isLoading || workflowsQuery.isLoading;
  const failed = productsQuery.isError || workflowsQuery.isError;

  const metrics = [
    { label: "Products", value: productsQuery.data?.manifests.length ?? 0 },
    { label: "Workflows", value: workflowsQuery.data?.manifests.length ?? 0 },
    {
      label: "Active records",
      value:
        (productsQuery.data?.manifests.filter((manifest) => manifest.metadata.active).length ?? 0) +
        (workflowsQuery.data?.manifests.filter((manifest) => manifest.metadata.active).length ?? 0),
    },
    {
      label: "Current view",
      value: products.length + workflows.length,
    },
  ];

  return (
    <div className="page-stack">
      <section className="hero-panel">
        <div>
          <span className="eyebrow">Delivery control</span>
          <h1>Products and workflows stop living in scattered repos and start living in the core.</h1>
          <p>
            This view is intentionally dual-purpose: operational lists for scanning what already
            exists, and advanced composers for defining new delivery behavior straight into the
            control plane.
          </p>
        </div>
        <div className="hero-panel__badge">
          <span>Composition mode</span>
          <strong>{source === "live" ? "Core-backed authoring" : "Read-only demo fallback"}</strong>
        </div>
      </section>

      {source === "mock" ? (
        <div className="note-banner">
          Read operations are using mock data. New products and workflows need a live
          yggdrasil-core to be created.
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
            <span className="eyebrow">Delivery index</span>
            <h2>Products and orchestration</h2>
            <p>
              The forms below stay intentionally close to manifest shape, because these two kinds
              still benefit from advanced control.
            </p>
          </div>
          <label className="search-field">
            <span>Filter</span>
            <input
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              placeholder="github-dispatch, rabbitmq-platform..."
            />
          </label>
        </div>

        {loading ? (
          <div className="state-card">Loading products and workflows...</div>
        ) : failed ? (
          <div className="state-card state-card--danger">
            Failed to load delivery manifests from yggdrasil-core.
          </div>
        ) : (
          <div className="workspace-grid workspace-grid--two">
            <section className="section-panel">
              <div className="section-panel__header">
                <div>
                  <span className="eyebrow">Products</span>
                  <h3>Delivery composition</h3>
                </div>
              </div>
              <ManifestComposer
                title="Create product"
                description="Start from a valid raw_k8s baseline and tune the spec only where the product actually differs."
                defaultName="sample-product"
                defaultNamespace="global"
                defaultDescription="Sample product authored from the console."
                defaultLabels={prettyJSON({ surface: "console", kind: "product" })}
                defaultSpec={prettyJSON(defaultProductSpec)}
                actionLabel="Create product"
                pending={productMutation.isPending}
                onSubmit={(payload) => productMutation.mutateAsync(payload)}
              />
              <div className="entity-list">
                {products.map((manifest) => (
                  <ManifestCard key={manifest.id} manifest={manifest} />
                ))}
              </div>
            </section>

            <section className="section-panel">
              <div className="section-panel__header">
                <div>
                  <span className="eyebrow">Workflows</span>
                  <h3>Orchestration manifests</h3>
                </div>
              </div>
              <ManifestComposer
                title="Create workflow"
                description="Use a real integration step as the starting point and evolve the workflow from there."
                defaultName="sample-workflow"
                defaultNamespace="global"
                defaultDescription="Sample workflow authored from the console."
                defaultLabels={prettyJSON({ surface: "console", kind: "workflow" })}
                defaultSpec={prettyJSON(defaultWorkflowSpec)}
                actionLabel="Create workflow"
                pending={workflowMutation.isPending}
                onSubmit={(payload) => workflowMutation.mutateAsync(payload)}
              />
              <div className="entity-list">
                {workflows.map((manifest) => (
                  <ManifestCard key={manifest.id} manifest={manifest} />
                ))}
              </div>
            </section>
          </div>
        )}
      </section>
    </div>
  );
}

interface ManifestComposerProps {
  title: string;
  description: string;
  defaultName: string;
  defaultNamespace: string;
  defaultDescription: string;
  defaultLabels: string;
  defaultSpec: string;
  actionLabel: string;
  pending: boolean;
  onSubmit: (payload: CreateManifestPayload) => Promise<unknown>;
}

function ManifestComposer({
  title,
  description,
  defaultName,
  defaultNamespace,
  defaultDescription,
  defaultLabels,
  defaultSpec,
  actionLabel,
  pending,
  onSubmit,
}: ManifestComposerProps) {
  const [name, setName] = useState(defaultName);
  const [namespace, setNamespace] = useState(defaultNamespace);
  const [manifestDescription, setManifestDescription] = useState(defaultDescription);
  const [labels, setLabels] = useState(defaultLabels);
  const [spec, setSpec] = useState(defaultSpec);
  const [submissionError, setSubmissionError] = useState<string | null>(null);

  async function submit() {
    try {
      setSubmissionError(null);
      await onSubmit({
        name,
        namespace,
        description: manifestDescription,
        labels: parseStringMapInput(labels, "labels"),
        spec: parseObjectInput(spec, "manifest spec"),
      });
      setName(defaultName);
      setNamespace(defaultNamespace);
      setManifestDescription(defaultDescription);
    } catch (error) {
      setSubmissionError(error instanceof Error ? error.message : "Failed to create manifest.");
    }
  }

  return (
    <div className="form-stack">
      <div>
        <h4>{title}</h4>
        <p>{description}</p>
      </div>

      <div className="form-grid">
        <label className="field">
          <span>Name</span>
          <input value={name} onChange={(event) => setName(event.target.value)} />
        </label>
        <label className="field">
          <span>Namespace</span>
          <input value={namespace} onChange={(event) => setNamespace(event.target.value)} />
        </label>
      </div>

      <label className="field">
        <span>Description</span>
        <textarea
          rows={3}
          value={manifestDescription}
          onChange={(event) => setManifestDescription(event.target.value)}
        />
      </label>

      <label className="field">
        <span>Labels JSON</span>
        <textarea rows={5} value={labels} onChange={(event) => setLabels(event.target.value)} />
      </label>

      <label className="field">
        <span>Spec JSON</span>
        <textarea rows={18} value={spec} onChange={(event) => setSpec(event.target.value)} />
      </label>

      {submissionError ? <div className="state-card state-card--danger">{submissionError}</div> : null}

      <div className="form-actions">
        <button className="button" onClick={submit} disabled={pending || !name.trim()}>
          {pending ? "Saving..." : actionLabel}
        </button>
      </div>
    </div>
  );
}

function ManifestCard({ manifest }: { manifest: ManifestRecord }) {
  const specSummary = summarizeManifest(manifest);

  return (
    <article className="entity-card">
      <div className="entity-card__header">
        <div>
          <h3>{manifest.metadata.name}</h3>
          <p>
            {manifest.kind} / {manifest.metadata.namespace}
          </p>
        </div>
        <span className="plugin-card__version">v{manifest.version}</span>
      </div>
      <div className="entity-card__meta">
        <span>{manifest.metadata.description || "No description yet"}</span>
        <span>{specSummary}</span>
      </div>
      {manifest.metadata.labels && Object.keys(manifest.metadata.labels).length > 0 ? (
        <div className="token-row">
          {Object.entries(manifest.metadata.labels).map(([key, value]) => (
            <span className="token" key={key}>
              {key}:{value}
            </span>
          ))}
        </div>
      ) : null}
    </article>
  );
}

function summarizeManifest(manifest: ManifestRecord) {
  const spec = manifest.spec as Record<string, unknown> | null;
  if (!spec || typeof spec !== "object" || Array.isArray(spec)) {
    return "No summary available";
  }

  if (manifest.kind === "product") {
    const components = Array.isArray(spec.components) ? spec.components.length : 0;
    return `${components} components`;
  }

  if (manifest.kind === "workflow") {
    const steps = Array.isArray(spec.steps) ? spec.steps.length : 0;
    return `${steps} steps`;
  }

  return "Manifest";
}
