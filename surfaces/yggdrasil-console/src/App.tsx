import { NavLink, Route, Routes } from "react-router-dom";
import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { fetchIntegrationCatalog } from "./lib/api";
import { OverviewPage } from "./pages/OverviewPage";
import { CatalogPage } from "./pages/CatalogPage";
import { DiscoverPage } from "./pages/DiscoverPage";
import { IntegrationsPage } from "./pages/IntegrationsPage";
import { PeoplePage } from "./pages/PeoplePage";
import { DeliveryPage } from "./pages/DeliveryPage";
import { SurfacesPage } from "./pages/SurfacesPage";
import { InstanceComposer } from "./components/InstanceComposer";
import type { IntegrationCatalogEntry } from "./types";

export function App() {
  const [selectedEntry, setSelectedEntry] = useState<IntegrationCatalogEntry | null>(null);

  const catalogQuery = useQuery({
    queryKey: ["integration-catalog"],
    queryFn: fetchIntegrationCatalog,
  });

  const domains = catalogQuery.data?.domains ?? [];
  const source = catalogQuery.data?.source ?? "live";

  const summary = useMemo(() => {
    const entries = domains.flatMap((domain) => domain.sections.flatMap((section) => section.entries));
    const instances = entries.flatMap((entry) => entry.instances ?? []);
    return {
      plugins: entries.length,
      instances: instances.length,
      unhealthy: entries.filter((entry) => entry.status !== "healthy").length,
    };
  }, [domains]);

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="sidebar__brand">
          <div className="sidebar__mark">Y</div>
          <div>
            <strong>Yggdrasil Console</strong>
            <small>admin plane</small>
          </div>
        </div>

        <nav className="sidebar__nav">
          <NavItem to="/">Overview</NavItem>
          <NavItem to="/discover">Discover</NavItem>
          <NavItem to="/catalog">Plugin catalog</NavItem>
          <NavItem to="/integrations">Integration instances</NavItem>
          <NavItem to="/surfaces">Surfaces</NavItem>
          <NavItem to="/people">Collaborators & teams</NavItem>
          <NavItem to="/delivery">Products & workflows</NavItem>
        </nav>

        <div className="sidebar__summary">
          <span className="eyebrow">At a glance</span>
          <strong>{summary.plugins} plugins</strong>
          <strong>{summary.instances} instances</strong>
          <strong>{summary.unhealthy} non-healthy entries</strong>
        </div>
      </aside>

      <main className="content">
        <header className="topbar">
          <div>
            <span className="eyebrow">Console mode</span>
            <h1>{source === "live" ? "Core-backed runtime" : "Demo fallback"}</h1>
          </div>
          <div className={`source-banner source-banner--${source}`}>
            {source === "live"
              ? "Reading directly from yggdrasil-core"
              : "Reading mock data because yggdrasil-core is unavailable in dev"}
          </div>
        </header>

        {catalogQuery.isLoading ? (
          <div className="state-card">Loading the Yggdrasil catalog...</div>
        ) : catalogQuery.isError ? (
          <div className="state-card state-card--danger">
            Failed to load the catalog.
          </div>
        ) : (
          <Routes>
            <Route path="/" element={<OverviewPage domains={domains} source={source} />} />
            <Route path="/discover" element={<DiscoverPage source={source} />} />
            <Route
              path="/catalog"
              element={<CatalogPage domains={domains} onCreateInstance={setSelectedEntry} />}
            />
            <Route
              path="/integrations"
              element={<IntegrationsPage domains={domains} onCreateInstance={setSelectedEntry} />}
            />
            <Route path="/surfaces" element={<SurfacesPage source={source} />} />
            <Route path="/people" element={<PeoplePage source={source} />} />
            <Route path="/delivery" element={<DeliveryPage source={source} />} />
          </Routes>
        )}
      </main>

      <InstanceComposer
        open={selectedEntry !== null}
        selectedEntry={selectedEntry}
        onClose={() => setSelectedEntry(null)}
      />
    </div>
  );
}

function NavItem({ to, children }: { to: string; children: string }) {
  return (
    <NavLink
      to={to}
      end={to === "/"}
      className={({ isActive }) => `sidebar__link ${isActive ? "sidebar__link--active" : ""}`}
    >
      {children}
    </NavLink>
  );
}
