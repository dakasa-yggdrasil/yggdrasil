# BuildProject Lifecycle Enforcement — yggdrasil-core

**Data:** 2026-04-10
**Status:** Spec em revisão
**Tipo:** Core state lifecycle enforcement
**Prioridade:** 🔴 Alta — sem isso, ephemerais vazam e a filosofia "descartáveis" se quebra
**Escopo:** Fazer o core enforçar lifecycle de `BuildProject.Ephemeral=true` automaticamente — background loop expira projects passados de `ExpiresAt`, dispara teardown workflows, emite events, e marca como deleted.
**Depende de:** [Event Stream](./2026-04-10-event-stream-design.md) (para emitir events de lifecycle)
**Parte do audit report:** [`2026-04-10-yggdrasil-product-audit-report.md`](./2026-04-10-yggdrasil-product-audit-report.md) Gap 4

---

## 1. Contexto e motivação

### 1.1 O problema

`BuildProject` no model do core (em `yggdrasil-core/model/topology.go`) já tem campos:

```go
type BuildProject struct {
    // ...
    Ephemeral bool   // marked as temporary
    ExpiresAt string // when should expire (RFC 3339 timestamp)
    Immutable bool
}
```

Esses campos declaram a **intenção**: "este build project é ephemeral e deve expirar em X". Mas **nada no core enforça** essa intenção. Se um BuildProject ephemeral é criado com `ExpiresAt = "2026-04-12T00:00:00Z"`, ele permanece no sistema para sempre a menos que alguém externo o delete manualmente.

Isso fere a filosofia fundamental: **"ecossistemas devem se tornar banais e descartáveis"**. Se ephemerais não expirarem automaticamente, a promessa de "descartável" é uma mentira — é um TODO eterno para o usuário.

### 1.2 Por que é core

- **Não pode ser integration**: integrations são externas ao core. Uma integration-cleanup externa precisaria consultar o core periodicamente perguntando "qual BuildProject expirou?", depois disparar ações. Isso delega state management do core a algo externo, violando a filosofia "core gerencia estado com maestria".
- **Não pode ser surface**: surfaces expõem state, não o gerenciam.
- **Tem que ser core**: enforcement de lifecycle de entidades do core é responsabilidade do core. BuildProject é uma entity interna; seu lifecycle é interno.

### 1.3 Filosofia alinhada

"Core gerencia estado com maestria" inclui **cumprir as promessas feitas nos schemas**. Se o schema diz `Ephemeral + ExpiresAt`, core deve enforçar. Do contrário, o schema é decorativo.

## 2. Design

### 2.1 Princípios

1. **Background loop** no core, rodando periodicamente, verifica BuildProjects elegíveis para expiração.
2. **Expiração não é deleção imediata**: core marca como `expired` e emite um event. Deleção (hard delete) pode ser feita por um teardown workflow configurável, ou após um grace period opcional.
3. **Teardown workflow configurável**: cada BuildProject pode declarar qual workflow rodar quando expira (ex: "drop databases", "cleanup S3 prefix", "remove DNS records"). Default: nenhum workflow, só marca como deleted.
4. **Idempotência**: o loop é safe-to-retry. Rodar o loop 2x não causa dupla expiração.
5. **Events emitidos no event stream**: cada transição de estado vira um event que consumers (integrations, surfaces) podem observar.
6. **Não bloqueia operations in-flight**: se um BuildProject está em meio de um product apply, a expiração espera. Evita race conditions.

### 2.2 Estados do lifecycle

```
┌───────────┐  create  ┌──────────┐  extend   ┌──────────┐
│ (does not │ ───────▶ │  active  │ ─────────▶│  active  │
│   exist)  │          │          │ (new ExpAt)│          │
└───────────┘          └──────────┘           └──────────┘
                             │                      │
                             │ ExpiresAt < now      │
                             ▼                      │
                      ┌──────────────┐              │
                      │   expiring   │ ◀────────────┘
                      │ (teardown    │
                      │  in progress)│
                      └──────────────┘
                             │
                             │ teardown success
                             ▼
                      ┌──────────────┐
                      │   deleted    │
                      │ (soft delete)│
                      └──────────────┘
                             │
                             │ retention grace period (configurable)
                             ▼
                      ┌──────────────┐
                      │  hard-deleted│
                      │  (row gone)  │
                      └──────────────┘
```

