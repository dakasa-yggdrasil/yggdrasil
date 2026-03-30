import { useMemo, useState } from "react";
import { PluginEntryCard } from "../components/PluginEntryCard";
import type { IntegrationCatalogDomain, IntegrationCatalogEntry } from "../types";

interface CatalogPageProps {
  domains: IntegrationCatalogDomain[];
  onCreateInstance: (entry: IntegrationCatalogEntry) => void;
}

export function CatalogPage({ domains, onCreateInstance }: CatalogPageProps) {
  const [filter, setFilter] = useState("");

  const filteredDomains = useMemo(() => {
    const lowered = filter.trim().toLowerCase();
    if (!lowered) {
      return domains;
    }

    return domains
      .map((domain) => ({
        ...domain,
        sections: domain.sections
          .map((section) => ({
            ...section,
            entries: section.entries.filter((entry) =>
              [entry.plugin_name, entry.description, entry.provider, entry.section, entry.entry]
                .filter(Boolean)
                .some((value) => value!.toLowerCase().includes(lowered)),
            ),
          }))
          .filter((section) => section.entries.length > 0),
      }))
      .filter((domain) => domain.sections.length > 0);
  }, [domains, filter]);

  return (
    <div className="page-stack">
      <section className="section-card">
        <div className="section-card__header">
          <div>
            <span className="eyebrow">Plugin catalog</span>
            <h2>Browse the ecosystem as domains, not random workers</h2>
          </div>
          <label className="search-field">
            <span>Filter</span>
            <input
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              placeholder="rabbitmq, grafana, github..."
            />
          </label>
        </div>
      </section>

      {filteredDomains.map((domain) => (
        <section className="section-card" key={domain.domain}>
          <div className="section-card__header">
            <div>
              <span className="eyebrow">Domain</span>
              <h2>{domain.domain}</h2>
            </div>
          </div>
          <div className="catalog-section-stack">
            {domain.sections.map((section) => (
              <div className="catalog-section-block" key={`${domain.domain}-${section.name}`}>
                <div className="catalog-section-block__header">
                  <span className="eyebrow">{section.name}</span>
                  <strong>{section.entries.length} entries</strong>
                </div>
                <div className="plugin-grid">
                  {section.entries.map((entry) => (
                    <PluginEntryCard
                      key={`${entry.plugin_name}-${entry.entry}`}
                      entry={entry}
                      onCreateInstance={onCreateInstance}
                    />
                  ))}
                </div>
              </div>
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}
