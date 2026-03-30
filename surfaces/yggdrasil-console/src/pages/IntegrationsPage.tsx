import { useMemo } from "react";
import { StatusPill } from "../components/StatusPill";
import type { IntegrationCatalogDomain, IntegrationCatalogEntry } from "../types";

interface IntegrationsPageProps {
  domains: IntegrationCatalogDomain[];
  onCreateInstance: (entry: IntegrationCatalogEntry) => void;
}

interface Row {
  domain: string;
  plugin: IntegrationCatalogEntry;
  instanceName: string;
  instanceDescription?: string;
  status: string;
  owners: string[];
}

export function IntegrationsPage({ domains, onCreateInstance }: IntegrationsPageProps) {
  const rows = useMemo<Row[]>(() => {
    return domains.flatMap((domain) =>
      domain.sections.flatMap((section) =>
        section.entries.flatMap((entry) => {
          if (!entry.instances?.length) {
            return [
              {
                domain: domain.domain,
                plugin: entry,
                instanceName: "No instance configured",
                instanceDescription: undefined,
                status: "unconfigured",
                owners: [],
              },
            ];
          }

          return entry.instances.map((instance) => ({
            domain: domain.domain,
            plugin: entry,
            instanceName: instance.integration_instance.name,
            instanceDescription: instance.description,
            status: instance.status,
            owners: instance.owners ?? [],
          }));
        }),
      ),
    );
  }, [domains]);

  return (
    <div className="page-stack">
      <section className="section-card">
        <div className="section-card__header">
          <div>
            <span className="eyebrow">Integration instances</span>
            <h2>Where governance becomes concrete</h2>
            <p>
              This view is for the configured instances underneath each plugin entry. New
              instances are created as manifests in the core.
            </p>
          </div>
        </div>
        <div className="instance-table">
          <div className="instance-table__head">
            <span>Domain</span>
            <span>Plugin</span>
            <span>Instance</span>
            <span>Status</span>
            <span>Owners</span>
            <span>Action</span>
          </div>
          {rows.map((row) => (
            <div
              className="instance-table__row"
              key={`${row.plugin.plugin_name}-${row.instanceName}-${row.domain}`}
            >
              <span>{row.domain}</span>
              <span>
                {row.plugin.plugin_name}
                <small>{row.plugin.section}</small>
              </span>
              <span>
                {row.instanceName}
                <small>{row.instanceDescription || row.plugin.description || "Pending configuration"}</small>
              </span>
              <span>
                <StatusPill status={row.status} />
              </span>
              <span>{row.owners.length > 0 ? row.owners.join(", ") : "Unassigned"}</span>
              <span>
                <button className="button button--secondary" onClick={() => onCreateInstance(row.plugin)}>
                  {row.instanceName === "No instance configured" ? "Configure" : "New instance"}
                </button>
              </span>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}
