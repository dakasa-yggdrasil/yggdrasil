# Event Stream — yggdrasil-core

**Data:** 2026-04-10
**Status:** Spec em revisão
**Tipo:** Core primitive (new feature)
**Prioridade:** 🔴 Máxima — fundacional para outros core gaps (BuildProject lifecycle, workflow trigger system, audit log projections)
**Escopo:** Adicionar ao yggdrasil-core a primitive de **event stream** — emissão durável de events de mutação de estado, com subscription via cursor-based pull, contracts em JSON Schema, schema-versioned events, TTL configurável por tipo.
**Depende de:** nada
**Habilita:** BuildProject lifecycle enforcement, workflow trigger system (schedule + event), audit log projection (via integration), workflow run history projection (via integration ou surface), activity feed em surfaces, SIEM export (via integration).
**Parte do audit report:** [`2026-04-10-yggdrasil-product-audit-report.md`](./2026-04-10-yggdrasil-product-audit-report.md) Gap 1

---

## 1. Contexto e motivação

### 1.1 Por que esta primitive

A filosofia do Yggdrasil é que **core gerencia o estado do ecossistema com maestria**. Hoje, core faz isso para manifests, topology, products e workflows individualmente — cada um com suas próprias tabelas, timestamps e auditoria parcial (`product_materializations` para products, `updated_at` em manifests, etc.).

O que **falta** é uma trilha canônica unificada de **mutações de estado** que seja:

- **Durável**: persistida, não ephemeral
- **Ordered**: per-aggregate ordering garantido; global monotonic cursor para avanço de subscribers
- **Observable**: expostas para subscribers via RPC (pull-based)
- **Schema-versioned**: cada tipo de evento tem JSON Schema publicado, versionado
- **Language-agnostic**: consumers em qualquer linguagem conseguem parsear sem dependency em Go
- **Retention-configurable**: diferentes tipos de evento podem ter diferentes TTLs

Essa primitive é **fundacional** porque várias outras features do core (já pedidas ou implícitas) podem ser implementadas como consequência direta:

- **Audit log** é uma projeção do event stream (integration subscribe e envia para SIEM)
- **Workflow run history** é uma projeção (subscribe em `workflow.*` e reconstrói timeline)
- **BuildProject lifecycle events** (created/extended/expired) são events emitidos pelo lifecycle enforcer
- **Activity feed no console** é uma projeção em real-time (surface polling)
- **Trigger event-driven de workflows** é um subscriber interno do core (workflows com `trigger.mode=event` subscribem a events com filtros)
- **Notificações Slack/email** são integrations que subscribem e disparam ações
- **Cost tracking** é uma projeção de `product.applied` + `integration.execute` events correlated com billing APIs

Sem event stream, cada uma dessas features precisaria reimplementar sua própria infraestrutura de events, com tabelas separadas e polling customizado. Isso viola a filosofia minimalista e cria fragmentação de estado.

### 1.2 Por que é core (não integration, não surface)

- **Não pode ser integration**: integrations são externas ao core e não têm visão canônica das mutações. Uma integration externa não sabe quando um manifest foi mutado, porque a mutação acontece dentro do core. Delegar event emission a integrations significaria que integrations precisariam "intercepter" operações do core, o que é acoplamento errado.
- **Não pode ser surface**: surfaces estão acima do core e consomem eventos via API. Não geram eventos; expõem/consomem.
- **Tem que ser core**: só core tem visão e timing corretos para emitir events consistentes com suas próprias mutações de estado. Core emite (no mesmo commit da mutação, idealmente), persiste, e expõe via RPC para subscribers.

## 2. Filosofia de design

### 2.1 Princípios

1. **Contract-first em JSON Schema**: toda definição de event type é um JSON Schema em `docs/contracts/events/v1/{category}/{type}.schema.json`. Core valida ao emit time. Consumers parseiam e validam contra o mesmo schema em qualquer linguagem.
2. **Pull-based como primary**: subscribers chamam RPC `event_stream.pull` com cursor e filtros. Core responde com batch de events. Subscribers avançam o cursor por conta própria. Sem push callbacks, sem slow-consumer backpressure no core.
3. **Per-aggregate ordering + global monotonic cursor**: events relacionados a um mesmo aggregate (ex: um `manifest_id` específico) mantém ordem causal. Cursor global é monotônico (sequence number auto-incrementando) mas **não** garante ordering cross-aggregate em multi-worker scenarios (os relógios/transações podem interlear).
4. **Retention configurável por tipo**: cada event type tem um TTL default. Admins podem override via configuration (ex: `authorization.decided` retém 7 anos para compliance, `workflow.step.completed` retém 30 dias).
5. **Idempotência de emit**: mesmo evento nunca é emitido duas vezes. `event_id` é gerado por core no momento do emit (UUID) e persistido num unique index junto com a operação de mutação.
6. **Transactional com a mutação**: event emit acontece na mesma transação PostgreSQL da mutação de estado. Se a transação fizer rollback, o event não é persistido. Garantia: se você vê a mutação, o event existe.
7. **Schema versioning explícito**: cada event type tem um campo `schema_version: string` no payload. Nunca há breaking change em v1; novos campos são opcionais. Breaking changes criam v2 e coexistem.
8. **Subscribers opacos**: core não rastreia subscribers individualmente por default. Subscribers são responsáveis por persistir seu próprio cursor. Isso mantém core stateless em relação a consumers. *(Opcional futuro: um `subscription_state` table para core rastrear cursors de subscribers conhecidos, mas não é MVP.)*

