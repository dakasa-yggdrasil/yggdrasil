import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createIntegrationInstance, fetchIntegrationCatalogEntryDetail } from "../lib/api";
import {
  asIntegrationTypeSpec,
  initializeSchemaValues,
  schemaFields,
  serializeSchemaValues,
  type FormFieldValue,
} from "../lib/schema";
import type { CreateIntegrationInstancePayload, IntegrationCatalogEntry } from "../types";

interface InstanceComposerProps {
  open: boolean;
  selectedEntry: IntegrationCatalogEntry | null;
  onClose: () => void;
}

export function InstanceComposer({ open, selectedEntry, onClose }: InstanceComposerProps) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [namespace, setNamespace] = useState("global");
  const [description, setDescription] = useState("");
  const [owners, setOwners] = useState("team:platform");
  const [status, setStatus] = useState("active");
  const [credentials, setCredentials] = useState<Record<string, FormFieldValue>>({});
  const [config, setConfig] = useState<Record<string, FormFieldValue>>({});
  const [discoveryEnabled, setDiscoveryEnabled] = useState(false);
  const [discoveryMode, setDiscoveryMode] = useState("manual");
  const [syncIntervalSeconds, setSyncIntervalSeconds] = useState("0");
  const [defaultDryRun, setDefaultDryRun] = useState(false);
  const [maxBatchSize, setMaxBatchSize] = useState("25");
  const [submissionError, setSubmissionError] = useState<string | null>(null);

  const detailQuery = useQuery({
    queryKey: ["integration-entry-detail", selectedEntry?.domain, selectedEntry?.section, selectedEntry?.entry],
    queryFn: () =>
      fetchIntegrationCatalogEntryDetail(
        selectedEntry!.domain,
        selectedEntry!.section,
        selectedEntry!.entry,
      ),
    enabled: open && Boolean(selectedEntry),
  });

  const integrationSpec = useMemo(() => {
    if (!detailQuery.data) {
      return null;
    }
    return asIntegrationTypeSpec(detailQuery.data.integrationTypeManifest.spec);
  }, [detailQuery.data]);

  useEffect(() => {
    if (!detailQuery.data || !integrationSpec) {
      return;
    }

    setDescription(detailQuery.data.entry.description ?? "");
    setCredentials(initializeSchemaValues(integrationSpec.credential_schema));
    setConfig(initializeSchemaValues(integrationSpec.instance_schema));
    setDiscoveryMode("manual");
    setDiscoveryEnabled(false);
    setSubmissionError(null);
    setName("");
    setNamespace("global");
  }, [detailQuery.data, integrationSpec]);

  const createMutation = useMutation({
    mutationFn: (payload: CreateIntegrationInstancePayload) => createIntegrationInstance(payload),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["integration-catalog"] });
      onClose();
    },
    onError: (error: Error) => {
      setSubmissionError(error.message);
    },
  });

  if (!open || !selectedEntry) {
    return null;
  }

  const credentialFields = schemaFields(integrationSpec?.credential_schema);
  const configFields = schemaFields(integrationSpec?.instance_schema);

  function updateField(
    group: "credentials" | "config",
    key: string,
    value: FormFieldValue,
  ) {
    if (group === "credentials") {
      setCredentials((current) => ({ ...current, [key]: value }));
      return;
    }
    setConfig((current) => ({ ...current, [key]: value }));
  }

  function submit() {
    if (!detailQuery.data || !integrationSpec) {
      return;
    }

    setSubmissionError(null);

    const payload: CreateIntegrationInstancePayload = {
      name,
      namespace,
      description,
      type_ref: {
        name: detailQuery.data.entry.integration_type.name,
        namespace: detailQuery.data.entry.integration_type.namespace,
      },
      status,
      owners: owners
        .split(",")
        .map((item) => item.trim())
        .filter(Boolean),
      credentials: serializeSchemaValues(integrationSpec.credential_schema, credentials),
      config: serializeSchemaValues(integrationSpec.instance_schema, config),
      discovery: {
        enabled: discoveryEnabled,
        mode: discoveryMode || "manual",
        sync_interval_seconds: Number.parseInt(syncIntervalSeconds || "0", 10) || 0,
      },
      execution: {
        default_dry_run: defaultDryRun,
        max_batch_size: Number.parseInt(maxBatchSize || "0", 10) || 0,
      },
    };

    createMutation.mutate(payload);
  }

  return (
    <div className="composer-backdrop">
      <div className="composer-panel">
        <div className="composer-panel__header">
          <div>
            <span className="eyebrow">Create integration instance</span>
            <h2>{selectedEntry.plugin_name}</h2>
            <p>
              Feeding the core through the console now creates a real{" "}
              <code>integration_instance</code> manifest.
            </p>
          </div>
          <button className="button button--ghost" onClick={onClose}>
            Close
          </button>
        </div>

        {detailQuery.isLoading ? (
          <div className="state-card">Loading plugin schema...</div>
        ) : detailQuery.isError ? (
          <div className="state-card state-card--danger">
            Failed to load the plugin schema.
          </div>
        ) : (
          <div className="composer-grid">
            <section className="composer-section">
              <h3>Instance identity</h3>
              <label className="field">
                <span>Name</span>
                <input value={name} onChange={(event) => setName(event.target.value)} placeholder="grafana-platform-api" />
              </label>
              <label className="field">
                <span>Namespace</span>
                <input
                  value={namespace}
                  onChange={(event) => setNamespace(event.target.value)}
                  placeholder="global"
                />
              </label>
              <label className="field">
                <span>Description</span>
                <textarea
                  rows={3}
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                  placeholder="Primary runtime instance for platform operations."
                />
              </label>
              <label className="field">
                <span>Owners</span>
                <input
                  value={owners}
                  onChange={(event) => setOwners(event.target.value)}
                  placeholder="team:platform, collaborator:ana"
                />
              </label>
              <label className="field">
                <span>Status</span>
                <select value={status} onChange={(event) => setStatus(event.target.value)}>
                  <option value="active">active</option>
                  <option value="draft">draft</option>
                  <option value="disabled">disabled</option>
                </select>
              </label>
            </section>

            <section className="composer-section">
              <h3>Credentials schema</h3>
              {credentialFields.length === 0 ? (
                <div className="empty-inline">This plugin does not require stored credentials.</div>
              ) : (
                credentialFields.map((field) => (
                  <SchemaInput
                    key={field.key}
                    name={field.key}
                    value={credentials[field.key]}
                    onChange={(value) => updateField("credentials", field.key, value)}
                    type={field.property.type}
                    isSecret={Boolean(field.property.secret)}
                    description={field.property.description}
                    enumValues={field.property.enum}
                    required={field.required}
                  />
                ))
              )}
            </section>

            <section className="composer-section">
              <h3>Instance config schema</h3>
              {configFields.length === 0 ? (
                <div className="empty-inline">This plugin does not expose instance config fields.</div>
              ) : (
                configFields.map((field) => (
                  <SchemaInput
                    key={field.key}
                    name={field.key}
                    value={config[field.key]}
                    onChange={(value) => updateField("config", field.key, value)}
                    type={field.property.type}
                    isSecret={Boolean(field.property.secret)}
                    description={field.property.description}
                    enumValues={field.property.enum}
                    required={field.required}
                  />
                ))
              )}
            </section>

            <section className="composer-section">
              <h3>Runtime behavior</h3>
              <label className="field field--inline">
                <span>Discovery enabled</span>
                <input
                  type="checkbox"
                  checked={discoveryEnabled}
                  onChange={(event) => setDiscoveryEnabled(event.target.checked)}
                />
              </label>
              <label className="field">
                <span>Discovery mode</span>
                <input
                  value={discoveryMode}
                  onChange={(event) => setDiscoveryMode(event.target.value)}
                  placeholder="manual"
                />
              </label>
              <label className="field">
                <span>Sync interval seconds</span>
                <input
                  type="number"
                  min="0"
                  value={syncIntervalSeconds}
                  onChange={(event) => setSyncIntervalSeconds(event.target.value)}
                />
              </label>
              <label className="field field--inline">
                <span>Default dry run</span>
                <input
                  type="checkbox"
                  checked={defaultDryRun}
                  onChange={(event) => setDefaultDryRun(event.target.checked)}
                />
              </label>
              <label className="field">
                <span>Max batch size</span>
                <input
                  type="number"
                  min="0"
                  value={maxBatchSize}
                  onChange={(event) => setMaxBatchSize(event.target.value)}
                />
              </label>
            </section>
          </div>
        )}

        {submissionError ? <div className="state-card state-card--danger">{submissionError}</div> : null}

        <div className="composer-panel__footer">
          <button className="button button--ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="button"
            onClick={submit}
            disabled={createMutation.isPending || detailQuery.isLoading || !name.trim()}
          >
            {createMutation.isPending ? "Creating..." : "Create integration instance"}
          </button>
        </div>
      </div>
    </div>
  );
}

