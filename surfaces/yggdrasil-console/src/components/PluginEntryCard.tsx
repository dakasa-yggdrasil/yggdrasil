import { StatusPill } from "./StatusPill";
import type { IntegrationCatalogEntry } from "../types";

interface PluginEntryCardProps {
  entry: IntegrationCatalogEntry;
  onCreateInstance: (entry: IntegrationCatalogEntry) => void;
}

export function PluginEntryCard({ entry, onCreateInstance }: PluginEntryCardProps) {
  return (
    <article className="plugin-card">
      <div className="plugin-card__meta">
        <span className="plugin-card__path">
          {entry.domain} / {entry.section} / {entry.entry}
        </span>
        <StatusPill status={entry.status} />
      </div>
      <div className="plugin-card__head">
        <div>
          <h3>{entry.plugin_name}</h3>
          <p>{entry.description || "No description provided for this plugin yet."}</p>
        </div>
        <div className="plugin-card__version">v{entry.adapter_version || "n/a"}</div>
      </div>
      <div className="plugin-card__details">
        <div>
          <span className="eyebrow">Provider</span>
          <strong>{entry.provider}</strong>
        </div>
        <div>
          <span className="eyebrow">Capabilities</span>
          <div className="token-row">
            {(entry.capabilities || []).map((capability) => (
              <span className="token" key={capability}>
                {capability}
              </span>
            ))}
          </div>
        </div>
      </div>
      <div className="plugin-card__instances">
        <span className="eyebrow">Configured instances</span>
        <strong>{entry.instances?.length ?? 0}</strong>
      </div>
      <div className="plugin-card__actions">
        <button className="button button--secondary" onClick={() => onCreateInstance(entry)}>
          New instance
        </button>
      </div>
    </article>
  );
}