### 2.2 Trade-offs aceitos

- **Não é real-time**: pull-based introduz latência = intervalo de polling. Para real-time, uma surface pode implementar websocket/SSE que faz pulls rápidos internamente.
- **Write amplification**: cada mutação grava em 2+ tabelas (a tabela da entidade + a tabela de events). Aceito pelo valor de auditability.
- **Storage crescimento**: events acumulam. Retention policy + periodic cleanup job resolvem.
- **Cross-aggregate ordering não garantido**: events de dois manifests diferentes podem chegar "fora de ordem" num subscriber se dois workers emitirem em transações concorrentes. Aceito porque per-aggregate ordering (o que importa para reasoning) é garantido via FK + sequence.

## 3. Arquitetura

### 3.1 Estrutura geral

```
┌─────────────────────────────────────────────────────────────┐
│                       yggdrasil-core                         │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Mutation operation (e.g., manifest.create handler)  │   │
│  │                                                       │   │
│  │  BEGIN TX                                             │   │
│  │    INSERT INTO manifests (...);                       │   │
│  │    events.Emit("manifest.created", payload, tx);     │   │
│  │  COMMIT TX                                            │   │
│  └──────────────────────────────────────────────────────┘   │
│                         │                                    │
│                         ▼                                    │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  repository/events.go                                 │   │
│  │                                                       │   │
│  │  AppendEvent(tx, type, aggregate_id, payload,        │   │
│  │              schema_version, metadata)                │   │
│  │                                                       │   │
│  │  - validates payload against JSON schema             │   │
│  │  - generates event_id (UUID v7 for time-ordering)    │   │
│  │  - generates sequence (BIGSERIAL)                    │   │
│  │  - inserts into event_log table in same TX          │   │
│  └──────────────────────────────────────────────────────┘   │
│                         │                                    │
│                         ▼                                    │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  PostgreSQL: event_log table                          │   │
│  │  (primary key: event_id UUID v7)                      │   │
│  │  (index: sequence, type, aggregate_id, created_at)    │   │
│  └──────────────────────────────────────────────────────┘   │
│                         │                                    │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  Pull API (RPC)                                       │   │
│  │                                                       │   │
│  │  yggdrasil-core.event_stream.pull                    │   │
│  │    INPUT: { cursor, filters, limit }                 │   │
│  │    OUTPUT: { events[], next_cursor, has_more }       │   │
│  └──────────────────────────────────────────────────────┘   │
│                         │                                    │
└─────────────────────────┼────────────────────────────────────┘
                          │
                          │ RPC calls
                          ▼
    ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
    │ Integration  │  │   Surface    │  │   Internal   │
    │  subscriber  │  │  subscriber  │  │   consumer   │
    │ (e.g. SIEM   │  │ (e.g. con-   │  │ (e.g. trig-  │
    │  exporter)   │  │  sole activ- │  │  ger engine) │
    │              │  │  ity feed)   │  │              │
    └──────────────┘  └──────────────┘  └──────────────┘
```

### 3.2 Estrutura de um event

```json
{
  "event_id": "018f2b4a-1234-7abc-def0-123456789012",
  "sequence": 4827,
  "type": "manifest.product.created",
  "schema_version": "v1",
  "aggregate_type": "manifest",
  "aggregate_id": "018f2b4a-5678-4abc-def0-987654321098",
  "actor": {
    "type": "collaborator",
    "id": "alice",
    "context": {
      "surface": "yggdrasil-api",
      "request_id": "req-abc123"
    }
  },
  "emitted_at": "2026-04-10T12:34:56.789Z",
  "payload": {
    "manifest_name": "dakasa-platform",
    "manifest_namespace": "dakasa",
    "kind": "product",
    "version": 1,
    "checksum": "sha256:..."
  },
  "metadata": {
    "correlation_id": "workflow-run-xyz",
    "causation_id": "018f2b4a-aaaa-7bbb-cccc-dddddddddddd"
  }
}
```

**Campos obrigatórios (todos events):**

- `event_id` (UUID v7): identidade única, time-ordered (útil para sorting secundário)
- `sequence` (int64): monotonic global counter, PostgreSQL BIGSERIAL
- `type` (string): dotted path, ex: `manifest.product.created`
- `schema_version` (string): versão do schema do payload (v1, v2, ...)
- `aggregate_type` (string): tipo do aggregate (manifest, product, workflow_run, buildproject, topology_node, etc.)
- `aggregate_id` (string|UUID): id do aggregate — per-aggregate ordering é garantido via este campo
- `actor` (object): quem causou o evento (opcional — `null` para events emitted by system background jobs)
- `emitted_at` (RFC 3339 timestamp): momento do emit
- `payload` (object): dados específicos do tipo de evento, validados contra `schema_version`

