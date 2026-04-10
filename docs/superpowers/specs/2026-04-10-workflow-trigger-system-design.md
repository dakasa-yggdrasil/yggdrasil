# Workflow Trigger System (Schedule + Event) — yggdrasil-core

**Data:** 2026-04-10
**Status:** Spec em revisão
**Tipo:** Core state lifecycle + execution primitive
**Prioridade:** 🔴 Alta — completa o workflow engine que hoje só suporta `manual` trigger
**Escopo:** Implementar os trigger modes `schedule` e `event` que já estão reservados no schema do workflow, mas sem implementação. Core passa a gerenciar quando workflows rodam (não apenas como rodam).
**Depende de:** [Event Stream](./2026-04-10-event-stream-design.md) (para eventos-fonte de triggers do tipo `event`)
**Parte do audit report:** [`2026-04-10-yggdrasil-product-audit-report.md`](./2026-04-10-yggdrasil-product-audit-report.md) Gap 5

---

## 1. Contexto e motivação

### 1.1 O problema

O workflow manifest do Yggdrasil declara suporte a 3 trigger modes:

```go
// manifest/workflow.go:14
supportedWorkflowTriggerModes = []string{"manual", "event", "schedule"}
```

Mas **apenas `manual`** funciona hoje. Workflows com `trigger.mode: "schedule"` ou `trigger.mode: "event"` são aceitos pelo validator (schema permite), mas nada acontece — não há scheduler, não há event subscriber. O que deveria disparar esses workflows simplesmente não existe no core.

Isso é **feature declarada mas não implementada** — pior do que feature não declarada, porque cria expectativa falsa.

### 1.2 Por que é core

Inicialmente considerei que schedule/event triggers poderiam ser **integrations externas** (ex: `integration-scheduler` com cron, `integration-webhook-receiver` com HTTP listener). Após re-calibration filosófica com o user, ficou claro que **não**:

> "Core gerencia estado com maestria" inclui **gerenciar quando coisas devem acontecer no ecossistema**. Se core sabe que "o workflow X deve rodar toda hora" ou "o workflow Y deve rodar quando um certo event acontece", core deve orquestrar isso diretamente. Delegar para integrations fragmenta o lifecycle management.

Integrations **podem produzir eventos externos** (webhook receivers, Kafka consumers) — esses eventos chegam ao core via event stream (Gap 1). Integrations **podem ser schedulers alternativos** — mas o scheduler primário, que avalia o trigger declarativo do workflow, é core.

### 1.3 Filosofia alinhada

- **Core gerencia lifecycle**: triggers são parte do lifecycle (quando algo acontece)
- **Event stream é infraestrutura fundacional**: event triggers se constroem sobre ele (subscribe com filtros)
- **Minimalismo**: adicionar scheduler nativo ao core é uma **única** responsabilidade nova (gerar dispatches no tempo certo) — não múltiplas
- **Contract-first**: triggers são declarados no workflow manifest via schema bem definido

## 2. Design geral

### 2.1 Duas features relacionadas mas separáveis

1. **Schedule triggers**: workflow declara `trigger.mode: "schedule"` + `trigger.schedule: "<cron expr>"`. Core avalia cron, dispara em cada matching tick.
2. **Event triggers**: workflow declara `trigger.mode: "event"` + `trigger.event: { types: [...], filters: {...} }`. Core subscribe no event stream, dispara workflow quando matching events chegam.

Ambos compartilham o mesmo **primitive subjacente**: "dispatch a workflow em certas condições detectadas pelo core". Mas as condições são muito diferentes (tempo vs evento), então os implementadores são dois loops/subsystems distintos.

### 2.2 Princípios

1. **Declarative in the workflow manifest**: o trigger é parte do manifest, versionado. Não há "trigger manifest" separado.
2. **At-least-once dispatch**: se o core crashar no meio de um tick, o trigger pode disparar 2x. Workflows devem ser idempotentes ou lidar com duplicação via correlation_id. (Trade-off: exactly-once é muito mais caro.)
3. **Compostos independentes**: schedule e event triggers podem ser implementados em qualquer ordem. Priorizar schedule (simpler) primeiro.
4. **Escalable com workers**: múltiplos yggdrasil-core workers não podem disparar o mesmo trigger simultaneamente (evitar duplicação). Usar optimistic locking via DB.
5. **Visibilidade total**: cada dispatch emite event no event stream (`workflow.run.dispatched`), com metadata indicando o trigger que causou.

## 3. Schedule triggers

### 3.1 Schema do trigger

No workflow manifest, o trigger de schedule é declarado assim:

```json
{
  "kind": "workflow",
  "metadata": {
    "name": "cleanup-expired-ephemerals",
    "namespace": "dakasa"
  },
  "spec": {
    "trigger": {
      "mode": "schedule",
      "schedule": {
        "cron_expression": "0 * * * *",
        "timezone": "UTC",
        "start_at": "2026-04-10T00:00:00Z",
        "end_at": "2027-04-10T00:00:00Z",
        "default_inputs": {
          "dry_run": false
        }
      }
    },
    "input_schema": {
      "properties": {
        "dry_run": { "type": "boolean" }
      }
    },
    "steps": [ ... ]
  }
}
```

**Campos:**

- `trigger.mode: "schedule"` — identifica o tipo
- `trigger.schedule.cron_expression` — padrão POSIX cron (`minute hour day_of_month month day_of_week`). Exemplos:
  - `"0 * * * *"` — toda hora cheia
  - `"*/5 * * * *"` — a cada 5 minutos
  - `"0 2 * * *"` — todo dia às 02:00
  - `"0 0 * * 0"` — toda domingo à meia-noite
- `trigger.schedule.timezone` — timezone do cron expression (default `UTC`). Aceita qualquer timezone IANA (ex: `America/Sao_Paulo`).
- `trigger.schedule.start_at` (opcional) — não dispara antes desse timestamp
- `trigger.schedule.end_at` (opcional) — não dispara depois desse timestamp
- `trigger.schedule.default_inputs` (opcional) — inputs injected em cada dispatch. Workflow engine merge com input_schema.

### 3.2 Cron expression validation

No parser do workflow manifest, `cron_expression` é validado contra um cron parser (ex: `github.com/robfig/cron/v3`). Expressions inválidas causam manifest validation error.

### 3.3 Scheduler loop

Novo addon: `addons/workflow_scheduler.go`.

```go
func init() {
    Register("workflow_scheduler", bootstrapWorkflowScheduler, 40)
}

func bootstrapWorkflowScheduler(ctx context.Context, app *runtime.ServiceApp) error {
    db, ok := Postgres(app)
    if !ok {
        return fmt.Errorf("postgres addon is not available")
    }

    interval := schedulerLoopInterval()  // default: 10s
    stopLoop := startSchedulerLoop(ctx, db, interval)
    app.RegisterCloser(func(context.Context) error {
        stopLoop()
        return nil
    })

    return nil
}

func startSchedulerLoop(ctx context.Context, db *sql.DB, interval time.Duration) func() {
    ticker := time.NewTicker(interval)
    stop := make(chan struct{})

    go func() {
        for {
            select {
            case <-stop:
                return
            case <-ticker.C:
                if err := runSchedulerPass(ctx, db); err != nil {
                    logger.Warn("scheduler pass failed", zap.Error(err))
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

### 3.4 `runSchedulerPass` — lógica do tick

```go
func runSchedulerPass(ctx context.Context, db *sql.DB) error {
    now := time.Now().UTC()

    // 1. Load all workflows with trigger.mode == "schedule"
    workflows, err := loadScheduledWorkflows(ctx, db)
    if err != nil {
        return err
    }

    // 2. For each workflow, check if it should fire "now"
    for _, wf := range workflows {
        schedule := wf.Spec.Trigger.Schedule

        // Respect start_at / end_at
        if schedule.StartAt != nil && now.Before(*schedule.StartAt) {
            continue
        }
        if schedule.EndAt != nil && now.After(*schedule.EndAt) {
            continue
        }

        // Parse cron expression (use cached parser if possible)
        cronSched, err := parseCron(schedule.CronExpression, schedule.Timezone)
        if err != nil {
            logger.Warn("invalid cron expression", zap.String("workflow", wf.Metadata.Name), zap.Error(err))
            continue
        }

        // Find the last known fire time from scheduler state
        lastFiredAt, err := getLastFiredAt(ctx, db, wf.ID)
        if err != nil {
            return err
        }
        if lastFiredAt == nil {
            lastFiredAt = &schedule.StartAt  // or wf.CreatedAt if start_at is nil
        }

        // Compute the next fire time after lastFiredAt
        nextFireAt := cronSched.Next(*lastFiredAt)

        // If the next fire time has passed, dispatch now
        if nextFireAt.Before(now) || nextFireAt.Equal(now) {
            if err := dispatchScheduledWorkflow(ctx, db, wf, nextFireAt); err != nil {
                logger.Warn("dispatch failed", zap.String("workflow", wf.Metadata.Name), zap.Error(err))
            }
        }
    }

    return nil
}
```

### 3.5 Tabela `workflow_schedule_state`

Para persistir `lastFiredAt` por workflow e garantir idempotência entre workers:

```sql
CREATE TABLE workflow_schedule_state (
    workflow_manifest_id  UUID PRIMARY KEY,
    last_fired_at         TIMESTAMPTZ NOT NULL,
    next_fire_at          TIMESTAMPTZ NOT NULL,
    consecutive_failures  INTEGER NOT NULL DEFAULT 0,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    FOREIGN KEY (workflow_manifest_id) REFERENCES manifests(id) ON DELETE CASCADE
);

