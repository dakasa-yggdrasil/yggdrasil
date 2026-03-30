import { useDeferredValue, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchCatalogDiscovery } from "../lib/api";
import { StatusPill } from "../components/StatusPill";
import type { CatalogDiscoveryItem } from "../types";

interface DiscoverPageProps {
  source: "live" | "mock";
}

export function DiscoverPage({ source }: DiscoverPageProps) {
  const [filter, setFilter] = useState("");
  const deferredFilter = useDeferredValue(filter);

  const discoveryQuery = useQuery({
    queryKey: ["catalog-discovery"],
    queryFn: fetchCatalogDiscovery,
  });
  const discoverySource = discoveryQuery.data?.source ?? source;

  const filteredItems = useMemo(() => {
    const items = discoveryQuery.data?.items ?? [];
    const query = deferredFilter.trim().toLowerCase();
    if (!query) {
      return items;
    }

    return items.filter((item) =>
      [
        item.kind,
        item.name,
        item.display_name,
        item.description,
        item.domain,
        item.section,
        item.entry,
        item.repository,
      ]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(query)),
    );
  }, [deferredFilter, discoveryQuery.data?.items]);

  const sources = discoveryQuery.data?.sources ?? [];

  return (
    <div className="page-stack">
      <section className="section-card">
        <div className="section-card__header">
          <div>
            <span className="eyebrow">Discovery</span>
            <h2>What discovery-capable integrations can already see</h2>
            <p>
              Discovery stays optional and provider-agnostic. The console reads candidates from the
              core, while the core decides which integration instances are allowed to act as
              discovery sources.
            </p>
          </div>
          <label className="search-field">
            <span>Filter</span>
            <input
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              placeholder="surface, rabbitmq, github..."
            />
          </label>
        </div>
        {discoverySource === "mock" ? (
          <div className="inline-banner">
            Discovery is using mock data because yggdrasil-core is unavailable in dev.
          </div>
        ) : null}
      </section>

      <section className="metric-grid">
        <article className="metric-card">
          <span className="eyebrow">Sources</span>
          <strong>{sources.length}</strong>
        </article>
        <article className="metric-card">
          <span className="eyebrow">Candidates</span>
          <strong>{discoveryQuery.data?.items.length ?? 0}</strong>
        </article>
        <article className="metric-card">
          <span className="eyebrow">Registered</span>
          <strong>
            {(discoveryQuery.data?.items ?? []).filter((item) => item.registration_status === "registered").length}
          </strong>
        </article>
        <article className="metric-card">
          <span className="eyebrow">Unregistered</span>
          <strong>
            {(discoveryQuery.data?.items ?? []).filter((item) => item.registration_status !== "registered").length}
          </strong>
        </article>
      </section>

      <section className="section-card">
        <div className="section-card__header">
          <div>
            <span className="eyebrow">Sources</span>
            <h2>Discovery backends</h2>
          </div>
        </div>
        <div className="plugin-grid">
          {sources.map((entry) => (
            <article className="plugin-card" key={entry.integration_instance.id}>
              <div className="plugin-card__meta">
                <span className="plugin-card__path">
                  {entry.domain || entry.provider} / {entry.section || "discovery"} / {entry.entry || entry.plugin_name}
                </span>
                <StatusPill status={entry.health_status || "unknown"} />
              </div>
              <div className="plugin-card__head">
                <div>
                  <h3>{entry.integration_instance.name}</h3>
                  <p>{entry.plugin_name} via {entry.provider}</p>
                </div>
                <div className="plugin-card__version">{entry.discovery_status || "pending"}</div>
              </div>
              {entry.message ? <p>{entry.message}</p> : null}
            </article>
          ))}
        </div>
      </section>

      <section className="section-card">
        <div className="section-card__header">
          <div>
            <span className="eyebrow">Candidates</span>
            <h2>Available integrations and surfaces</h2>
          </div>
        </div>
        {discoveryQuery.isLoading ? (
          <div className="state-card">Running discovery through the core...</div>
        ) : discoveryQuery.isError ? (
          <div className="state-card state-card--danger">
            Failed to run catalog discovery through yggdrasil-core.
          </div>
        ) : (
          <div className="plugin-grid">
            {filteredItems.map((item) => (
              <DiscoveryCard item={item} key={`${item.kind}-${item.name}-${item.repository ?? item.source.integration_instance.id}`} />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}

function DiscoveryCard({ item }: { item: CatalogDiscoveryItem }) {
  return (
    <article className="plugin-card">
      <div className="plugin-card__meta">
        <span className="plugin-card__path">
          {item.kind}
          {item.domain ? ` / ${item.domain}` : ""}
          {item.section ? ` / ${item.section}` : ""}
          {item.entry ? ` / ${item.entry}` : ""}
        </span>
        <StatusPill status={item.registration_status === "registered" ? "healthy" : "unconfigured"} />
      </div>
      <div className="plugin-card__head">
        <div>
          <h3>{item.display_name || item.name}</h3>
          <p>{item.description || "No description provided by the discovery source."}</p>
        </div>
        <div className="plugin-card__version">{item.registration_status}</div>
      </div>
      <div className="plugin-card__details">
        <div>
          <span className="eyebrow">Discovered via</span>
          <strong>{item.source.integration_instance.name}</strong>
        </div>
        <div>
          <span className="eyebrow">Registration</span>
          <strong>
            {item.registered_manifest
              ? `${item.registered_manifest.kind} ${item.registered_manifest.namespace}/${item.registered_manifest.name}`
              : "not registered in core"}
          </strong>
        </div>
      </div>
      {item.repository ? (
        <div className="plugin-card__actions">
          <a className="button button--secondary" href={item.repository} target="_blank" rel="noreferrer">
            Open repository
          </a>
        </div>
      ) : null}
    </article>
  );
}