**Campos opcionais:**

- `metadata.correlation_id`: id opcional para correlacionar events de uma mesma operação lógica
- `metadata.causation_id`: id do event que causou este (para reconstrução de cadeias causais)
- `metadata.*`: qualquer outro metadata

### 3.3 Catálogo inicial de event types (v1)

O MVP emite events em **4 categorias principais**:

#### Category: `manifest.*`

Events de mutações em manifests:

- `manifest.{kind}.created` — um novo manifest foi criado (kind = rbac, policy, integration_type, integration_instance, resource, product, workflow)
- `manifest.{kind}.updated` — uma nova versão de manifest existente foi criada (bump version)
- `manifest.{kind}.deactivated` — um manifest foi soft-deleted (active=false)

**Payload comum (todos manifest.\*):**

```json
{
  "manifest_id": "uuid",
  "kind": "product|workflow|...",
  "namespace": "string",
  "name": "string",
  "version": "int",
  "checksum": "sha256:...",
  "labels": { ... }
}
```

#### Category: `product.*`

Events de lifecycle de products (materialization, installation):

- `product.materialized` — `product.materialize` foi executado com sucesso
- `product.installation.reconciled` — `installation.reconcile` foi executado
- `product.installation.applied` — `installation.apply` foi executado com sucesso
- `product.installation.apply_failed` — `installation.apply` falhou
- `product.installation.observed` — `installation.observe` retornou
- `product.installation.state_discovered` — `installation_state.discover` retornou

#### Category: `workflow.*`

Events de lifecycle de workflow runs:

- `workflow.run.dispatched` — um workflow foi dispatched
- `workflow.run.started` — a execução começou
- `workflow.step.started` — um step específico começou
- `workflow.step.completed` — um step terminou com sucesso
- `workflow.step.failed` — um step falhou (antes de retry esgotar)
- `workflow.step.retry` — um step está sendo retentado
- `workflow.run.completed` — o workflow inteiro terminou com sucesso
- `workflow.run.failed` — o workflow inteiro falhou
- `workflow.run.cancelled` — o workflow foi cancelado (para futuro)

#### Category: `buildproject.*`

Events de lifecycle de BuildProjects:

- `buildproject.created` — criado
- `buildproject.extended` — `expires_at` foi atualizado (futuro: auto-extend on activity)
- `buildproject.expired` — passou da data de expiração e foi marcado como expired
- `buildproject.deleted` — soft-deleted

#### Category: `integration.*`

Events de runtime de integrations:

- `integration.instance.health_changed` — status de health mudou (ex: de `healthy` para `degraded`)
- `integration.operation.executed` — uma operation foi executada (create/update/delete de recurso externo)
- `integration.operation.failed` — uma operation falhou

#### Category: `authorization.*`

Events de decisões de authorization (importantes para audit/compliance):

- `authorization.evaluated` — RBAC+Policy foi avaliado
- `authorization.allowed` — resultado: allow
- `authorization.denied` — resultado: deny

**Payload de authorization.\*:**

```json
{
  "collaborator_id": "alice",
  "resource": "products/dakasa-platform",
  "action": "apply",
  "decision": "allow",
  "matched_roles": ["dakasa-deployer"],
  "matched_rules": ["allow-deploy-dakasa"],
  "rbac_manifest_ref": { "id": "uuid", "name": "...", "namespace": "..." },
  "policy_manifest_ref": { "id": "uuid", "name": "...", "namespace": "..." }
}
```

#### Category: `topology.*`

Events de mutações em topology:

- `topology.node.created`, `topology.node.updated`, `topology.node.deleted`
- `topology.edge.created`, `topology.edge.deleted`
- `topology.document.created`, `topology.document.updated`, `topology.document.deleted`

### 3.4 Schema versioning strategy

- **Every event type has a schema version** (starts at `v1`)
- **v1 is forever non-breaking**: you can add optional fields, you cannot remove, rename, or change types of existing fields
- **Breaking changes create v2**: old events keep `schema_version: "v1"`, new events start emitting `schema_version: "v2"`
- **Subscribers declare supported versions**: pull RPC can filter `supported_versions: ["v1", "v2"]`; events with unsupported versions are skipped
- **JSON Schemas published in `yggdrasil-core/docs/contracts/events/{version}/`**: each type has its own schema file

## 4. Persistência

### 4.1 Tabela `event_log`