**Estados formais:**

- `active` — BuildProject válido, não expirou
- `expiring` — detectado como expired, teardown workflow em progresso (se houver)
- `deleted` — soft-deleted; row ainda existe na DB mas marcado como deleted
- `hard-deleted` — row removida da DB

### 2.3 Extensão do schema de BuildProject

Adicionar campos ao model:

```go
type BuildProject struct {
    ID                   uuid.UUID
    InfraMapID           uuid.UUID
    ProjectEnvResourceID uuid.UUID
    BuildName            string
    EnvType              string
    Cloud                string
    Ephemeral            bool
    ExpiresAt            string
    ClusterName          string
    ClusterZone          string
    Immutable            bool
    
    // NEW fields:
    LifecycleStatus      string          // "active" | "expiring" | "deleted"
    TeardownWorkflowRef  *ManifestSelector // optional; workflow to run when expiring
    ExpiringStartedAt    *time.Time      // when expiring state was entered
    DeletedAt            *time.Time      // when soft-delete happened
    ExtendedAt           *time.Time      // when ExpiresAt was last updated (for "extend on activity" patterns)
    TeardownRunID        string          // ID of the workflow run that's doing teardown (for idempotency)
}
```

**Campos novos:**

- `LifecycleStatus` — estado atual do lifecycle (default `"active"`)
- `TeardownWorkflowRef` — workflow ref opcional para rodar on expiration. Workflow recebe `{ buildproject_id, buildproject_name, env_type, cloud, cluster_name, ... }` como inputs
- `ExpiringStartedAt`, `DeletedAt`, `ExtendedAt` — timestamps para audit + debug
- `TeardownRunID` — para garantir idempotência (mesmo BuildProject nunca dispara teardown duas vezes)

### 2.4 Migration

```sql
ALTER TABLE topology_build_projects
    ADD COLUMN lifecycle_status VARCHAR(32) NOT NULL DEFAULT 'active',
    ADD COLUMN teardown_workflow_namespace VARCHAR(128),
    ADD COLUMN teardown_workflow_name VARCHAR(128),
    ADD COLUMN expiring_started_at TIMESTAMPTZ,
    ADD COLUMN deleted_at TIMESTAMPTZ,
    ADD COLUMN extended_at TIMESTAMPTZ,
    ADD COLUMN teardown_run_id VARCHAR(128);

-- Index para o background loop encontrar expired projects rápido
CREATE INDEX topology_build_projects_expiring_idx
    ON topology_build_projects (lifecycle_status, expires_at)
    WHERE ephemeral = true AND lifecycle_status = 'active';
```

### 2.5 Background loop (o "lifecycle enforcer")

Novo addon no core: `addons/buildproject_lifecycle.go`.

```go
func init() {
    Register("buildproject_lifecycle", bootstrapBuildProjectLifecycle, 30)
}

func bootstrapBuildProjectLifecycle(ctx context.Context, app *runtime.ServiceApp) error {
    db, ok := Postgres(app)
    if !ok {
        return fmt.Errorf("postgres addon is not available")
    }

    interval := buildProjectLifecycleInterval()
    stopLoop := startBuildProjectLifecycleLoop(ctx, db, interval)
    app.RegisterCloser(func(context.Context) error {
        stopLoop()
        return nil
    })

    return nil
}

func startBuildProjectLifecycleLoop(ctx context.Context, db *sql.DB, interval time.Duration) func() {
    ticker := time.NewTicker(interval)
    stop := make(chan struct{})

    go func() {
        for {
            select {
            case <-stop:
                return
            case <-ticker.C:
                if err := runLifecyclePass(ctx, db); err != nil {
                    logger.Warn("lifecycle pass failed", zap.Error(err))
                }
            }
        }
    }()

    return func() {
        close(stop)
        ticker.Stop()
    }
}
```

### 2.6 `runLifecyclePass` — um pass do loop

