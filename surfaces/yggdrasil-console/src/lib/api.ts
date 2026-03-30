import {
  mockCatalogDiscovery,
  mockCollaborators,
  mockCatalogEntryDetail,
  mockProducts,
  mockSurfaces,
  mockTeamMemberships,
  mockTeams,
  mockWorkflows,
  mockIntegrationCatalog,
} from "./mock";
import type {
  CollaboratorsResponse,
  CreateCollaboratorPayload,
  CreateIntegrationInstancePayload,
  CreateManifestPayload,
  CreateTeamPayload,
  CatalogDiscoveryResponse,
  IntegrationCatalogEntryDetailResponse,
  IntegrationCatalogResponse,
  ManifestRecord,
  ManifestsResponse,
  SurfacesResponse,
  TeamMembershipsResponse,
  TeamsResponse,
  UpsertTeamMembershipPayload,
  CollaboratorRecord,
  TeamRecord,
  TeamMembershipRecord,
} from "../types";

const apiBaseUrl = (import.meta.env.VITE_YGGDRASIL_API_BASE_URL as string | undefined)?.trim() ?? "";

export type DataSourceMode = "live" | "mock";

export interface IntegrationCatalogResult extends IntegrationCatalogResponse {
  source: DataSourceMode;
}

export interface IntegrationCatalogEntryDetailResult extends IntegrationCatalogEntryDetailResponse {
  source: DataSourceMode;
}

export interface CatalogDiscoveryResult extends CatalogDiscoveryResponse {
  source: DataSourceMode;
}

function isDevFallbackAllowed() {
  return Boolean(import.meta.env.DEV);
}

function writeUnavailableError(): Error {
  return new Error(
    [
      "This write operation is unavailable because the console is running in demo mode.",
      "Start yggdrasil-core and point the console to it to enable writes.",
    ].join(" "),
  );
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });

  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;
    try {
      const body = (await response.json()) as { error?: string };
      if (body.error) {
        message = body.error;
      }
    } catch {
      // ignore body parsing
    }
    throw new Error(message);
  }

  return (await response.json()) as T;
}

export async function fetchIntegrationCatalog(): Promise<IntegrationCatalogResult> {
  try {
    const response = await requestJSON<IntegrationCatalogResponse>("/api/v1/console/integration-catalog");
    return { ...response, source: "live" };
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    return { ...mockIntegrationCatalog(), source: "mock" };
  }
}

export async function fetchCatalogDiscovery(): Promise<CatalogDiscoveryResult> {
  try {
    const response = await requestJSON<CatalogDiscoveryResponse>("/api/v1/console/catalog-discovery");
    return { ...response, source: "live" };
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    return { ...mockCatalogDiscovery(), source: "mock" };
  }
}

export async function fetchIntegrationCatalogEntryDetail(
  domain: string,
  section: string,
  entry: string,
): Promise<IntegrationCatalogEntryDetailResult> {
  try {
    const response = await requestJSON<IntegrationCatalogEntryDetailResponse>(
      `/api/v1/console/integration-catalog/${domain}/${section}/${entry}`,
    );
    return { ...response, source: "live" };
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    return { ...mockCatalogEntryDetail(domain, section, entry), source: "mock" };
  }
}

export async function createIntegrationInstance(
  payload: CreateIntegrationInstancePayload,
): Promise<{ manifest: ManifestRecord }> {
  try {
    return await requestJSON<{ manifest: ManifestRecord }>("/api/v1/console/integration-instances", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    throw writeUnavailableError();
  }
}

export async function fetchCollaborators(): Promise<CollaboratorsResponse> {
  try {
    return await requestJSON<CollaboratorsResponse>("/api/v1/console/collaborators");
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    return mockCollaborators();
  }
}

export async function createCollaborator(
  payload: CreateCollaboratorPayload,
): Promise<{ collaborator: CollaboratorRecord }> {
  try {
    return await requestJSON<{ collaborator: CollaboratorRecord }>("/api/v1/console/collaborators", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    throw writeUnavailableError();
  }
}

export async function fetchTeams(): Promise<TeamsResponse> {
  try {
    return await requestJSON<TeamsResponse>("/api/v1/console/teams");
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    return mockTeams();
  }
}

export async function createTeam(payload: CreateTeamPayload): Promise<{ team: TeamRecord }> {
  try {
    return await requestJSON<{ team: TeamRecord }>("/api/v1/console/teams", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    throw writeUnavailableError();
  }
}

export async function fetchTeamMemberships(): Promise<TeamMembershipsResponse> {
  try {
    return await requestJSON<TeamMembershipsResponse>("/api/v1/console/team-memberships");
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    return mockTeamMemberships();
  }
}

export async function upsertTeamMembership(
  payload: UpsertTeamMembershipPayload,
): Promise<{ membership: TeamMembershipRecord }> {
  try {
    return await requestJSON<{ membership: TeamMembershipRecord }>("/api/v1/console/team-memberships", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    throw writeUnavailableError();
  }
}

export async function fetchProducts(): Promise<ManifestsResponse> {
  try {
    return await requestJSON<ManifestsResponse>("/api/v1/console/products");
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    return mockProducts();
  }
}

export async function fetchSurfaces(): Promise<SurfacesResponse> {
  try {
    return await requestJSON<SurfacesResponse>("/api/v1/console/surfaces");
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    return mockSurfaces();
  }
}

export async function createSurface(
  payload: CreateManifestPayload,
): Promise<{ manifest: ManifestRecord }> {
  try {
    return await requestJSON<{ manifest: ManifestRecord }>("/api/v1/console/surfaces", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    throw writeUnavailableError();
  }
}

export async function createProduct(
  payload: CreateManifestPayload,
): Promise<{ manifest: ManifestRecord }> {
  try {
    return await requestJSON<{ manifest: ManifestRecord }>("/api/v1/console/products", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    throw writeUnavailableError();
  }
}

export async function fetchWorkflows(): Promise<ManifestsResponse> {
  try {
    return await requestJSON<ManifestsResponse>("/api/v1/console/workflows");
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    return mockWorkflows();
  }
}

export async function createWorkflow(
  payload: CreateManifestPayload,
): Promise<{ manifest: ManifestRecord }> {
  try {
    return await requestJSON<{ manifest: ManifestRecord }>("/api/v1/console/workflows", {
      method: "POST",
      body: JSON.stringify(payload),
    });
  } catch (error) {
    if (!isDevFallbackAllowed()) {
      throw error;
    }
    throw writeUnavailableError();
  }
}