```sql
CREATE TABLE event_log (
    event_id        UUID PRIMARY KEY,
    sequence        BIGSERIAL NOT NULL UNIQUE,
    type            VARCHAR(128) NOT NULL,
    schema_version  VARCHAR(16) NOT NULL DEFAULT 'v1',
    aggregate_type  VARCHAR(64) NOT NULL,
    aggregate_id    VARCHAR(128) NOT NULL,
    actor_type      VARCHAR(32),
    actor_id        VARCHAR(128),
    actor_context   JSONB,
    emitted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    payload         JSONB NOT NULL,
    metadata        JSONB,

    CONSTRAINT event_log_payload_not_null CHECK (payload IS NOT NULL)
);

-- Primary read pattern: cursor-based pull with filters
CREATE INDEX event_log_sequence_idx ON event_log (sequence);
CREATE INDEX event_log_type_sequence_idx ON event_log (type, sequence);
CREATE INDEX event_log_aggregate_idx ON event_log (aggregate_type, aggregate_id, sequence);
CREATE INDEX event_log_emitted_at_idx ON event_log (emitted_at);

-- For TTL cleanup
CREATE INDEX event_log_type_emitted_idx ON event_log (type, emitted_at);
```

**Notas:**

- `sequence BIGSERIAL` dá monotonic global ordering dentro de um PostgreSQL database. Múltiplos yggdrasil-core workers conectados à mesma DB compartilham o mesmo sequence.
- `event_id` é UUID v7 (time-ordered), útil para sort secundário e não tem colisão cross-worker.
- `emitted_at` é set at insert time (server default `NOW()`); **não** é set pelo caller para evitar clock skew.
- Para cross-aggregate ordering forte (se alguém quiser), o `sequence` serve como tiebreaker determinístico.

### 4.2 Inserção transacional

```go
// repository/events.go

type EventRepository struct {
    db *sql.DB
}

type EmitEventRequest struct {
    Type           string
    SchemaVersion  string
    AggregateType  string
    AggregateID    string
    Actor          *EventActor
    Payload        map[string]any
    Metadata       map[string]any
}

func (r *EventRepository) Emit(
    ctx context.Context,
    tx *sql.Tx,  // MUST be called from within a transaction to ensure atomicity with mutation
    req EmitEventRequest,
) (eventID uuid.UUID, err error) {
    // 1. Validate payload against registered schema for req.Type + req.SchemaVersion
    if err := contractdocs.ValidateEvent(req.Type, req.SchemaVersion, req.Payload); err != nil {
        return uuid.Nil, fmt.Errorf("event payload validation: %w", err)
    }

    // 2. Generate event_id (UUID v7)
    eventID, err = uuid.NewV7()
    if err != nil {
        return uuid.Nil, err
    }

    // 3. Insert into event_log (sequence auto-assigned)
    payloadJSON, _ := json.Marshal(req.Payload)
    metadataJSON, _ := json.Marshal(req.Metadata)

    var actorType, actorID sql.NullString
    var actorContextJSON []byte
    if req.Actor != nil {
        actorType = sql.NullString{String: req.Actor.Type, Valid: true}
        actorID = sql.NullString{String: req.Actor.ID, Valid: true}
        actorContextJSON, _ = json.Marshal(req.Actor.Context)
    }

    _, err = tx.ExecContext(ctx, `
        INSERT INTO event_log (
            event_id, type, schema_version, aggregate_type, aggregate_id,
            actor_type, actor_id, actor_context, payload, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    `, eventID, req.Type, req.SchemaVersion, req.AggregateType, req.AggregateID,
       actorType, actorID, actorContextJSON, payloadJSON, metadataJSON)

    if err != nil {
        return uuid.Nil, fmt.Errorf("insert event_log: %w", err)
    }

    return eventID, nil
}
```

**Pontos-chave:**

- A função **exige `*sql.Tx`** — não cria transação própria. O caller (handler de mutação) passa sua transação existente. Garantia: event só existe se mutação committar.
- Schema validation acontece antes do INSERT para rejeitar events malformados cedo.
- Sequence é auto-gerado pela DB (BIGSERIAL); não precisa computar client-side.

## 5. API de subscription (Pull RPC)

### 5.1 RPC queue

`yggdrasil-core.event_stream.pull`

### 5.2 Request schema (JSON Schema em `docs/contracts/rpc/event_stream/v1/pull_request.schema.json`)

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["cursor", "limit"],
  "properties": {
    "cursor": {
      "type": "object",
      "description": "Position in the event stream. First call uses {sequence: 0} to start from beginning.",
      "required": ["sequence"],
      "properties": {
        "sequence": {
          "type": "integer",
          "minimum": 0,
          "description": "Exclusive lower bound. Returns events with sequence > this value."
        }
      }
    },
    "limit": {
      "type": "integer",
      "minimum": 1,
      "maximum": 1000,
      "description": "Max events to return in this call."
    },
    "filters": {
      "type": "object",
      "description": "Optional filters. All filters are AND-combined.",
      "properties": {
        "types": {
          "type": "array",
          "items": { "type": "string" },
          "description": "Only return events whose type matches one of these patterns. Supports wildcards (e.g., 'manifest.*', 'workflow.run.*')."
        },
        "aggregate_type": {
          "type": "string",
          "description": "Only events for this aggregate type."
        },
        "aggregate_id": {
          "type": "string",
          "description": "Only events for this specific aggregate id."
        },
        "supported_schema_versions": {
          "type": "array",
          "items": { "type": "string" },
          "description": "Only events whose schema_version is in this list. Events with unknown versions are skipped."
        },
        "emitted_after": {
          "type": "string",
          "format": "date-time",
          "description": "Only events emitted after this timestamp (UTC)."
        }
      }
    }
  }
}
```

### 5.3 Response schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["events", "next_cursor", "has_more"],
  "properties": {
    "events": {
      "type": "array",
      "items": {
        "$ref": "https://yggdrasil.io/contracts/events/v1/event.schema.json"
      }
    },
    "next_cursor": {
      "type": "object",
      "required": ["sequence"],
      "properties": {
        "sequence": { "type": "integer" }
      },
      "description": "Cursor to pass in the next pull call to continue from where this call left off."
    },
    "has_more": {
      "type": "boolean",
      "description": "True if there are more events available beyond next_cursor (i.e., the response was limit-truncated). Subscribers can use this to decide whether to pull again immediately or wait."
    }
  }
}
```