```go
func runLifecyclePass(ctx context.Context, db *sql.DB) error {
    // Step 1: Find active ephemeral BuildProjects past ExpiresAt
    candidates, err := findExpiredCandidates(ctx, db)
    if err != nil {
        return err
    }

    for _, bp := range candidates {
        if err := transitionToExpiring(ctx, db, bp); err != nil {
            // Log and continue; don't let one failure block others
            logger.Warn("failed to transition to expiring", zap.String("id", bp.ID.String()), zap.Error(err))
        }
    }

    // Step 2: Find 'expiring' BuildProjects whose teardown completed
    expiring, err := findExpiringWithCompletedTeardown(ctx, db)
    for _, bp := range expiring {
        if err := transitionToDeleted(ctx, db, bp); err != nil {
            logger.Warn("failed to transition to deleted", zap.String("id", bp.ID.String()), zap.Error(err))
        }
    }

    // Step 3: Hard-delete 'deleted' projects past retention (if retention configured)
    if buildProjectHardDeleteRetentionDays > 0 {
        retentionCutoff := time.Now().Add(-time.Duration(buildProjectHardDeleteRetentionDays) * 24 * time.Hour)
        if err := hardDeleteOldDeleted(ctx, db, retentionCutoff); err != nil {
            logger.Warn("hard-delete pass failed", zap.Error(err))
        }
    }

    return nil
}
```

### 2.7 `transitionToExpiring`

```go
func transitionToExpiring(ctx context.Context, db *sql.DB, bp BuildProject) error {
    tx, err := db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // 1. Atomically update lifecycle_status from 'active' to 'expiring'
    //    WHERE lifecycle_status = 'active' AND expires_at < NOW()
    //    ensures idempotency: if another worker already transitioned, UPDATE affects 0 rows
    result, err := tx.ExecContext(ctx, `
        UPDATE topology_build_projects
        SET lifecycle_status = 'expiring',
            expiring_started_at = NOW()
        WHERE id = $1
          AND lifecycle_status = 'active'
          AND ephemeral = true
          AND expires_at::timestamptz < NOW()
    `, bp.ID)
    if err != nil {
        return err
    }
    affected, _ := result.RowsAffected()
    if affected == 0 {
        // Another worker already picked up; skip
        return nil
    }

    // 2. Emit event to event stream (same transaction)
    events.Emit(tx, EmitEventRequest{
        Type:          "buildproject.expired",
        SchemaVersion: "v1",
        AggregateType: "buildproject",
        AggregateID:   bp.ID.String(),
        Actor:         &EventActor{Type: "system", ID: "buildproject-lifecycle-loop"},
        Payload: map[string]any{
            "buildproject_id": bp.ID,
            "name":            bp.BuildName,
            "env_type":        bp.EnvType,
            "cloud":           bp.Cloud,
            "expires_at":      bp.ExpiresAt,
            "teardown_workflow_ref": bp.TeardownWorkflowRef,
        },
    })

    // 3. If teardown workflow is configured, dispatch it
    if bp.TeardownWorkflowRef != nil {
        runID, err := dispatchTeardownWorkflow(ctx, tx, bp)
        if err != nil {
            return err
        }
        // Save run_id for later tracking
        _, err = tx.ExecContext(ctx, `
            UPDATE topology_build_projects
            SET teardown_run_id = $1
            WHERE id = $2
        `, runID, bp.ID)
        if err != nil {
            return err
        }
    }

    return tx.Commit()
}
```

**Pontos-chave:**

- Transição é atomic via SQL UPDATE com WHERE conditions (optimistic locking)
- Se múltiplos workers do core tentarem transicionar o mesmo BP, apenas um ganha (outros vêem `affected = 0`)
- Event é emitido na mesma transaction — audit trail garantido
- Teardown workflow dispatch também é na mesma tx — se falhar, tudo rola back

### 2.8 Teardown workflow dispatch

Quando `TeardownWorkflowRef` está configurado, o lifecycle loop dispatcha o workflow passando contexto do BuildProject como inputs.