CREATE INDEX workflow_schedule_state_next_fire_idx
    ON workflow_schedule_state (next_fire_at);
```

`next_fire_at` é pré-computado e indexed, permitindo que o loop pegue "scheduled workflows due now" com um único query eficiente:

```sql
SELECT wf.*, state.last_fired_at, state.next_fire_at
FROM manifests wf
LEFT JOIN workflow_schedule_state state ON state.workflow_manifest_id = wf.id
WHERE wf.kind = 'workflow'
  AND wf.active = true
  AND wf.spec->'trigger'->>'mode' = 'schedule'
  AND (state.next_fire_at IS NULL OR state.next_fire_at <= NOW())
ORDER BY state.next_fire_at ASC NULLS FIRST
LIMIT 100;
```

### 3.6 `dispatchScheduledWorkflow` — atomic dispatch

```go
func dispatchScheduledWorkflow(ctx context.Context, db *sql.DB, wf Workflow, nextFireAt time.Time) error {
    tx, err := db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // 1. Atomically update workflow_schedule_state: set last_fired_at = nextFireAt
    //    Use WHERE (last_fired_at IS NULL OR last_fired_at < nextFireAt) for idempotency
    result, err := tx.ExecContext(ctx, `
        INSERT INTO workflow_schedule_state (workflow_manifest_id, last_fired_at, next_fire_at)
        VALUES ($1, $2, $3)
        ON CONFLICT (workflow_manifest_id) DO UPDATE
        SET last_fired_at = $2,
            next_fire_at = $3,
            updated_at = NOW()
        WHERE workflow_schedule_state.last_fired_at < $2
    `, wf.ID, nextFireAt, computeNextFireAfter(wf, nextFireAt))
    if err != nil {
        return err
    }
    affected, _ := result.RowsAffected()
    if affected == 0 {
        // Another worker already fired this tick; skip
        return nil
    }

    // 2. Dispatch the workflow (insert into dispatch queue or call dispatcher directly)
    runID := uuid.New()
    err = workflowDispatcher.Dispatch(ctx, tx, DispatchRequest{
        RunID:       runID,
        WorkflowRef: ManifestSelector{ID: wf.ID},
        Inputs:      wf.Spec.Trigger.Schedule.DefaultInputs,
        Metadata: map[string]any{
            "triggered_by":       "schedule",
            "scheduled_time":     nextFireAt.Format(time.RFC3339),
            "cron_expression":    wf.Spec.Trigger.Schedule.CronExpression,
        },
    })
    if err != nil {
        return err
    }

    // 3. Emit event (same tx)
    events.Emit(tx, EmitEventRequest{
        Type:          "workflow.run.dispatched",
        SchemaVersion: "v1",
        AggregateType: "workflow_run",
        AggregateID:   runID.String(),
        Actor:         &EventActor{Type: "system", ID: "workflow-scheduler"},
        Payload: map[string]any{
            "workflow_ref":    ManifestSelector{ID: wf.ID, Name: wf.Metadata.Name, Namespace: wf.Metadata.Namespace},
            "run_id":          runID,
            "triggered_by":    "schedule",
            "scheduled_time":  nextFireAt,
        },
    })

    return tx.Commit()
}
```

**Atomicidade:** a UPDATE condicional (`WHERE last_fired_at < nextFireAt`) garante que dois workers tentando firar o mesmo tick veem apenas um sucesso. O perdedor vê `affected = 0` e skipa.

### 3.7 Missed ticks (catch-up vs skip)

Cenário: o scheduler loop crashou por 2h, volta online. Um workflow com cron `*/5 * * * *` deveria ter firado 24 vezes no intervalo. Como lidar?

**Opções:**

- **Catch-up**: firar 24 vezes imediatamente. Protege contra perda de execuções. **Problema**: se o workflow tem side effects, pode causar avalanche.
- **Skip**: firar só uma vez (a próxima programada após o downtime). **Problema**: perde execuções.
- **Configurável**: cada workflow declara `trigger.schedule.catchup_policy: "catch_up" | "skip"`.

**MVP decision**: **skip** default, **configurável** via campo opcional. Evita avalanche; workflows que precisam catch-up opt-in explicitamente.

```json
"schedule": {
  "cron_expression": "*/5 * * * *",
  "catchup_policy": "skip"
}
```

### 3.8 Desabilitar schedule temporariamente

Um workflow pode ter seu schedule "pausado" via flag:

```json
"trigger": {
  "mode": "schedule",
  "enabled": false,
  "schedule": { ... }
}
```

Quando `enabled: false`, o scheduler loop ignora o workflow. Útil para debug ou manutenção.

## 4. Event triggers

### 4.1 Schema do trigger

```json
{
  "kind": "workflow",
  "metadata": { "name": "deploy-on-manifest-change" },
  "spec": {
    "trigger": {
      "mode": "event",
      "event": {
        "types": ["manifest.product.updated"],
        "aggregate_filter": {
          "aggregate_type": "manifest",
          "name_pattern": "dakasa-*"
        },
        "payload_filters": [
          {
            "path": "namespace",
            "operator": "eq",
            "value": "dakasa"
          }
        ],
        "default_inputs": {
          "auto_deploy": true
        },
        "debounce_seconds": 30
      }
    },
    "input_schema": { ... },
    "steps": [ ... ]
  }
}
```

**Campos:**

- `trigger.mode: "event"` — identifica o tipo
- `trigger.event.types` — array de event type patterns para subscribe (ex: `"manifest.*"`, `"buildproject.created"`). Supports wildcards (same semantics as event_stream pull filters).
- `trigger.event.aggregate_filter` (opcional) — filtros adicionais por aggregate_type / aggregate_id / name pattern
- `trigger.event.payload_filters` (opcional) — filtros sobre o payload do evento (key path + operator + value). Operators: `eq`, `neq`, `in`, `not_in`, `contains`, `matches` (regex).
- `trigger.event.default_inputs` (opcional) — merged com inputs do workflow (event payload pode virar input via templating)
- `trigger.event.debounce_seconds` (opcional) — se o mesmo tipo de event dispara N vezes em < N segundos, core consolida em apenas 1 dispatch (útil para evitar avalanche em eventos rápidos)

### 4.2 Event subscription engine

Novo addon: `addons/workflow_event_triggers.go`.

Este addon roda um **subscriber interno do event stream**. Ele pull events continuamente, faz match contra os event triggers registered, e dispatcha workflows que casam.

```go
func runEventTriggerLoop(ctx context.Context, db *sql.DB) {
    cursor := loadLastProcessedCursor(ctx, db)

    for {
        // Pull next batch of events from event stream
        response, err := eventStream.Pull(ctx, PullRequest{
            Cursor: cursor,
            Limit:  100,
        })
        if err != nil {
            logger.Warn("event stream pull failed", zap.Error(err))
            time.Sleep(5 * time.Second)
            continue
        }

        for _, event := range response.Events {
            // Find all workflows whose event trigger matches this event
            matchingWorkflows := findMatchingEventTriggers(ctx, db, event)

            for _, wf := range matchingWorkflows {
                if shouldDebounce(wf, event) {
                    continue
                }

                err := dispatchEventTriggeredWorkflow(ctx, db, wf, event)
                if err != nil {
                    logger.Warn("dispatch failed", zap.Error(err))
                }
            }
        }

        // Advance cursor after processing batch
        cursor = response.NextCursor
        saveLastProcessedCursor(ctx, db, cursor)

        if !response.HasMore {
            time.Sleep(1 * time.Second)  // briefly wait before next poll
        }
    }
}
```

### 4.3 Matching logic

Um event matches um event trigger se **todas** as condições são satisfeitas:

1. `event.type` casa com algum pattern em `trigger.event.types` (wildcard match)
2. (Opcional) `event.aggregate_type` casa com `trigger.event.aggregate_filter.aggregate_type`
3. (Opcional) `event.aggregate_id` or `payload.name` casa com `trigger.event.aggregate_filter.name_pattern` (glob)
4. (Opcional) cada `payload_filter` é satisfeito contra `event.payload` via dotted path lookup

```go
func eventMatchesTrigger(event Event, trigger EventTrigger) bool {
    // 1. Type match
    typeMatches := false
    for _, pattern := range trigger.Types {
        if wildcardMatch(event.Type, pattern) {
            typeMatches = true
            break
        }
    }
    if !typeMatches {
        return false
    }

    // 2. Aggregate filter
    if trigger.AggregateFilter != nil {
        if trigger.AggregateFilter.AggregateType != "" && event.AggregateType != trigger.AggregateFilter.AggregateType {
            return false
        }
        if trigger.AggregateFilter.NamePattern != "" {
            name := extractName(event)  // from payload or aggregate_id
            if !globMatch(name, trigger.AggregateFilter.NamePattern) {
                return false
            }
        }
    }

    // 3. Payload filters
    for _, filter := range trigger.PayloadFilters {
        value := dottedPathLookup(event.Payload, filter.Path)
        if !applyOperator(value, filter.Operator, filter.Value) {
            return false
        }
    }

    return true
}
```

### 4.4 Cursor persistence para event trigger loop

Como o loop de event triggers é um subscriber do event stream, ele precisa persistir seu cursor para não re-processar events em caso de restart.

```sql
CREATE TABLE workflow_event_trigger_state (
    id                 VARCHAR(64) PRIMARY KEY,  -- fixo: "workflow_event_trigger_loop"
    last_cursor        TEXT NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Single-row table. Loop atualiza `last_cursor` após processar cada batch.

**Multi-worker scenario**: múltiplos workers rodando event trigger loops ao mesmo tempo. Precisa evitar duplicação de dispatch.

**Solução**: lock row-level via `SELECT ... FOR UPDATE SKIP LOCKED`:

```sql
SELECT id, last_cursor FROM workflow_event_trigger_state
WHERE id = 'workflow_event_trigger_loop'
FOR UPDATE SKIP LOCKED
LIMIT 1;
```

Apenas um worker segura o lock; outros fazem `SKIP LOCKED` e esperam próxima iteração. O worker com lock processa um batch, commit (libera lock), próxima iteração outro pode pegar.

Alternativa mais elegante: **leader election** via PostgreSQL advisory lock. Apenas o líder roda o loop. Leader change é seamless quando o atual cai.

**MVP decision**: advisory lock-based leader election. Simples e eficaz.

```go
// At start of each pass
_, err := db.Exec(`SELECT pg_try_advisory_lock(42)`)  // 42 is arbitrary int id for this lock
// If locked (this worker is leader), proceed. Else skip this pass.
```

### 4.5 Debounce logic

Se o trigger tem `debounce_seconds: 30` e eventos matching chegam rapidamente (ex: 10 eventos em 5 segundos), o loop:

1. Primeiro matching event: agenda dispatch para "now + 30s" (em memória)
2. Second matching event dentro do window: cancela o dispatch pending, re-agenda para "now + 30s"
3. ... (resets continuamente conforme novos events)
4. Após 30s sem novos events matching, dispatcha uma única vez

Isso é útil para "ao invés de disparar 10 deploys seguidos quando um arquivo é editado 10 vezes em 10s, dispare 1 deploy 30s após o último edit".

**Implementação**: map em memória `workflowID -> pendingDispatch`. Cada worker mantém seu próprio. **Caveat**: se o worker crashea com pending dispatches, eles são perdidos. **Aceito** no MVP — debounce é best-effort; exact semantics em multi-worker pode ser melhorado depois.

### 4.6 Event → Input mapping

O workflow receber o event como input? Sim, via templating. O dispatch passa o event payload como `metadata.triggering_event`, acessível em `{{ metadata.triggering_event }}`:

```json
{
  "trigger": {
    "mode": "event",
    "event": {
      "types": ["buildproject.created"],
      "default_inputs": {
        "build_project_id": "{{ metadata.triggering_event.payload.buildproject_id }}"
      }
    }
  }
}
```

O templating engine já suporta isso (ver workflow docs). Metadata do event é injetada no context do workflow run.

## 5. Events emitidos

Quando triggers disparam workflows, core emite events no event stream:

- `workflow.run.dispatched` — workflow foi dispatched (já existe no event stream spec; ganha campos no metadata para `triggered_by: "schedule" | "event" | "manual"`)
- `workflow.trigger.schedule.fired` — (opcional) schedule trigger "fired". Pode ser útil para debugging, mas redundante com `workflow.run.dispatched`. **MVP decision**: não emitir, só `workflow.run.dispatched`.
- `workflow.trigger.event.matched` — (opcional) event trigger "matched" um event. Mesmo raciocínio.

O metadata do `workflow.run.dispatched` carrega:

```json
{
  "run_id": "...",
  "workflow_ref": { ... },
  "triggered_by": "schedule",  // or "event" or "manual"
  "schedule": {
    "cron_expression": "0 * * * *",
    "scheduled_time": "2026-04-10T12:00:00Z"
  },
  "event": {
    "source_event_id": "018f2b4a-...",
    "source_event_type": "manifest.product.updated"
  }
}
```

Projeções (audit, activity feed) podem reconstruir "quem disparou o quê e por quê".

## 6. Implementação

### 6.1 Arquivos afetados

#### Schedule triggers

- `db/migrations/00NNN_workflow_schedule_state.sql` — tabela `workflow_schedule_state`
- `manifest/workflow.go` — validar `trigger.schedule.cron_expression` com cron parser
- `model/workflow.go` — struct `WorkflowScheduleTrigger`
- `addons/workflow_scheduler.go` (novo) — loop + dispatch logic
- `repository/workflow_schedule.go` — queries para candidatos, lastFiredAt, dispatch
- `docs/contracts/workflows/v1/trigger_schedule.schema.json`

#### Event triggers

- `db/migrations/00NNN_workflow_event_trigger_state.sql` — tabela `workflow_event_trigger_state`
- `manifest/workflow.go` — validar `trigger.event`
- `model/workflow.go` — struct `WorkflowEventTrigger`
- `addons/workflow_event_triggers.go` (novo) — subscriber loop + match logic
- `repository/workflow_event_trigger.go` — queries para event triggers + cursor state
- `docs/contracts/workflows/v1/trigger_event.schema.json`

### 6.2 Fases de implementação

#### Fase 1 — Schedule triggers

- [ ] Dependency: `github.com/robfig/cron/v3` (cron parser)
- [ ] Migration `workflow_schedule_state`
- [ ] Manifest validator aceita `trigger.mode: "schedule"` + valida cron expression
- [ ] Scheduler loop em novo addon
- [ ] `dispatchScheduledWorkflow` com atomic update
- [ ] `catchup_policy: "skip"` default, `catch_up` como opt-in
- [ ] `enabled: false` flag para pausar
- [ ] Events emitidos (`workflow.run.dispatched` com metadata.triggered_by=schedule)
- [ ] Integration tests: cron expression básica, skip policy, multi-worker concurrency

#### Fase 2 — Event triggers (depois da Fase 1)

- [ ] Migration `workflow_event_trigger_state`
- [ ] Manifest validator aceita `trigger.mode: "event"` + schema validation
- [ ] Event trigger loop em novo addon, com advisory lock leader election
- [ ] Event stream pull com cursor persistence
- [ ] Matching logic (types, aggregate, payload filters)
- [ ] Dispatch com event metadata injected
- [ ] Debounce logic (in-memory per-worker, best-effort)
- [ ] Events emitidos (`workflow.run.dispatched` com metadata.triggered_by=event)
- [ ] Integration tests: event match, no match, multi-type, payload filter, debounce

#### Fase 3 — Integration com features existentes

- [ ] BuildProject lifecycle enforcement (Gap 4) pode usar event triggers: workflow `cleanup-expired` dispara em `buildproject.expired` events. Exemplo em `docs/bootstrap/manifests/workflows/`
- [ ] Scheduled cleanup workflows como alternativa: workflow `cleanup-expired` dispara com cron `*/5 * * * *` e queries expired
- [ ] Dogfooding: conversão de alguns workflows existentes para usar schedule/event triggers

#### Fase 4 — Docs + examples

- [ ] README do workflow engine explica triggers modes
- [ ] Exemplo schedule em `docs/bootstrap/manifests/workflows/`
- [ ] Exemplo event trigger em `docs/bootstrap/manifests/workflows/`
- [ ] JSON Schemas atualizados em `docs/contracts/workflows/v1/`

## 7. Casos de uso concretos

### 7.1 Scheduled cleanup de ephemerais (alternativa ao loop do Gap 4)

```json
{
  "kind": "workflow",
  "metadata": { "name": "cleanup-expired-ephemerals" },
  "spec": {
    "trigger": {
      "mode": "schedule",
      "schedule": {
        "cron_expression": "*/10 * * * *",
        "catchup_policy": "skip"
      }
    },
    "steps": [
      {
        "id": "list-expired",
        "use": { "kind": "integration", "instance_ref": { "name": "yggdrasil-core" }, "operation": "topology.build_project.list" },
        "with": {
          "filters": { "lifecycle_status": "active", "expires_at_before": "{{ now }}", "ephemeral_only": true }
        }
      },
      {
        "id": "expire-each",
        "depends_on": ["list-expired"],
        "use": { "kind": "integration", "instance_ref": { "name": "yggdrasil-core" }, "operation": "topology.build_project.expire_now_batch" },
        "with": {
          "build_project_ids": "{{ steps.list-expired.output.items | map: 'id' }}"
        }
      }
    ]
  }
}
```

**Observação:** isso é **alternativa** ao loop nativo em `buildproject_lifecycle` addon do Gap 4. As duas abordagens são válidas — Gap 4 tem lifecycle hardcoded, Gap 5 permite flexibilidade via workflows declarativos. Ambas podem coexistir: lifecycle enforcer é default, workflow agendado é para casos customizados.

### 7.2 Event-driven deploy on manifest change

```json
{
  "kind": "workflow",
  "metadata": { "name": "auto-deploy-on-product-update" },
  "spec": {
    "trigger": {
      "mode": "event",
      "event": {
        "types": ["manifest.product.updated"],
        "aggregate_filter": {
          "name_pattern": "dakasa-*"
        },
        "debounce_seconds": 60
      }
    },
    "steps": [
      {
        "id": "apply-updated-product",
        "use": { "kind": "product", "operation": "installation.apply" },
        "with": {
          "product_ref": {
            "namespace": "{{ metadata.triggering_event.payload.namespace }}",
            "name": "{{ metadata.triggering_event.payload.name }}"
          }
        }
      }
    ]
  }
}
```

**Flow:**
1. Dev atualiza `dakasa-app` product manifest via RPC
2. Core emite event `manifest.product.updated` com payload incluindo name/namespace
3. Event trigger loop detecta match (name_pattern = "dakasa-*")
4. Debounces 60s para agrupar múltiplas updates (se houver)
5. Dispatcha `auto-deploy-on-product-update` workflow
6. Workflow aplica o product atualizado no target

Isso é **GitOps-style continuous deployment**, mas orquestrado pelo core Yggdrasil sem precisar de ArgoCD externo.

### 7.3 Event-driven notification on critical incident

```json
{
  "kind": "workflow",
  "metadata": { "name": "notify-slack-on-critical" },
  "spec": {
    "trigger": {
      "mode": "event",
      "event": {
        "types": ["product.installation.apply_failed", "workflow.run.failed"],
        "payload_filters": [
          {
            "path": "severity",
            "operator": "eq",
            "value": "critical"
          }
        ]
      }
    },
    "steps": [
      {
        "id": "post-to-slack",
        "use": { "kind": "integration", "instance_ref": { "name": "slack-dakasa" }, "operation": "send_message" },
        "with": {
          "channel": "#incidents",
          "text": "Critical incident: {{ metadata.triggering_event.type }} — {{ metadata.triggering_event.payload.error }}"
        }
      }
    ]
  }
}
```

### 7.4 Scheduled backup com timezone brasileiro

```json
{
  "kind": "workflow",
  "metadata": { "name": "daily-backup" },
  "spec": {
    "trigger": {
      "mode": "schedule",
      "schedule": {
        "cron_expression": "0 2 * * *",
        "timezone": "America/Sao_Paulo"
      }
    },
    "steps": [
      {
        "id": "snapshot-rds",
        "use": { "kind": "integration", "instance_ref": { "name": "aws-dakasa-prod" }, "operation": "create_rds_snapshot" },
        "with": {
          "db_instance": "dakasa-prod",
          "snapshot_name": "dakasa-backup-{{ now | date: '%Y-%m-%d' }}"
        }
      }
    ]
  }
}
```

Roda **02:00 BRT** diariamente (o scheduler converte de America/Sao_Paulo para UTC).

## 8. Alternativas consideradas

### 8.1 Scheduler via Temporal

Usar Temporal como scheduler interno do core.

**Rejeitado porque:**
- Adiciona dependency pesada (Temporal) ao core
- Core deve ser operável com só PostgreSQL + (opcional) RabbitMQ
- Cron scheduler nativo é trivial em Go (~100 linhas)

### 8.2 Integration-scheduler externa

Criar `integration-scheduler-cron` como integration que dispatcha workflows externamente.

**Rejeitado porque** (re-calibrado com user feedback):
- Delegar lifecycle management para fora do core fragmenta o estado
- Core gerencia "quando coisas acontecem no ecossistema" como parte do state management
- Integration alternativa ainda pode existir **em adição** ao scheduler core, para casos que queiram cron externo (ex: reuso de cronjobs Kubernetes existentes)

### 8.3 Event triggers via RabbitMQ direct (pub/sub ao invés de pull)

Em vez de pull-based subscription do event stream, usar RabbitMQ pub/sub para push events a event triggers registered.

**Rejeitado porque:**
- Event stream é definidamente pull-based (ver [spec do event stream](./2026-04-10-event-stream-design.md))
- Inconsistência arquitetural
- Pull é simples e robust (subscriber maneja backpressure)

### 8.4 Schedule declarado fora do workflow manifest (separate schedule manifest)

Criar um novo manifest kind `schedule` que aponta para workflows.

**Rejeitado porque:**
- Adiciona concept novo quando workflow já tem campo `trigger` reservado
- Separação "workflow" vs "schedule" é artificial — o schedule **é** parte do trigger do workflow

## 9. Riscos e mitigações

| Risco | Impacto | Mitigação |
|---|---|---|
| Cron expression inválida causa crash do scheduler | Médio | Validation no manifest parser; expressions inválidas rejeitadas em submit time |
| Missed ticks devido a downtime do scheduler | Baixo (com `skip` default) | Default `skip` evita avalanche; operators cientes podem opt-in para `catch_up` |
| Race condition entre workers no mesmo tick | Baixo | Optimistic locking via SQL UPDATE condicional |
| Event trigger loop fica preso em um event problemático | Alto | Logs + metric em event processing time; max timeout por batch; events que causam crash de dispatch são skipados com log WARN |
| Debounce em memória perdido no restart | Baixo | Debounce é best-effort; aceito. Workflows devem ser idempotentes |
| Leader election failure (advisory lock não libera) | Médio | Advisory locks liberam automaticamente quando a conexão PG cai; acceptable |
| Event trigger match muito loose causa spam de dispatches | Alto | Autores de triggers são responsáveis por filters estreitos; debounce + max dispatches per minute como safeguard futuro |
| Mudança no schema de evento quebra triggers existentes | Médio | Schema versioning do event stream; triggers declaram `supported_schema_versions` opcional |

## 10. Compatibilidade

### 10.1 Workflows existentes com `trigger.mode: "manual"`

Continuam funcionando exatamente como antes. Schedule/event features são opt-in via mudança de trigger.mode.

### 10.2 Workflows existentes com `trigger.mode: "schedule"` ou `"event"` (schema-only)

Após esta feature ser implementada, esses workflows **passam a funcionar**. Se algum usuário criou workflows com esses modes esperando que funcionassem, eles começam a disparar automaticamente após deploy. **Potencial breaking behavior**.

**Mitigation:** documentar no CHANGELOG. Operators devem auditar workflows com esses modes antes do upgrade.

## 11. Critérios de aceitação

### Schedule triggers

- ✅ Cron expression parseada e validada em manifest parser
- ✅ Timezone support (IANA names)
- ✅ Scheduler loop rodando em background addon
- ✅ Dispatch atomic com optimistic locking
- ✅ `catchup_policy: "skip" | "catch_up"` suportado (default skip)
- ✅ `enabled: false` flag funcional
- ✅ Events `workflow.run.dispatched` emitidos com metadata
- ✅ Integration tests cobrindo: cron tick, missed ticks, multi-worker, enabled/disabled

### Event triggers

- ✅ Trigger schema validado em manifest parser
- ✅ Event trigger loop rodando em background addon (com leader election via advisory lock)
- ✅ Cursor persistido em `workflow_event_trigger_state`
- ✅ Matching logic (types + aggregate + payload filters) funcional
- ✅ Debounce best-effort in-memory
- ✅ Event metadata injected via templating (`metadata.triggering_event.*`)
- ✅ Events `workflow.run.dispatched` emitidos
- ✅ Integration tests cobrindo: type match, payload filter, debounce, cursor resume

## 12. Dependências

- **Depende de:** [Event Stream](./2026-04-10-event-stream-design.md) — event triggers subscribe no stream; schedule triggers emit events via stream
- **Relaciona com:** [BuildProject lifecycle enforcement](./2026-04-10-buildproject-lifecycle-enforcement-design.md) — alternativa declarativa via workflow agendado; não substitui, mas complementa
- **Habilita:** CD pipelines event-driven, cron jobs nativos, auto-deploy on change, notifications em eventos críticos, periodic maintenance tasks

## 13. Pontos em aberto

- **Max dispatches per minute per trigger**: safeguard contra infinite loops (ex: workflow dispara em `workflow.run.completed` e dispara a si mesmo). MVP não implementa; adicionar se for problema real.
- **Workflow → event trigger recursion detection**: mesmo problema. Não MVP.
- **Schedule com múltiplas timezones alternativas**: workflow que deveria rodar "10 AM local para cada região". Hoje só um timezone por workflow. Feature futura via múltiplos triggers per workflow (hoje só suporta um trigger).
- **Event payload transformation antes de input**: além de referenciar `metadata.triggering_event.payload.x`, permitir transformation mais complexa (jq-style). MVP usa templating simples; transformation complexa é futura.
- **Pause de event triggers**: como pausar event trigger (equivalente a `enabled: false` de schedule)? Mesmo pattern: `trigger.event.enabled: false`. Adicionar no MVP.
- **Unique constraints por workflow**: hoje cada workflow pode ter apenas UM trigger (mode é single-choice). Se precisar de dois (ex: cron + on-event), criar dois workflow manifests que chamam um mesmo step library. MVP mantém single trigger por workflow.

## 14. Resumo executivo

- **Feature:** core passa a suportar trigger modes `schedule` (cron) e `event` (subscribe no event stream), implementando os modes já reservados no schema.
- **Schedule:** background loop avalia cron expressions, dispatcha workflows, persiste `lastFiredAt` para idempotência. Default `catchup_policy: skip` evita avalanche.
- **Event:** background loop é subscriber do event stream, faz match contra triggers registered, dispatcha workflows matching. Leader election via advisory lock previne duplicação.
- **Eventos emitidos:** `workflow.run.dispatched` com `metadata.triggered_by: "schedule" | "event" | "manual"`.
- **Depende de:** event stream (subscription para event triggers, emission para ambos).
- **Habilita:** CD pipelines event-driven, cron jobs nativos, notifications em eventos, auto-deploy on manifest change, periodic cleanup.
- **Filosofia alinhada:** core gerencia quando workflows rodam, integrations podem produzir eventos externos mas não decidem o dispatch.