### 5.4 Cursor semantics

- Subscriber starts with `{sequence: 0}`
- Server returns events with `sequence > cursor.sequence` in ascending sequence order
- Server returns `next_cursor.sequence = last_event.sequence` (or original `cursor.sequence` if no events)
- Subscriber persists `next_cursor` and uses it for the next pull
- Filters are applied server-side; `next_cursor` reflects the last event *returned* after filtering — not the last event *in the log*. This means subscribers with narrow filters may still see `has_more: false` even if there are events they filtered out beyond their cursor. **(Caveat: this is a simplification; see §5.5 for the edge case.)**

### 5.5 Filter-aware cursor (edge case)

When filters are applied, the cursor needs to be "aware" of what was filtered. The simplest approach is:

- Server queries with `WHERE sequence > cursor AND {filters...} ORDER BY sequence LIMIT N`
- Sets `next_cursor.sequence = max(returned events' sequence)` OR if none returned, `next_cursor.sequence = cursor.sequence` (unchanged)
- Sets `has_more = true` if `count(returned) == limit`, else `false`

This means subscribers with narrow filters may do many empty pulls if most events are filtered out. For very narrow filters, this becomes inefficient. **Solution for future:** add an optimization where `next_cursor.sequence = max(sequence) FROM event_log WHERE {filters...} AND sequence > cursor AND sequence <= last_returned_sequence`, i.e., the cursor advances to the last event *that matched the filter*. This is a small SQL change for v2 of the API.

**MVP decision:** ship the simple version; revisit if filters-with-sparse-matches is a problem in practice.

### 5.6 Example subscriber flow

```python
# Python example (language-agnostic)
import requests

cursor = {"sequence": 0}
while True:
    response = call_yggdrasil_rpc("event_stream.pull", {
        "cursor": cursor,
        "limit": 100,
        "filters": {
            "types": ["authorization.*", "manifest.*"],
            "supported_schema_versions": ["v1"]
        }
    })
    for event in response["events"]:
        process(event)
    cursor = response["next_cursor"]
    if not response["has_more"]:
        time.sleep(5)  # no more events right now, wait before polling again
```

## 6. Retention policy

### 6.1 Configuration schema

Retention é configurado via **tabela `event_retention_policy`** no PostgreSQL (não via arquivo de config, porque deve ser alterável em runtime sem restart):

```sql
CREATE TABLE event_retention_policy (
    type_pattern    VARCHAR(128) PRIMARY KEY,  -- e.g., 'manifest.*', 'authorization.*', '*' (default)
    ttl_days        INTEGER NOT NULL CHECK (ttl_days >= 0),  -- 0 means infinite retention
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Default policies seeded at migration time:
INSERT INTO event_retention_policy (type_pattern, ttl_days) VALUES
    ('*', 90),                       -- default: 90 days
    ('authorization.*', 2555),       -- 7 years for compliance
    ('manifest.*', 0),               -- forever (manifest history is canonical)
    ('buildproject.*', 365),         -- 1 year for lifecycle events
    ('workflow.step.*', 30),         -- step-level events are verbose; 30 days
    ('workflow.run.*', 180);         -- run-level events kept longer
```

**Matching precedence:**
- More specific pattern wins (`manifest.product.created` > `manifest.*` > `*`)
- If no pattern matches, fall back to `*` default

### 6.2 Cleanup job

Background loop em yggdrasil-core (addon `event_log_cleaner`):

```go
func cleanupExpiredEvents(ctx context.Context, db *sql.DB) error {
    // For each policy, delete events older than TTL
    rows, err := db.QueryContext(ctx, `SELECT type_pattern, ttl_days FROM event_retention_policy WHERE ttl_days > 0`)
    defer rows.Close()
    for rows.Next() {
        var pattern string
        var ttlDays int
        rows.Scan(&pattern, &ttlDays)

        // Convert pattern (e.g., 'manifest.*') to SQL LIKE
        sqlPattern := strings.ReplaceAll(pattern, "*", "%")

        _, err := db.ExecContext(ctx, `
            DELETE FROM event_log
            WHERE type LIKE $1
              AND emitted_at < NOW() - ($2 || ' days')::interval
        `, sqlPattern, ttlDays)
        if err != nil {
            return err
        }
    }
    return nil
}
```