```go
func dispatchTeardownWorkflow(ctx context.Context, tx *sql.Tx, bp BuildProject) (string, error) {
    req := DispatchWorkflowRequest{
        WorkflowRef: bp.TeardownWorkflowRef,
        Inputs: map[string]any{
            "buildproject_id":   bp.ID.String(),
            "buildproject_name": bp.BuildName,
            "env_type":          bp.EnvType,
            "cloud":             bp.Cloud,
            "cluster_name":      bp.ClusterName,
            "cluster_zone":      bp.ClusterZone,
        },
        Metadata: map[string]any{
            "triggered_by":  "buildproject_lifecycle_loop",
            "buildproject_expired_at": bp.ExpiresAt,
        },
    }
    // The workflow dispatch is async; we only need the run_id back
    return workflowDispatcher.Dispatch(ctx, req)
}
```

O teardown workflow é responsabilidade do **autor do BuildProject** escrever. Exemplo típico para ephemeral PR env:

```json
{
  "kind": "workflow",
  "metadata": { "name": "teardown-pr-environment" },
  "spec": {
    "trigger": { "mode": "manual" },
    "input_schema": {
      "required": ["buildproject_name"],
      "properties": {
        "buildproject_name": { "type": "string" }
      }
    },
    "steps": [
      {
        "id": "drop-logical-databases",
        "use": { "kind": "integration", "instance_ref": { "name": "aws-dakasa-validation" }, "operation": "execute_sql" },
        "with": {
          "statement": "DROP DATABASE dakasa_identities_{{ inputs.buildproject_name }}",
          "database": "postgres"
        }
      },
      {
        "id": "delete-kubernetes-namespace",
        "depends_on": ["drop-logical-databases"],
        "use": { "kind": "integration", "instance_ref": { "name": "kubernetes-dakasa-nonprod" }, "operation": "declarative_apply" },
        "with": {
          "objects": [],
          "namespace": "{{ inputs.buildproject_name }}",
          "prune": true
        }
      },
      {
        "id": "remove-dns-record",
        "use": { "kind": "integration", "instance_ref": { "name": "aws-dakasa-validation" }, "operation": "delete_route53_record" },
        "with": {
          "record_name": "{{ inputs.buildproject_name }}.preview.dakasa.com"
        }
      }
    ]
  }
}
```

### 2.9 `transitionToDeleted` — depois do teardown completar

```go
func transitionToDeleted(ctx context.Context, db *sql.DB, bp BuildProject) error {
    // Check if teardown workflow run completed successfully
    if bp.TeardownRunID != "" {
        runStatus, err := queryWorkflowRunStatus(ctx, db, bp.TeardownRunID)
        if err != nil {
            return err
        }
        switch runStatus {
        case "succeeded":
            // Proceed to deletion
        case "running", "pending":
            return nil // Wait for completion
        case "failed":
            // Log the failure, emit event, stay in 'expiring' state
            events.Emit(nil, EmitEventRequest{
                Type: "buildproject.teardown_failed",
                Payload: map[string]any{
                    "buildproject_id": bp.ID,
                    "teardown_run_id": bp.TeardownRunID,
                },
            })
            return nil
        }
    }

    // Transition to 'deleted'
    tx, err := db.BeginTx(ctx, nil)
    defer tx.Rollback()

    _, err = tx.ExecContext(ctx, `
        UPDATE topology_build_projects
        SET lifecycle_status = 'deleted',
            deleted_at = NOW()
        WHERE id = $1 AND lifecycle_status = 'expiring'
    `, bp.ID)
    if err != nil {
        return err
    }

    events.Emit(tx, EmitEventRequest{
        Type:          "buildproject.deleted",
        SchemaVersion: "v1",
        AggregateType: "buildproject",
        AggregateID:   bp.ID.String(),
        Payload: map[string]any{
            "buildproject_id": bp.ID,
            "name":            bp.BuildName,
        },
    })

    return tx.Commit()
}
```

### 2.10 Hard-delete retention (opcional)

Projects em `deleted` permanecem na DB por um período configurável (default: 30 dias), depois são hard-deleted para não acumular storage infinitamente.

```go
func hardDeleteOldDeleted(ctx context.Context, db *sql.DB, cutoff time.Time) error {
    _, err := db.ExecContext(ctx, `
        DELETE FROM topology_build_projects
        WHERE lifecycle_status = 'deleted'
          AND deleted_at < $1
    `, cutoff)
    return err
}
```

