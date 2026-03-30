import { useDeferredValue, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createCollaborator,
  createTeam,
  fetchCollaborators,
  fetchTeamMemberships,
  fetchTeams,
  type DataSourceMode,
  upsertTeamMembership,
} from "../lib/api";
import { parseObjectInput } from "../lib/json";
import { StatusPill } from "../components/StatusPill";
import type {
  CollaboratorRecord,
  CreateCollaboratorPayload,
  CreateTeamPayload,
  TeamMembershipRecord,
  TeamRecord,
  UpsertTeamMembershipPayload,
} from "../types";

interface PeoplePageProps {
  source: DataSourceMode;
}

export function PeoplePage({ source }: PeoplePageProps) {
  const queryClient = useQueryClient();
  const [filter, setFilter] = useState("");
  const deferredFilter = useDeferredValue(filter.trim().toLowerCase());

  const collaboratorsQuery = useQuery({
    queryKey: ["collaborators"],
    queryFn: fetchCollaborators,
  });

  const teamsQuery = useQuery({
    queryKey: ["teams"],
    queryFn: fetchTeams,
  });

  const membershipsQuery = useQuery({
    queryKey: ["team-memberships"],
    queryFn: fetchTeamMemberships,
  });

  const collaboratorMutation = useMutation({
    mutationFn: createCollaborator,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["collaborators"] });
    },
  });

  const teamMutation = useMutation({
    mutationFn: createTeam,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["teams"] });
    },
  });

  const membershipMutation = useMutation({
    mutationFn: upsertTeamMembership,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["team-memberships"] }),
        queryClient.invalidateQueries({ queryKey: ["collaborators"] }),
        queryClient.invalidateQueries({ queryKey: ["teams"] }),
      ]);
    },
  });

  const collaborators = useMemo(() => {
    const entries = collaboratorsQuery.data?.collaborators ?? [];
    if (!deferredFilter) {
      return entries;
    }
    return entries.filter((collaborator) =>
      [
        collaborator.slug,
        collaborator.display_name,
        collaborator.primary_email,
        collaborator.status,
      ]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(deferredFilter)),
    );
  }, [collaboratorsQuery.data?.collaborators, deferredFilter]);

  const teams = useMemo(() => {
    const entries = teamsQuery.data?.teams ?? [];
    if (!deferredFilter) {
      return entries;
    }
    return entries.filter((team) =>
      [team.slug, team.name, team.type, team.status]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(deferredFilter)),
    );
  }, [teamsQuery.data?.teams, deferredFilter]);

  const memberships = useMemo(() => {
    const entries = membershipsQuery.data?.memberships ?? [];
    if (!deferredFilter) {
      return entries;
    }
    return entries.filter((membership) =>
      [membership.team_slug, membership.collaborator_slug, membership.role, membership.source]
        .filter(Boolean)
        .some((value) => value!.toLowerCase().includes(deferredFilter)),
    );
  }, [membershipsQuery.data?.memberships, deferredFilter]);

  const loading =
    collaboratorsQuery.isLoading || teamsQuery.isLoading || membershipsQuery.isLoading;
  const failed =
    collaboratorsQuery.isError || teamsQuery.isError || membershipsQuery.isError;

  const metrics = [
    { label: "Collaborators", value: collaboratorsQuery.data?.collaborators.length ?? 0 },
    { label: "Teams", value: teamsQuery.data?.teams.length ?? 0 },
    { label: "Memberships", value: membershipsQuery.data?.memberships.length ?? 0 },
    {
      label: "Active collaborators",
      value:
        collaboratorsQuery.data?.collaborators.filter(
          (collaborator) => collaborator.status === "active",
        ).length ?? 0,
    },
  ];

  return (
    <div className="page-stack">
      <section className="hero-panel">
        <div>
          <span className="eyebrow">People & structure</span>
          <h1>Feed collaborators and teams into the core without hiding the shape of the org.</h1>
          <p>
            This surface is where we stop treating identity like loose metadata. Teams,
            memberships, and collaborator context become governed records in the core.
          </p>
        </div>
        <div className="hero-panel__badge">
          <span>Current write path</span>
          <strong>{source === "live" ? "Console -> Core" : "Demo fallback for reads"}</strong>
        </div>
      </section>

      {source === "mock" ? (
        <div className="note-banner">
          Read operations are using mock data. Writes stay blocked until the console reaches
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
            <span className="eyebrow">Directory</span>
            <h2>People and team structure</h2>
            <p>
              The forms stay structured, but still leave room for richer profile payloads when we
              need them.
            </p>
          </div>
          <label className="search-field">
            <span>Filter</span>
            <input
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              placeholder="ana, platform, engineering..."
            />
          </label>
        </div>

        {loading ? (
          <div className="state-card">Loading collaborators, teams, and memberships...</div>
        ) : failed ? (
          <div className="state-card state-card--danger">
            Failed to load people data from yggdrasil-core.
          </div>
        ) : (
          <div className="workspace-grid workspace-grid--two">
            <div className="workspace-stack">
              <section className="section-panel">
                <div className="section-panel__header">
                  <div>
                    <span className="eyebrow">Collaborators</span>
                    <h3>Canonical subjects</h3>
                  </div>
                </div>
                <CollaboratorForm
                  teams={teamsQuery.data?.teams ?? []}
                  onSubmit={(payload) => collaboratorMutation.mutateAsync(payload)}
                  pending={collaboratorMutation.isPending}
                />
                <div className="entity-list">
                  {collaborators.map((collaborator) => (
                    <CollaboratorCard key={collaborator.id} collaborator={collaborator} />
                  ))}
                </div>
              </section>
            </div>

            <div className="workspace-stack">
              <section className="section-panel">
                <div className="section-panel__header">
                  <div>
                    <span className="eyebrow">Teams</span>
                    <h3>Groups with ownership context</h3>
                  </div>
                </div>
                <TeamForm
                  teams={teamsQuery.data?.teams ?? []}
                  onSubmit={(payload) => teamMutation.mutateAsync(payload)}
                  pending={teamMutation.isPending}
                />
                <div className="entity-list">
                  {teams.map((team) => (
                    <TeamCard key={team.id} team={team} />
                  ))}
                </div>
              </section>
            </div>
          </div>
        )}
      </section>

      {!loading && !failed ? (
        <section className="section-card">
          <div className="section-card__header">
            <div>
              <span className="eyebrow">Memberships</span>
              <h2>Who belongs where</h2>
            </div>
          </div>
          <div className="workspace-grid workspace-grid--wide">
            <section className="section-panel">
              <MembershipForm
                collaborators={collaboratorsQuery.data?.collaborators ?? []}
                teams={teamsQuery.data?.teams ?? []}
                onSubmit={(payload) => membershipMutation.mutateAsync(payload)}
                pending={membershipMutation.isPending}
              />
            </section>
            <section className="section-panel">
              <div className="list-table">
                <div className="list-table__head">
                  <span>Team</span>
                  <span>Collaborator</span>
                  <span>Role</span>
                  <span>Source</span>
                  <span>Status</span>
                </div>
                {memberships.map((membership) => (
                  <div className="list-table__row" key={membership.id}>
                    <span>{membership.team_slug}</span>
                    <span>{membership.collaborator_slug}</span>
                    <span>{membership.role}</span>
                    <span>{membership.source}</span>
                    <span>
                      <StatusPill status={membership.active ? "active" : "disabled"} />
                    </span>
                  </div>
                ))}
              </div>
            </section>
          </div>
        </section>
      ) : null}
    </div>
  );
}

interface CollaboratorFormProps {
  teams: TeamRecord[];
  onSubmit: (payload: CreateCollaboratorPayload) => Promise<unknown>;
  pending: boolean;
}

function CollaboratorForm({ teams, onSubmit, pending }: CollaboratorFormProps) {
  const [slug, setSlug] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [primaryEmail, setPrimaryEmail] = useState("");
  const [status, setStatus] = useState("active");
  const [primaryTeamID, setPrimaryTeamID] = useState("");
  const [employmentData, setEmploymentData] = useState('{\n  "title": "Platform engineer"\n}');
  const [thirdPartyIdentities, setThirdPartyIdentities] = useState('{\n  "github": {\n    "login": ""\n  }\n}');
  const [traits, setTraits] = useState("{}");
  const [submissionError, setSubmissionError] = useState<string | null>(null);

  async function submit() {
    try {
      setSubmissionError(null);
      await onSubmit({
        slug,
        display_name: displayName,
        primary_email: primaryEmail.trim() || undefined,
        status,
        primary_team_id: primaryTeamID || undefined,
        employment_data: parseObjectInput(employmentData, "employment data"),
        third_party_identities: parseObjectInput(
          thirdPartyIdentities,
          "third party identities",
        ),
        traits: parseObjectInput(traits, "traits"),
      });
      setSlug("");
      setDisplayName("");
      setPrimaryEmail("");
      setPrimaryTeamID("");
      setTraits("{}");
    } catch (error) {
      setSubmissionError(error instanceof Error ? error.message : "Failed to create collaborator.");
    }
  }

  return (
    <div className="form-stack">
      <div className="form-grid">
        <label className="field">
          <span>Slug</span>
          <input value={slug} onChange={(event) => setSlug(event.target.value)} placeholder="ana" />
        </label>
        <label className="field">
          <span>Display name</span>
          <input
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            placeholder="Ana Platform"
          />
        </label>
        <label className="field">
          <span>Primary email</span>
          <input
            value={primaryEmail}
            onChange={(event) => setPrimaryEmail(event.target.value)}
            placeholder="ana@dakasa.dev"
          />
        </label>
        <label className="field">
          <span>Status</span>
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            <option value="active">active</option>
            <option value="inactive">inactive</option>
            <option value="suspended">suspended</option>
          </select>
        </label>
        <label className="field">
          <span>Primary team</span>
          <select value={primaryTeamID} onChange={(event) => setPrimaryTeamID(event.target.value)}>
            <option value="">Select one team</option>
            {teams.map((team) => (
              <option key={team.id} value={team.id}>
                {team.name}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="form-grid">
        <label className="field">
          <span>Employment data JSON</span>
          <textarea
            rows={6}
            value={employmentData}
            onChange={(event) => setEmploymentData(event.target.value)}
          />
        </label>
        <label className="field">
          <span>Third-party identities JSON</span>
          <textarea
            rows={6}
            value={thirdPartyIdentities}
            onChange={(event) => setThirdPartyIdentities(event.target.value)}
          />
        </label>
      </div>

      <label className="field">
        <span>Traits JSON</span>
        <textarea rows={4} value={traits} onChange={(event) => setTraits(event.target.value)} />
      </label>

      {submissionError ? <div className="state-card state-card--danger">{submissionError}</div> : null}

      <div className="form-actions">
        <button className="button" onClick={submit} disabled={pending || !slug.trim() || !displayName.trim()}>
          {pending ? "Creating..." : "Create collaborator"}
        </button>
      </div>
    </div>
  );
}

interface TeamFormProps {
  teams: TeamRecord[];
  onSubmit: (payload: CreateTeamPayload) => Promise<unknown>;
  pending: boolean;
}

function TeamForm({ teams, onSubmit, pending }: TeamFormProps) {
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [type, setType] = useState("engineering");
  const [status, setStatus] = useState("active");
  const [parentTeamID, setParentTeamID] = useState("");
  const [owners, setOwners] = useState("team:platform");
  const [traits, setTraits] = useState("{}");
  const [submissionError, setSubmissionError] = useState<string | null>(null);

  async function submit() {
    try {
      setSubmissionError(null);
      await onSubmit({
        slug,
        name,
        type,
        status,
        parent_team_id: parentTeamID || undefined,
        owners: owners
          .split(",")
          .map((entry) => entry.trim())
          .filter(Boolean),
        traits: parseObjectInput(traits, "team traits"),
      });
      setSlug("");
      setName("");
      setParentTeamID("");
      setTraits("{}");
    } catch (error) {
      setSubmissionError(error instanceof Error ? error.message : "Failed to create team.");
    }
  }

  return (
    <div className="form-stack">
      <div className="form-grid">
        <label className="field">
          <span>Slug</span>
          <input
            value={slug}
            onChange={(event) => setSlug(event.target.value)}
            placeholder="platform"
          />
        </label>
        <label className="field">
          <span>Name</span>
          <input
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="Platform"
          />
        </label>
        <label className="field">
          <span>Type</span>
          <input value={type} onChange={(event) => setType(event.target.value)} />
        </label>
        <label className="field">
          <span>Status</span>
          <select value={status} onChange={(event) => setStatus(event.target.value)}>
            <option value="active">active</option>
            <option value="inactive">inactive</option>
            <option value="disabled">disabled</option>
          </select>
        </label>
        <label className="field">
          <span>Parent team</span>
          <select value={parentTeamID} onChange={(event) => setParentTeamID(event.target.value)}>
            <option value="">No parent</option>
            {teams.map((team) => (
              <option key={team.id} value={team.id}>
                {team.name}
              </option>
            ))}
          </select>
        </label>
      </div>

      <label className="field">
        <span>Owners</span>
        <input
          value={owners}
          onChange={(event) => setOwners(event.target.value)}
          placeholder="team:platform, collaborator:ana"
        />
      </label>

      <label className="field">
        <span>Traits JSON</span>
        <textarea rows={4} value={traits} onChange={(event) => setTraits(event.target.value)} />
      </label>

      {submissionError ? <div className="state-card state-card--danger">{submissionError}</div> : null}

      <div className="form-actions">
        <button className="button" onClick={submit} disabled={pending || !slug.trim() || !name.trim()}>
          {pending ? "Creating..." : "Create team"}
        </button>
      </div>
    </div>
  );
}

interface MembershipFormProps {
  collaborators: CollaboratorRecord[];
  teams: TeamRecord[];
  onSubmit: (payload: UpsertTeamMembershipPayload) => Promise<unknown>;
  pending: boolean;
}

function MembershipForm({ collaborators, teams, onSubmit, pending }: MembershipFormProps) {
  const [teamID, setTeamID] = useState("");
  const [collaboratorID, setCollaboratorID] = useState("");
  const [role, setRole] = useState("member");
  const [source, setSource] = useState("console");
  const [active, setActive] = useState(true);
  const [submissionError, setSubmissionError] = useState<string | null>(null);

  async function submit() {
    try {
      setSubmissionError(null);
      await onSubmit({
        team_id: teamID,
        collaborator_id: collaboratorID,
        role,
        source,
        active,
      });
      setRole("member");
      setSource("console");
    } catch (error) {
      setSubmissionError(
        error instanceof Error ? error.message : "Failed to upsert team membership.",
      );
    }
  }

  return (
    <div className="form-stack">
      <div className="section-panel__header">
        <div>
          <span className="eyebrow">Attach people</span>
          <h3>Membership editor</h3>
        </div>
      </div>

      <div className="form-grid">
        <label className="field">
          <span>Team</span>
          <select value={teamID} onChange={(event) => setTeamID(event.target.value)}>
            <option value="">Select one team</option>
            {teams.map((team) => (
              <option key={team.id} value={team.id}>
                {team.name}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>Collaborator</span>
          <select
            value={collaboratorID}
            onChange={(event) => setCollaboratorID(event.target.value)}
          >
            <option value="">Select one collaborator</option>
            {collaborators.map((collaborator) => (
              <option key={collaborator.id} value={collaborator.id}>
                {collaborator.display_name}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>Role</span>
          <input value={role} onChange={(event) => setRole(event.target.value)} />
        </label>
        <label className="field">
          <span>Source</span>
          <input value={source} onChange={(event) => setSource(event.target.value)} />
        </label>
        <label className="field field--inline">
          <span>Active</span>
          <input type="checkbox" checked={active} onChange={(event) => setActive(event.target.checked)} />
        </label>
      </div>

      {submissionError ? <div className="state-card state-card--danger">{submissionError}</div> : null}

      <div className="form-actions">
        <button
          className="button"
          onClick={submit}
          disabled={pending || !teamID || !collaboratorID}
        >
          {pending ? "Saving..." : "Save membership"}
        </button>
      </div>
    </div>
  );
}

function CollaboratorCard({ collaborator }: { collaborator: CollaboratorRecord }) {
  return (
    <article className="entity-card">
      <div className="entity-card__header">
        <div>
          <h3>{collaborator.display_name}</h3>
          <p>{collaborator.slug}</p>
        </div>
        <StatusPill status={collaborator.status} />
      </div>
      <div className="entity-card__meta">
        <span>{collaborator.primary_email || "No primary email"}</span>
        <span>{collaborator.employment_data?.title ? String(collaborator.employment_data.title) : "No title yet"}</span>
      </div>
      <div className="token-row">
        {Object.entries(collaborator.traits ?? {}).map(([key, value]) => (
          <span className="token" key={key}>
            {key}:{String(value)}
          </span>
        ))}
      </div>
    </article>
  );
}

function TeamCard({ team }: { team: TeamRecord }) {
  return (
    <article className="entity-card">
      <div className="entity-card__header">
        <div>
          <h3>{team.name}</h3>
          <p>{team.slug}</p>
        </div>
        <StatusPill status={team.status} />
      </div>
      <div className="entity-card__meta">
        <span>{team.type || "No type"}</span>
        <span>{team.owners?.length ? `${team.owners.length} owners` : "No owners"}</span>
      </div>
      <div className="token-row">
        {team.owners?.map((owner) => (
          <span className="token" key={owner}>
            {owner}
          </span>
        ))}
      </div>
    </article>
  );
}
