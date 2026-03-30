import type { IntegrationCatalogDomain, IntegrationCatalogEntry } from "../types";
import { StatusPill } from "../components/StatusPill";

interface OverviewPageProps {
  domains: IntegrationCatalogDomain[];
  source: "live" | "mock";
}

export function OverviewPage({ domains, source }: OverviewPageProps) {
  const entries = domains.flatMap((domain) =>
    domain.sections.flatMap((section) => section.entries),
  );
  const instances = entries.flatMap((entry) => entry.instances ?? []);

  const metrics = [
    { label: "Domains", value: domains.length },
    { label: "Plugins", value: entries.length },
    { label: "Instances", value: instances.length },
    {
      label: "Healthy entries",
      value: entries.filter((entry) => entry.status === "healthy").length,
    },
  ];

  return (
    <div className="page-stack">
      <section className="hero-panel">
        <div>
          <span className="eyebrow">Yggdrasil control plane</span>
          <h1>Ecosystem administration that feels composable, not hardcoded.</h1>
          <p>
            The console is now a real front door for plugin governance. It reads from the
            explicit catalog, surfaces health, and can create integration instances through the
            core-backed HTTP API.
          </p>
        </div>
        <div className="hero-panel__badge">
          <span>Mode</span>
          <strong>{source === "live" ? "Live core-backed" : "Demo fallback"}</strong>
        </div>
      </section>

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
            <span className="eyebrow">Current domains</span>
            <h2>What the console already sees</h2>
          </div>
        </div>
        <div className="overview-domain-grid">
          {domains.map((domain) => (
            <article className="overview-domain-card" key={domain.domain}>
              <div className="overview-domain-card__head">
                <h3>{domain.domain}</h3>
                <span>{domain.sections.length} sections</span>
              </div>
              <div className="overview-domain-card__body">
                {domain.sections.map((section) => (
                  <SectionPreview key={`${domain.domain}-${section.name}`} name={section.name} entries={section.entries} />
                ))}
              </div>
            </article>
          ))}
        </div>
      </section>
    </div>
  );
}

function SectionPreview({ name, entries }: { name: string; entries: IntegrationCatalogEntry[] }) {
  return (
    <div className="section-preview">
      <div className="section-preview__header">
        <span className="eyebrow">{name}</span>
        <strong>{entries.length}</strong>
      </div>
      <div className="section-preview__items">
        {entries.map((entry) => (
          <div className="section-preview__item" key={`${entry.plugin_name}-${entry.entry}`}>
            <span>{entry.plugin_name}</span>
            <StatusPill status={entry.status} />
          </div>
        ))}
      </div>
    </div>
  );
}