**Não emite event** — o event stream é o histórico, row gone não significa history gone.

## 3. API de gerenciamento de BuildProject

Além do loop automático, usuários podem interagir explicitamente via RPCs.

### 3.1 Novo RPC: `topology.build_project.extend`

Estende o `ExpiresAt` de um BuildProject ativo (útil para "auto-extend on activity" ou "dá mais 24h neste dev env").

```json
// Request
{
  "build_project_id": "uuid",
  "new_expires_at": "2026-04-14T00:00:00Z"
}

// Response
{
  "build_project": { ... updated ... }
}
```

Valida: BuildProject está em `active`. Se em `expiring` ou `deleted`, retorna erro `cannot_extend_expired`.

### 3.2 Novo RPC: `topology.build_project.expire_now`

Força expiração imediata de um BuildProject (útil para "limpa isso agora, não espera expiry"):

```json
// Request
{
  "build_project_id": "uuid"
}

// Response
{
  "transitioned": true
}
```

Executa o mesmo `transitionToExpiring` imediatamente, sem esperar ExpiresAt.

### 3.3 Novo RPC: `topology.build_project.restore`

Reverte um BuildProject de `deleted` para `active` (se ainda não hard-deleted). Útil em erros humanos ou emergências.

```json
// Request
{
  "build_project_id": "uuid",
  "new_expires_at": "2026-04-14T00:00:00Z"
}

// Response
{
  "build_project": { ... restored ... }
}
```

Valida: BuildProject em `deleted`, ainda não hard-deleted. Se foi hard-deleted, retorna `cannot_restore_hard_deleted`.

### 3.4 Listagem de BuildProjects por estado

`topology.build_project.list` já existe (no audit listado). Este spec adiciona filtros:

```json
{
  "filters": {
    "lifecycle_status": "active | expiring | deleted",
    "ephemeral_only": true,
    "expires_at_before": "2026-04-11T00:00:00Z"
  },
  "pagination": { "limit": 100 }
}
```

## 4. Eventos emitidos no event stream

Com a feature habilitada, os seguintes events são emitidos (em `docs/contracts/events/v1/buildproject/`):

| Event type | Quando | Payload |
|---|---|---|
| `buildproject.created` | Novo BuildProject é criado via `topology.build_project.create` | `{id, name, ephemeral, expires_at, env_type, cloud, ...}` |
| `buildproject.extended` | `topology.build_project.extend` chamado | `{id, old_expires_at, new_expires_at}` |
| `buildproject.expired` | Lifecycle loop transicionou de `active` → `expiring` | `{id, name, expires_at, teardown_workflow_ref}` |
| `buildproject.teardown_started` | Teardown workflow dispatched (se configurado) | `{id, teardown_run_id}` |
| `buildproject.teardown_completed` | Teardown workflow terminou com sucesso | `{id, teardown_run_id}` |
| `buildproject.teardown_failed` | Teardown workflow falhou | `{id, teardown_run_id, error}` |
| `buildproject.deleted` | Transição para `deleted` | `{id, name}` |
| `buildproject.hard_deleted` | Row removida da DB (opcional — pode ser skipped pra não emitir event para cada hard delete) | `{id}` |

Consumers podem construir projections:
- **Activity feed**: "PR env dakasa-pr-42 foi criado 2 dias atrás, expirou hoje, teardown em andamento"
- **Cost tracking**: correlacionar `buildproject.created` com `integration.execute` (provisioning) e `buildproject.deleted` (cleanup) para billing por env
- **Slack notifications**: alertar time quando `buildproject.teardown_failed`

## 5. Configuration

### 5.1 Environment variables

```
BUILDPROJECT_LIFECYCLE_INTERVAL_SECONDS=60
    # Default: 60. Quantos segundos entre passes do loop.

BUILDPROJECT_HARD_DELETE_RETENTION_DAYS=30
    # Default: 30. Quantos dias um BuildProject fica em 'deleted' antes de hard-delete.
    # 0 desabilita hard-delete (rows ficam pra sempre).

BUILDPROJECT_TEARDOWN_WORKFLOW_TIMEOUT_SECONDS=3600
    # Default: 3600 (1h). Quanto tempo o loop espera por um teardown workflow antes de desistir.
```