Roda a cada 1 hora (configurável via env var). Não é crítico se pular uma execução — events só ficam extras. Idempotente, safe-to-retry.

### 6.3 Alteração de retention em runtime

Admins (via surface ou direct SQL) podem atualizar `event_retention_policy`. Mudanças tomam efeito no próximo ciclo de cleanup (até 1h depois).

**Caveat:** reduzir TTL de um pattern faz events existentes se tornarem elegíveis para deletion no próximo cleanup. Increasing TTL não recupera events já deletados (não há undo). Admins devem tomar cuidado ao reduzir TTLs.

## 7. Contract files (JSON Schema)

Esses arquivos serão criados em `yggdrasil-core/docs/contracts/events/v1/`:

```
docs/contracts/events/v1/
├── event.schema.json                        # Base schema (common fields for all events)
├── manifest/
│   ├── manifest.created.schema.json         # (kind-agnostic; kind is in payload)
│   ├── manifest.updated.schema.json
│   └── manifest.deactivated.schema.json
├── product/
│   ├── product.materialized.schema.json
│   ├── product.installation.applied.schema.json
│   ├── product.installation.apply_failed.schema.json
│   └── ...
├── workflow/
│   ├── workflow.run.dispatched.schema.json
│   ├── workflow.run.started.schema.json
│   ├── workflow.step.started.schema.json
│   ├── workflow.step.completed.schema.json
│   ├── workflow.step.failed.schema.json
│   ├── workflow.run.completed.schema.json
│   └── workflow.run.failed.schema.json
├── buildproject/
│   ├── buildproject.created.schema.json
│   ├── buildproject.extended.schema.json
│   └── buildproject.expired.schema.json
├── integration/
│   ├── integration.instance.health_changed.schema.json
│   └── integration.operation.executed.schema.json
├── authorization/
│   └── authorization.evaluated.schema.json
└── topology/
    ├── topology.node.created.schema.json
    ├── topology.node.updated.schema.json
    └── ...
```

Cada schema é um arquivo individual, language-agnostic, usado tanto para validation at emit time quanto para documentação para consumers.

## 8. Migração / implementação faseada

### Fase 1 — Infraestrutura base

- [ ] Migration `00NNN_event_log.sql` (tabela `event_log` + `event_retention_policy` + índices)
- [ ] `repository/events.go` com `Emit(tx, req)` function
- [ ] `docs/contracts/events/v1/event.schema.json` (base schema)
- [ ] `contractdocs.ValidateEvent(type, version, payload)` helper
- [ ] Background job `cleanupExpiredEvents` rodando a cada 1h

### Fase 2 — Primeiros event types + RPC pull

- [ ] JSON Schemas para os 4 tipos mais críticos:
  - `manifest.created`
  - `product.installation.applied`
  - `workflow.run.completed`
  - `authorization.evaluated`
- [ ] Handlers de mutação correspondentes chamam `Emit()` dentro da tx
- [ ] RPC handler `event_stream.pull` com filters + cursor
- [ ] Integration tests cobrindo: emit → persist → pull → filter → cursor advance

### Fase 3 — Catálogo completo

- [ ] Schemas para todos os event types listados em §3.3
- [ ] Handlers de mutação cobrem todos os tipos
- [ ] Documentação em `docs/contracts/events/v1/README.md` explicando como consumer events em qualquer linguagem

### Fase 4 — Integration/surface consumers (paralelo com Fase 3)

- [ ] yggdrasil-console ganha uma "Activity" page que pull events e mostra stream
- [ ] Primeiro integration subscriber-ready: `integration-event-audit-exporter` (pull e envia para SIEM)
- [ ] CLI `ygg events pull --types manifest.* --limit 100` para debug

### Fase 5 — Validação em produção

- [ ] Deploy em ambiente de staging, observar volume de events, latência de pull, performance de queries
- [ ] Ajustar índices conforme necessário
- [ ] Documentar performance expectations (events/s, latência de pull, growth rate de event_log)

## 9. Considerações de performance

### 9.1 Volume esperado

Estimativa conservadora para um ecossistema médio (ex: DaKasa em validation):

- ~50 workflows/day executando, com ~10 steps cada = 500 step events/day = ~15,000/month
- ~100 manifest mutations/day = 3,000/month
- ~500 authorization decisions/day = 15,000/month
- ~10 buildproject lifecycle events/day = 300/month
- **Total: ~35,000 events/month** em ambiente pequeno

Para ecossistema grande (1000x a carga): 35 milhões de events/month. PostgreSQL suporta facilmente com particionamento por mês se necessário.

### 9.2 Índices + query performance

Pull típico com filtro `type='manifest.*' AND sequence > X LIMIT 100`:

```sql
SELECT * FROM event_log
WHERE sequence > $1
  AND type LIKE 'manifest.%'
ORDER BY sequence
LIMIT 100;
```