interface SchemaInputProps {
  name: string;
  value: FormFieldValue | undefined;
  onChange: (value: FormFieldValue) => void;
  type: string;
  isSecret?: boolean;
  description?: string;
  enumValues?: unknown[];
  required?: boolean;
}

function SchemaInput({
  name,
  value,
  onChange,
  type,
  isSecret,
  description,
  enumValues,
  required,
}: SchemaInputProps) {
  const label = (
    <span>
      {name}
      {required ? " *" : ""}
    </span>
  );

  if (type === "boolean") {
    return (
      <label className="field field--inline">
        {label}
        <input
          type="checkbox"
          checked={Boolean(value)}
          onChange={(event) => onChange(event.target.checked)}
        />
      </label>
    );
  }

  if (enumValues && enumValues.length > 0) {
    return (
      <label className="field">
        {label}
        <select value={String(value ?? "")} onChange={(event) => onChange(event.target.value)}>
          <option value="">Select one value</option>
          {enumValues.map((option) => (
            <option key={String(option)} value={String(option)}>
              {String(option)}
            </option>
          ))}
        </select>
        {description ? <small>{description}</small> : null}
      </label>
    );
  }

  if (type === "array") {
    return (
      <label className="field">
        {label}
        <textarea
          rows={4}
          value={String(value ?? "[]")}
          onChange={(event) => onChange(event.target.value)}
          placeholder='["value-a", "value-b"]'
        />
        {description ? <small>{description}</small> : null}
      </label>
    );
  }

  return (
    <label className="field">
      {label}
      <input
        type={isSecret ? "password" : type === "integer" || type === "number" ? "number" : "text"}
        value={String(value ?? "")}
        onChange={(event) => onChange(event.target.value)}
      />
      {description ? <small>{description}</small> : null}
    </label>
  );
}