### 5.2 Overrides per-BuildProject

O teardown workflow é per-BuildProject (campo `TeardownWorkflowRef`). Outros parâmetros são globais via env vars — simples o suficiente para MVP. Podem virar per-BuildProject no futuro se necessário.

## 6. Concurrency e multi-worker scenarios

### 6.1 Múltiplos yggdrasil-core workers

Em deploy com N workers (supported por HA natural do core), todos os N rodam o lifecycle loop independentemente. Potencial de race condition: dois workers tentam transicionar o mesmo BuildProject ao mesmo tempo.

**Solução: optimistic locking via SQL UPDATE com WHERE conditions.** O UPDATE em `transitionToExpiring` tem WHERE `lifecycle_status = 'active' AND expires_at < NOW()`. Apenas um worker acerta; outros veem `affected = 0` e skip.

### 6.2 Worker crash durante teardown dispatch

Cenário: worker A transicionou BP para `expiring`, dispatchou teardown workflow, mas crashou antes de commitar o `teardown_run_id`. Resultado:
- BP está em `expiring` com `teardown_run_id = NULL`
- Workflow está rodando mas ninguém rastreia

**Mitigação:**
- A transaction que faz `UPDATE lifecycle_status + INSERT event + dispatch workflow` é atomic. Se crash mid-tx, rollback → BP volta para `active`.
- Se o dispatch é async (via RabbitMQ publish), o pub acontece na tx — se tx rollback, message não é enviada.
- Na próxima iteração do loop, BP volta a ser candidato → retenta.

### 6.3 Teardown workflow run failures

Se o teardown workflow crashea, event `buildproject.teardown_failed` é emitido, BP fica em `expiring` indefinidamente. Operator precisa intervir (fix o workflow, re-dispatch, ou `topology.build_project.expire_now` para forçar).

**Alternativa MVP**: um retry automático com backoff após teardown_failed (ex: retry após 1h, até 3x). **Não** é MVP; MVP só reporta failure via event.

## 7. Implementação

### 7.1 Arquivos afetados

- `db/migrations/00NNN_buildproject_lifecycle.sql` — adiciona campos ao topology_build_projects + índice
- `model/topology.go` — BuildProject ganha novos campos
- `repository/topology.go` — queries para `FindExpiredCandidates`, `FindExpiringWithCompletedTeardown`, `HardDeleteOldDeleted`
- `addons/buildproject_lifecycle.go` (novo) — loop addon
- `controllers/message/topology.go` — RPCs `extend`, `expire_now`, `restore`
- `docs/contracts/events/v1/buildproject/*.schema.json` — 7+ event schemas
- `docs/contracts/rpc/topology/v1/` — schemas dos novos RPCs

### 7.2 Fases de implementação

#### Fase 1 — Schema + model

- [ ] Migration com novos campos + índice
- [ ] Model BuildProject atualizado
- [ ] Repository queries (find candidates, transitions)
- [ ] Unit tests

#### Fase 2 — Lifecycle loop (sem teardown workflow)

- [ ] `addons/buildproject_lifecycle.go` com loop + `runLifecyclePass`
- [ ] `transitionToExpiring` sem dispatch de workflow (só marca estado + emite event)
- [ ] `transitionToDeleted` (sem esperar teardown)
- [ ] `hardDeleteOldDeleted`
- [ ] Integration tests cobrindo: create BP ephemeral → wait → loop detects → expires → deletes

#### Fase 3 — Teardown workflow dispatch

- [ ] Campo `TeardownWorkflowRef` no BuildProject
- [ ] `dispatchTeardownWorkflow` na transição para `expiring`
- [ ] `transitionToDeleted` aguarda teardown workflow completar
- [ ] `buildproject.teardown_failed` event emission
- [ ] Integration tests cobrindo: BP com teardown workflow → expira → workflow roda → marca deleted

#### Fase 4 — Management RPCs