Com índice `event_log_type_sequence_idx (type, sequence)`, essa query é O(log N + limit). Deve responder em <10ms para milhões de rows.

### 9.3 Write amplification

Cada mutação significa: INSERT na tabela da entidade + INSERT em `event_log`. Duplicação de write volume. Aceito porque:
- PostgreSQL bulk INSERTs são baratos
- Committing na mesma tx tem overhead mínimo
- Valor de auditability justifica

### 9.4 Partitioning (futuro)

Para deploys muito grandes, `event_log` pode ser particionada por mês usando PostgreSQL declarative partitioning:

```sql
CREATE TABLE event_log (...) PARTITION BY RANGE (emitted_at);
CREATE TABLE event_log_2026_04 PARTITION OF event_log FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
-- etc.
```

Vantagens: DROP PARTITION é instantâneo (limpeza de retention); queries com timestamp filter tocam menos partitions; storage menor por partition.

**MVP decision**: sem particionamento inicial. Adicionar quando volume justificar.

## 10. Segurança e permissions

### 10.1 Quem pode pull events

Pull RPC **não** é público em trust-boundary mode. O modelo é:

- Surfaces (yggdrasil-api, yggdrasil-console, custom) são o trust boundary. Core confia em qualquer caller da sua RPC — assume que a surface já fez auth.
- Se o operator quiser que só certos subscribers possam pull certos event types, isso é **policy no nível da surface**, não do core. Ex: yggdrasil-api pode ter middleware que valida se o collaborator tem role `event_reader` antes de fazer a call ao core.
- Core pode (opcionalmente) validar um `auth` field no request e aplicar RBAC/Policy sobre o pull — mesmo modelo que outras RPCs. Quando `auth.collaborator_id` está presente, core valida `read` action sobre `events/{type_pattern}` via manifest RBAC.

**MVP decision:** core não enforça policies no pull (trust boundary). Se um subscriber quer policies, é responsabilidade da surface fazer a call ao core em nome do collaborator e filtrar/rejeitar baseado em policy.

### 10.2 Sensitive data em payloads

Alguns events podem carregar dados sensíveis (ex: `authorization.denied` com reasoning que menciona o recurso denied, ou `manifest.integration_instance.created` com secret refs).

**Convenção:**

- Events **nunca** incluem secret values in clear. Apenas `secret_refs` (pointers).
- Events de manifest incluem `checksum` mas não payload completo; consumers que precisam do payload pull via `manifest.get`.
- Events de authorization incluem a decision, mas não o `input` dict completo (apenas `resource` e `action`).

Essa convenção é enforced via **schema validation** — schemas rejeitam campos como `credentials`, `password`, `api_key` em event payloads.

## 11. Compatibilidade com yggdrasil-core atual

### 11.1 O que muda

- **Nova migration** (`00NNN_event_log.sql`) — não quebra nada, só adiciona tabelas
- **Nova dependency** em `repository/events.go` importada pelos handlers que mutam estado
- **Handlers existentes** precisam chamar `events.Emit(tx, ...)` dentro das transações existentes — alteração cirúrgica, não refactor
- **Novo RPC queue** (`yggdrasil-core.event_stream.pull`) — registrado como novo consumer em `addons/rabbitmq.go`

### 11.2 O que NÃO muda

- Schema de manifests, products, workflows existentes: intacto
- RPCs existentes: intactas
- `product_materializations`, `integration_runtime_state` tables: mantidas (são específicas, event stream é geral, ambos podem coexistir durante transição)

### 11.3 Backfill de events antigos

Events pré-existentes (manifests criados antes desta feature) **não** são backfilled. O event stream começa do zero no momento do deploy. Consumers que queiram histórico anterior devem ler as tabelas originais (manifests, product_materializations, etc.).

**Por que não backfillar:** backfill introduz risco de events "fabricados" com timestamps incorretos, actors desconhecidos, etc. Melhor começar limpo.

## 12. Alternativas consideradas

### 12.1 Usar PostgreSQL LISTEN/NOTIFY em vez de pull

PostgreSQL tem `LISTEN/NOTIFY` para push notifications. Poderiam ser usadas em vez de polling.

**Rejeitado porque:**
- LISTEN/NOTIFY tem payload limit de 8KB (precisaria wrappear events)
- Notifications não são durable — se um subscriber cair, perde notifications enquanto estiver offline
- Force usar uma conexão PG dedicada por subscriber (escalabilidade)
- Pull-based é mais simples e mais universal (funciona com qualquer client language)

### 12.2 Usar RabbitMQ fanout exchange

Emitir events como messages em um fanout exchange do RabbitMQ.

**Rejeitado porque:**
- Amarra event stream ao RabbitMQ especificamente. Core deve ser transport-agnostic (hoje o único transport é RabbitMQ, mas a filosofia é plugável).
- Non-durable por default; seria preciso setup de queues durables por consumer
- Consumer offline perde messages durante downtime
- PostgreSQL como source-of-truth é mais simples e mais durável