- [ ] `topology.build_project.extend` RPC
- [ ] `topology.build_project.expire_now` RPC
- [ ] `topology.build_project.restore` RPC
- [ ] Unit + integration tests

#### Fase 5 — Documentation + examples

- [ ] Atualizar docs do topology em README do core
- [ ] Exemplo de teardown workflow em `docs/bootstrap/manifests/workflows/`
- [ ] Documentar event schemas em `docs/contracts/events/v1/buildproject/`

## 8. Casos de uso concretos

### 8.1 Ephemeral PR env auto-cleanup

```json
{
  "kind": "build_project",
  "metadata": { "name": "dakasa-pr-42" },
  "spec": {
    "ephemeral": true,
    "expires_at": "2026-04-12T00:00:00Z",
    "teardown_workflow_ref": {
      "name": "teardown-pr-environment",
      "namespace": "dakasa"
    },
    "env_type": "ephemeral",
    "cloud": "aws"
  }
}
```

PR é merged → alguém (ou outro workflow) chama `topology.build_project.expire_now(dakasa-pr-42)` → loop pega → dispatcha teardown → resources limpos → marca deleted.

OU simplesmente espera `ExpiresAt` passar → mesma coisa automaticamente.

### 8.2 Auto-extend on activity

Um workflow "activity ping" roda a cada commit em PR:

```json
{
  "steps": [
    {
      "id": "extend-pr-env",
      "use": { "kind": "integration", "instance_ref": { "name": "yggdrasil-core" }, "operation": "topology.build_project.extend" },
      "with": {
        "build_project_id": "{{ inputs.build_project_id }}",
        "new_expires_at": "{{ now_plus_48h }}"
      }
    }
  ]
}
```

Enquanto há atividade, o PR env é estendido. Quando atividade para por 48h+, expira naturalmente.

### 8.3 Dev cloud env — expire_now manual

Dev terminou de usar seu dev cloud env, quer limpar imediatamente:

```
ygg buildproject expire --id dakasa-dev-alice
```

Loop pega na próxima iteração (dentro de 60s), dispatcha teardown, limpa.

## 9. Alternativas consideradas

### 9.1 Delegar lifecycle a um cron externo

Um container cron externo (ou Kubernetes CronJob) chama RPCs do core periodicamente para fazer cleanup.

**Rejeitado porque:**
- State management de BuildProjects é core concern; delegar externo fragmenta
- Cron externo precisaria fazer polling + complex logic (transition states, wait for workflows)
- Mais moving parts, mais modos de falha
- Viola "core gerencia estado com maestria"

### 9.2 Temporal-based lifecycle

Usar Temporal para modelar o lifecycle como workflow de longo prazo.

**Rejeitado porque:**
- Adiciona dependência pesada (Temporal) ao core
- Core deveria ser operável com só PostgreSQL + (opcional) RabbitMQ
- Background loop + optimistic locking resolve o problema com 100 linhas de Go

### 9.3 Hard-delete imediato (sem soft-delete)

Quando expira, remove row imediatamente.

**Rejeitado porque:**
- Perde audit trail (quem expirou, quando, teardown deu certo?)
- Irreversível (sem undo em caso de erro)
- Conflita com filosofia "core mantém histórico de mutações"

### 9.4 Lifecycle como eventos reativos (sem loop)

Em vez de loop periódico, usar `LISTEN/NOTIFY` do PostgreSQL para reagir a `INSERT` de novos BuildProjects.

**Rejeitado porque:**
- Complicado para expiration baseada em tempo (precisa de timer per-BP)
- LISTEN/NOTIFY não funciona bem em multi-worker (cada worker recebe, dedup complicado)
- Loop periódico é simples, auditável, e performance é OK com índice correto

## 10. Riscos e mitigações

| Risco | Impacto | Mitigação |
|---|---|---|
| Loop não roda (bug, worker morto) e BPs não expiram | Alto | Monitoring via Heimdall (health check do loop emite metric); alerta se último pass > 10 min atrás |
| Teardown workflow falha silenciosamente | Alto | Event `buildproject.teardown_failed` é emitido; BP fica em `expiring`, visível em listagens com filtro |
| Race condition entre dois workers na transição | Baixo | Optimistic locking via SQL WHERE (§2.7); apenas um ganha |
| Teardown workflow demora muito (horas/dias) | Médio | Timeout configurável; após timeout, event `teardown_timeout` emitido, BP fica em `expiring` até operator agir |
| Accidental expiration de BP de produção (bug de dev seta `ephemeral=true` num BP importante) | Alto | `ephemeral` é flag explicit; review de código + RBAC no `build_project.create` restringe quem pode setar `ephemeral=true` |
| `restore` de BP cujo teardown já rodou (infra já deletada) | Alto | `restore` não re-cria infra; apenas reverte o state no Yggdrasil. Operator precisa manualmente re-criar infra. Documentar claramente. Alternativa: bloquear `restore` após teardown_completed |

## 11. Compatibilidade

### 11.1 BuildProjects existentes sem `lifecycle_status`

Migration seta `lifecycle_status = 'active'` como default para todas as rows existentes. Nenhum BP legacy fica em `expiring` ou `deleted` acidentalmente.

### 11.2 BuildProjects existentes sem `ephemeral=true`

Loop só atua em BPs com `ephemeral = true`. BPs legacy com `ephemeral = false` (ou null) não são afetados.

### 11.3 BuildProjects com `expires_at` mas sem `ephemeral`

Edge case: BP tem `expires_at` setado mas `ephemeral = false`. **Loop ignora** — só atua quando ambos `ephemeral = true AND expires_at < now`. Operator que configurou `expires_at` sem `ephemeral` tem intenção diferente (talvez apenas metadata).

## 12. Critérios de aceitação

- ✅ Novos campos adicionados ao schema + model
- ✅ Background loop funcional com optimistic locking
- ✅ Transições `active → expiring → deleted → hard-deleted` implementadas
- ✅ Teardown workflow dispatch funcional (opcional por BP)
- ✅ Events emitidos no event stream: `created`, `expired`, `deleted`, `teardown_started/completed/failed`
- ✅ RPCs `extend`, `expire_now`, `restore` funcionais
- ✅ Integration tests cobrindo: criação → expiração → teardown → deletion
- ✅ Schema validation rejeita BPs com `teardown_workflow_ref` apontando para workflow inexistente (no create time)
- ✅ Configuration via env vars documentada
- ✅ JSON Schemas de events publicados

## 13. Dependências

- **Depende de:** [Event Stream](./2026-04-10-event-stream-design.md) — para emitir events de lifecycle. Pode ser implementado simultaneamente, mas lifecycle enforcement só "fica completo" quando events funcionam.
- **Habilita:** ephemeral environments reais (PR envs, dev envs, DR drills com TTL), cleanup automático, cost saving por non-leaked resources.

## 14. Pontos em aberto

- **Retries automáticos do teardown workflow**: MVP reporta failure e para. Operator intervém. Futuro: retry automático com backoff exponencial.
- **Teardown timeout granularity**: hoje global (env var). Futuro: per-BP override.
- **Manual teardown override**: capacidade de "eu sei que o teardown workflow está quebrado, marca como deleted mesmo assim". Via RPC `force_delete` ou flag no `expire_now`. Não é MVP.
- **Parent-child BuildProject relationships**: se um ephemeral BP tem child BPs, como cascade? Hoje cada BP é independente. Futuro: cascade delete opcional.
- **Grace period antes de teardown**: "expirou, mas espera 1h antes de rodar teardown, caso alguém queira estender na última hora". Não é MVP; pode ser implementado via workflow de teardown que tem `sleep` primeiro.

## 15. Resumo executivo

- **Feature:** core enforça lifecycle automático de BuildProjects ephemeral
- **Mecanismo:** background loop + optimistic locking + event emission
- **Estados:** active → expiring → deleted → hard-deleted
- **Teardown workflow:** opcional, per BP, dispatched automaticamente quando expira
- **Depende de:** event stream (emitir events de lifecycle)
- **Habilita:** ephemerais reais (não vazam), cleanup automático, audit trail completo
- **Filosofia alinhada:** core gerencia estado com maestria, cumprindo as promessas do schema