### 12.3 Usar Kafka/NATS externo

Publicar events em Kafka ou NATS Streaming externo.

**Rejeitado porque:**
- Adiciona dependency externa pesada ao core
- Core deve ser rodável com só PostgreSQL + (optional) RabbitMQ. Kafka é muito maior
- Consumer de Kafka em qualquer linguagem é non-trivial; JSON Schema-based pull é simples

### 12.4 Event sourcing puro (events são o source of truth)

Em vez de ter tabelas `manifests`, `products`, etc., ter apenas `event_log` e reconstruir state das projeções.

**Rejeitado porque:**
- Refactoring massivo de todo o core
- Queries para current state ficam muito mais lentas (precisam fold events)
- Yggdrasil funciona bem com state-based primitives; event stream é aditivo, não substituto

## 13. Riscos e mitigações

| Risco | Impacto | Mitigação |
|---|---|---|
| Write amplification derruba performance do core | Médio | Benchmark em Fase 5; índices otimizados; particionamento como escape hatch |
| Consumers lentos afetam produção de events | Baixo | Pull-based: consumers independentes. Core não espera por ninguém. |
| Schema drift (events em v1 que consumers antigos não entendem) | Médio | Schema versioning explícito; `supported_schema_versions` filter; docs claras sobre backwards compatibility |
| Events sensíveis vazam dados | Alto | Convenção em §10.2; schema validation bloqueia campos sensíveis; review em PR ao adicionar novos event types |
| Cleanup policy agressiva deleta events antes de subscribers consumirem | Médio | Default conservador (90 dias); admins podem ver em tabela `event_retention_policy` antes de reduzir; alertas quando retention é reduzida |
| Sequence gaps se transações são rolled back | Baixo | Sequence gaps são OK; subscribers não devem assumir contiguidade, apenas monotonicidade. Documentar isto claramente |
| Pull com filtros muito específicos causa empty pulls recorrentes | Baixo | Otimização §5.5 é fácil de adicionar se necessário; MVP aceita |

## 14. Critérios de aceitação

Para o event stream ser considerado "pronto" (MVP):

- ✅ Tabela `event_log` + `event_retention_policy` criadas via migration
- ✅ `repository.Emit(tx, req)` funcional, com schema validation
- ✅ 4+ event types implementados (manifest.created, product.installation.applied, workflow.run.completed, authorization.evaluated)
- ✅ RPC `event_stream.pull` funcional com filters + cursor
- ✅ JSON Schemas publicados em `docs/contracts/events/v1/`
- ✅ Background cleanup job rodando a cada 1h
- ✅ Testes de integração cobrindo: emit → persist → pull → filter → cursor advance → retention cleanup
- ✅ Documentação para consumers em qualquer linguagem (README em `docs/contracts/events/v1/`)
- ✅ Primeira projeção funcional (activity page no yggdrasil-console OU primeiro integration subscriber)

## 15. Dependências de outros gaps

**Este gap não depende de nenhum outro gap.** É a primeira spec a ser implementada (fundacional).

**Outros gaps que dependem deste:**

- Gap 4 (BuildProject lifecycle enforcement) — emite events `buildproject.*`
- Gap 5 (Workflow trigger system) — events é a infraestrutura para "event-driven triggers" e subscribers internos do scheduler
- Projections futuras: audit log, workflow run history, activity feed, etc.

## 16. Pontos em aberto

- **Exact set of event types para v1**: lista em §3.3 é inicial. Pode crescer à medida que consumers pedem eventos específicos. Adicionar novos types é non-breaking (novos schemas).
- **Multi-tenancy em retention**: hoje retention é global. Futuro: retention policies podem ser namespaced (ex: events de namespace `dakasa` têm retention X, de namespace `outro-tenant` têm Y). Decidir quando multi-tenancy real for priorizada.
- **Causation chain vs correlation ID**: spec prevê ambos em metadata, mas convenção de uso não é estrita. Documentar padrões conforme usage real.
- **Delete vs soft-delete no cleanup**: hoje é hard delete. Se algum caso de uso pedir "restore de events deletados por erro", considerar soft-delete com flag `deleted_at`.
- **Ordering cross-aggregate forte**: aceito como tradeoff. Se algum caso de uso exigir total ordering (além de monotonic sequence), discutir.

## 17. Resumo executivo

- **Feature**: state change event stream no core, durável, pull-based, com retention configurável
- **Fundacional**: outras 4 features do core (BuildProject lifecycle, workflow trigger system, audit log projection, workflow run history projection) se constroem sobre ela
- **Filosofia alinhada**: core emite, integrations/surfaces consomem; contract-first JSON Schema; language-agnostic
- **Persistência**: tabela `event_log` em PostgreSQL, transacional com mutations
- **API**: RPC `event_stream.pull` com cursor + filters
- **Trade-offs aceitos**: não é real-time (pull-based), write amplification, cross-aggregate ordering não garantido
- **Implementação**: 5 fases incrementais; MVP completo quando os critérios de aceitação em §14 forem atendidos
