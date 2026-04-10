# RPC Pagination — yggdrasil-core

**Data:** 2026-04-10
**Status:** Spec em revisão
**Tipo:** Core enhancement (existing RPCs)
**Prioridade:** 🔴 Alta — escalabilidade de state queries
**Escopo:** Adicionar cursor-based pagination a todas as list RPCs do core, de forma consistente e backwards-compatible.
**Depende de:** nada
**Parte do audit report:** [`2026-04-10-yggdrasil-product-audit-report.md`](./2026-04-10-yggdrasil-product-audit-report.md) Gap 2

---

## 1. Contexto e motivação

### 1.1 O problema

Hoje, os list RPCs do core retornam **todos os resultados** sem limite nem cursor. Exemplos:

- `yggdrasil-core.manifest.list` retorna todos os manifests que casam com os filtros
- `yggdrasil-core.collaborator.list` retorna todos os collaborators
- `yggdrasil-core.team.list` retorna todos os teams
- `yggdrasil-core.product.list` (via `console.products` HTTP endpoint) retorna todos os products

Em ecossistemas pequenos (dezenas de manifests), isso funciona. Em ecossistemas grandes (centenas ou milhares de manifests), isso vira:

- **Latência alta** em queries → response bodies grandes demais
- **Memória no core** → carregar tudo em memória antes de serializar
- **Payloads gigantes na wire** → RabbitMQ message size limits (default 128MB) podem ser atingidos
- **Surfaces precisam paginar client-side** → carregam tudo e jogam fora o que não precisam

### 1.2 Por que é core

Core serve state queries. Surfaces (yggdrasil-api, console, CLI) consomem essas queries para expor listagens. **Surfaces não podem paginar o que core não pagina** — elas só veem o que core entrega. Se core retorna 10.000 items, a surface recebe 10.000 items e tem que paginar no cliente (absurdo para UI, impossível para CLI).

Pagination tem que ser **feature do core** para ser efetiva. Cada list RPC precisa aceitar cursor + limit e retornar `next_cursor` para continuação.

### 1.3 Filosofia alinhada

Esta feature não adiciona novos concerns ao core — **ela faz o que core já faz, de forma escalável**. Serve state queries. É um fix de escalabilidade em primitive existente, não uma nova primitive.

## 2. Design

### 2.1 Princípios

1. **Cursor-based**, não offset-based. Cursor é mais seguro em listas que mudam (offset pode pular ou duplicar items quando novas entries são inseridas); cursor usa um identifier estável.
2. **Backwards-compatible**: RPCs existentes continuam aceitando requests sem pagination params. Quando pagination não é especificada, core aplica um **default limit** (ex: 100 items) e retorna `has_more: true` se houver mais. Callers antigos veem os primeiros 100 items sem erro.
3. **Consistente entre RPCs**: todas as list RPCs usam a mesma estrutura de `PaginationRequest` e `PaginationResponse`. Single convention.
4. **Cursor opaco para o caller**: o cursor é uma string codificada pelo core. Callers não devem parsear nem construir cursors — apenas passar adiante o `next_cursor` recebido.
5. **Filters continuam existindo**: pagination é ortogonal a filters. Filters reduzem o conjunto antes de paginar.

### 2.2 Estrutura do request

Todas as list RPCs passam a aceitar um campo opcional `pagination`:

```json
{
  "filters": {
    "namespace": "dakasa",
    "kind": "product",
    "active_only": true
  },
  "pagination": {
    "cursor": "eyJpZCI6IjEyMyIsInNlcSI6NDUifQ==",
    "limit": 50
  }
}
```

**Campos:**

- `pagination.cursor` (string, optional): cursor opaco retornado pelo servidor em um call anterior. **Primeiro call** omite ou passa `""` (vazio).
- `pagination.limit` (int, optional): máximo de items a retornar neste call. Se omitido, default = 100. Máximo = 1000.

### 2.3 Estrutura do response

```json
{
  "items": [ /* array de items específicos do RPC */ ],
  "pagination": {
    "next_cursor": "eyJpZCI6IjE3MyIsInNlcSI6OTV9",
    "has_more": true,
    "total_estimate": 427
  }
}
```

**Campos:**

- `items`: array de items. Tamanho ≤ `limit`.
- `pagination.next_cursor` (string): cursor para o próximo call. Quando `has_more: false`, `next_cursor` pode ser `""` (vazio) ou o mesmo do call anterior — callers devem checar `has_more`.
- `pagination.has_more` (bool): `true` se há mais items após este batch, `false` se este é o último.
- `pagination.total_estimate` (int, optional): estimativa de total de items matching o filter. **Não é exato** (pode usar PostgreSQL `EXPLAIN` stats). Provido como hint para UIs que queiram mostrar "Showing 50 of ~400". Se o RPC não suportar estimate eficiente, o campo é omitido.

### 2.4 Formato do cursor

O cursor é **opaco para o caller** mas tem estrutura interna conhecida pelo core. Formato:

```json
{
  "last_id": "018f2b4a-5678-4abc-def0-987654321098",
  "last_sort_value": "2026-04-10T12:34:56Z",
  "sort_key": "created_at_desc"
}
```

Serializado como **base64 de JSON**. Exemplo: `eyJsYXN0X2lkIjoiMTIzIiwibGFzdF9zb3J0X3ZhbHVlIjoiMjAyNi0wNC0xMFQxMjozNDo1NloiLCJzb3J0X2tleSI6ImNyZWF0ZWRfYXRfZGVzYyJ9`.

**Por que base64 de JSON:**
- Opaco para caller (não parece estruturado)
- Trivial de deserializar pelo core
- Evolução fácil (adicionar campos sem breaking change)
- Debuggável via base64 decode manual se necessário

### 2.5 Sort keys

Cada list RPC tem um **sort default**. Pagination respeita o sort. Sort alternativos podem ser pedidos via campo `sort`:

```json
{
  "filters": { ... },
  "sort": "updated_at_desc",
  "pagination": { "limit": 50 }
}
```

**Sort keys disponíveis** (varia por RPC):

| RPC | Default sort | Alternative sorts |
|---|---|---|
| `manifest.list` | `updated_at_desc` | `created_at_desc`, `name_asc`, `version_desc` |
| `collaborator.list` | `slug_asc` | `created_at_desc`, `display_name_asc` |
| `team.list` | `slug_asc` | `created_at_desc` |
| `product.list` | `namespace_asc, name_asc` | `created_at_desc` |
| `workflow.list` | `namespace_asc, name_asc` | `created_at_desc` |
| `buildproject.list` | `created_at_desc` | `expires_at_asc`, `expires_at_desc` |
| `topology_node.list` | `slug_asc` | `created_at_desc`, `updated_at_desc` |
| `event_stream.pull` | (já usa sequence ascending, não precisa mudar) | — |

### 2.6 Consistency guarantees

Durante uma sequência de pagination (cursor1 → cursor2 → cursor3 → ...):

- **Items adicionados depois** do primeiro call **podem ou não** aparecer em calls subsequentes, dependendo da sort key e dos valores novos.
  - Exemplo: sort = `created_at_desc`, cursor aponta para `last_sort_value = T1`. Se um novo item com `created_at > T1` aparecer, ele será retornado no próximo call (porque o server busca por `created_at < T1` para continuar).
  - Exemplo: sort = `name_asc`, cursor aponta para `last_sort_value = "charlie"`. Um novo item com `name = "bravo"` **não** aparecerá (porque está antes do cursor), mas um novo com `name = "delta"` aparecerá.
- **Items removidos** entre calls são skipados naturalmente (o cursor aponta para valores, não para índices).
- **Items atualizados** que mudam o valor da sort key podem aparecer em posições diferentes ou ser pulados.

**Garantia básica:** no sort key + cursor, core retorna items ordered + sem duplicatas **dentro de um único snapshot por call**. Consistency cross-call é **eventual**, não **strong**.

Isso é **idêntico ao comportamento do event_stream pull** (ver [event stream spec §5.5](./2026-04-10-event-stream-design.md)), para consistência filosófica.

## 3. Implementação

### 3.1 Go struct compartilhada

Em `model/pagination.go` (novo arquivo):

```go
package model

// PaginationRequest is the optional input for paginated list RPCs.
type PaginationRequest struct {
    Cursor string `json:"cursor,omitempty"`
    Limit  int    `json:"limit,omitempty"`
}

// PaginationResponse is included in all list RPC responses.
type PaginationResponse struct {
    NextCursor    string `json:"next_cursor"`
    HasMore       bool   `json:"has_more"`
    TotalEstimate int64  `json:"total_estimate,omitempty"`
}

// DefaultPaginationLimit é aplicado quando caller não especifica Limit.
const DefaultPaginationLimit = 100

// MaxPaginationLimit é o teto para Limit. Requests acima disso são capped.
const MaxPaginationLimit = 1000
```

### 3.2 Cursor encoding helper

Em `repository/pagination.go` (novo arquivo):

```go
package repository

import (
    "encoding/base64"
    "encoding/json"
)

// Cursor é a representação interna do cursor opaco.
type Cursor struct {
    LastID        string `json:"last_id"`
    LastSortValue any    `json:"last_sort_value"`
    SortKey       string `json:"sort_key"`
}

// EncodeCursor serializa Cursor como string opaca.
func EncodeCursor(c Cursor) string {
    raw, _ := json.Marshal(c)
    return base64.StdEncoding.EncodeToString(raw)
}

// DecodeCursor parseia a string opaca de volta para Cursor.
// Retorna zero-value Cursor se a string for vazia.
func DecodeCursor(s string) (Cursor, error) {
    if s == "" {
        return Cursor{}, nil
    }
    raw, err := base64.StdEncoding.DecodeString(s)
    if err != nil {
        return Cursor{}, err
    }
    var c Cursor
    if err := json.Unmarshal(raw, &c); err != nil {
        return Cursor{}, err
    }
    return c, nil
}
```

### 3.3 Query pattern por repository

Cada repository que expõe list precisa implementar o pattern:

```go
// Exemplo: repository/manifests.go
func (r *ManifestRepository) List(
    ctx context.Context,
    filters ListManifestsFilters,
    sort string,
    pagination model.PaginationRequest,
) ([]model.Manifest, model.PaginationResponse, error) {
    // 1. Normalize pagination
    if pagination.Limit <= 0 {
        pagination.Limit = model.DefaultPaginationLimit
    }
    if pagination.Limit > model.MaxPaginationLimit {
        pagination.Limit = model.MaxPaginationLimit
    }

    // 2. Decode cursor
    cursor, err := DecodeCursor(pagination.Cursor)
    if err != nil {
        return nil, model.PaginationResponse{}, fmt.Errorf("invalid cursor: %w", err)
    }

    // 3. Build SQL with cursor WHERE clause + ORDER BY + LIMIT+1
    //    (LIMIT+1 to detect if there are more items)
    query, args := r.buildListQuery(filters, sort, cursor, pagination.Limit+1)

    // 4. Execute query
    rows, err := r.db.QueryContext(ctx, query, args...)
    // ... scan into items

    // 5. Check if there are more (len > limit means yes)
    hasMore := len(items) > pagination.Limit
    if hasMore {
        items = items[:pagination.Limit]
    }

    // 6. Build next_cursor from last item
    var nextCursor string
    if len(items) > 0 {
        lastItem := items[len(items)-1]
        nextCursor = EncodeCursor(Cursor{
            LastID:        lastItem.ID,
            LastSortValue: extractSortValue(lastItem, sort),
            SortKey:       sort,
        })
    }

    return items, model.PaginationResponse{
        NextCursor: nextCursor,
        HasMore:    hasMore,
    }, nil
}
```

### 3.4 SQL pattern para cursor-based WHERE clause

Para `sort = "created_at_desc"` e `cursor.LastSortValue = "2026-04-10T12:34:56Z"`:

```sql
SELECT * FROM manifests
WHERE (created_at, id) < ($1, $2)  -- tuple comparison for (cursor_value, cursor_id)
  AND {filter conditions}
ORDER BY created_at DESC, id DESC
LIMIT $3
```

Usando **tuple comparison** `(created_at, id)` para garantir uniqueness mesmo quando múltiplos items têm o mesmo `created_at`. O `id` secundário resolve ties deterministicamente.

Para `sort = "name_asc"`:

```sql
SELECT * FROM manifests
WHERE (name, id) > ($1, $2)
  AND {filter conditions}
ORDER BY name ASC, id ASC
LIMIT $3
```

### 3.5 Backwards compatibility

Callers que **não** passam `pagination` no request continuam funcionando:

- `pagination.Cursor = ""` → começa do início
- `pagination.Limit = 0` → normalizado para `DefaultPaginationLimit = 100`
- Response ainda inclui `pagination.NextCursor` e `pagination.HasMore` — callers antigos podem ignorar esses campos
- Response **sempre** retorna até 100 items no máximo para callers antigos. Se havia mais, eles são truncados. Callers antigos recebem `HasMore: true` mas não sabem o que fazer com isso.

**Breaking change potencial:** callers antigos que esperavam "tudo" passam a receber até 100. Mitigation: documentar claramente no changelog. Callers que precisam de "tudo" podem passar `limit: 1000` (max) ou iterar via cursor.

**Alternativa**: um feature flag `legacy_full_list=true` que desabilita pagination e retorna tudo como antes. Mantido até todos os callers migrarem. Deprecated após uma release.

**Decisão MVP:** **sem feature flag**. Caller antigos passam a receber 100 items max. Breaking change documentado. Callers grandes migram para cursor-based.

### 3.6 Total estimate (opcional)

Para surfaces que queiram mostrar "Showing 50 of ~400", core pode prover estimate barato via PostgreSQL:

```sql
-- Para tabelas pequenas (<10k rows), use COUNT exato
SELECT count(*) FROM manifests WHERE {filters};

-- Para tabelas grandes, use EXPLAIN estimate (barato, aproximado)
EXPLAIN SELECT * FROM manifests WHERE {filters};
-- Parse output: "rows=427" → total_estimate: 427
```

**MVP decision:** não implementar `total_estimate` no primeiro cut. Campo é opcional no schema; surfaces podem deixar de mostrar o contador por enquanto. Adicionar via follow-up quando UI demandar.

## 4. RPCs afetadas

Todas as list RPCs do core precisam ser atualizadas. Lista completa:

| RPC | Repository | Sort default | Comment |
|---|---|---|---|
| `yggdrasil-core.manifest.list` | ManifestRepository | `updated_at_desc` | Principal lista; usado pelo console catalog |
| `yggdrasil-core.collaborator.list` | IdentityRepository | `slug_asc` | Lista de colaboradores |
| `yggdrasil-core.team.list` | IdentityRepository | `slug_asc` | Lista de teams |
| `yggdrasil-core.team.membership.list` | IdentityRepository | `team_slug_asc, collaborator_slug_asc` | Memberships |
| `yggdrasil-core.integration.catalog.list` | (derivado de manifests) | `domain_asc, section_asc, entry_asc` | Catálogo de plugins |
| `yggdrasil-core.integration.status.list` | IntegrationRepository | `name_asc` | Integration instance health listing |
| `yggdrasil-core.integration.instance_health.list` | IntegrationRepository | `name_asc` | Instance health detail |
| `yggdrasil-core.topology.node.list` | TopologyRepository | `slug_asc` | Nodes |
| `yggdrasil-core.topology.edge.list` | TopologyRepository | `created_at_desc` | Edges |
| `yggdrasil-core.topology.document.list` | TopologyRepository | `created_at_desc` | Documents |
| `yggdrasil-core.topology.build_project.list` | TopologyRepository | `created_at_desc` | Build projects (inclui expires_at_asc como alt sort) |
| `yggdrasil-core.product.list` | ProductRepository | `namespace_asc, name_asc` | Lista de products (via manifest.list filter on kind) |
| `yggdrasil-core.workflow.list` | WorkflowRepository | `namespace_asc, name_asc` | Lista de workflows (via manifest.list filter on kind) |
| `yggdrasil-core.event_stream.pull` | EventRepository | (sequence_asc, already paginated) | **Não precisa mudar** — event stream já é paginado |

**Total:** 13 RPCs afetadas. Event stream é exceção porque já foi desenhado com cursor desde o início.

## 5. Migration plan

### Fase 1 — Infraestrutura compartilhada

- [ ] `model/pagination.go` com `PaginationRequest`, `PaginationResponse`, constants
- [ ] `repository/pagination.go` com `Cursor`, `EncodeCursor`, `DecodeCursor`
- [ ] SQL helper para construir WHERE clause de cursor (tuple comparison)
- [ ] Unit tests cobrindo encode/decode cursor, normalization, edge cases

### Fase 2 — RPCs críticas

Começar pelas RPCs mais consumidas:

- [ ] `manifest.list` — adicionar pagination + sort
- [ ] `collaborator.list` — adicionar pagination + sort
- [ ] `team.list` — adicionar pagination + sort
- [ ] `integration.catalog.list` — adicionar pagination + sort
- [ ] Integration tests cobrindo: primeiro call, subsequent calls, filter + pagination combinado, sort alternativos

### Fase 3 — RPCs restantes

- [ ] `team.membership.list`
- [ ] `integration.status.list`
- [ ] `integration.instance_health.list`
- [ ] `topology.node.list`
- [ ] `topology.edge.list`
- [ ] `topology.document.list`
- [ ] `topology.build_project.list`

### Fase 4 — Documentação e cleanup

- [ ] Atualizar JSON Schemas de cada RPC request/response em `docs/contracts/`
- [ ] Documentar o pattern geral em `docs/contracts/pagination/v1/README.md`
- [ ] Migration notes no CHANGELOG sobre o breaking change de default limit
- [ ] Surfaces (yggdrasil-api, yggdrasil-console) adaptadas para passar/consumir pagination

## 6. Testing

### 6.1 Casos críticos

- **First call sem cursor**: cursor="", limit=50 → retorna primeiros 50, next_cursor non-empty, has_more baseado em total
- **Subsequent calls**: next_cursor é passed forward → continua de onde parou, sem duplicatas
- **Último batch**: retorna N items onde N < limit, has_more=false, next_cursor pode ser empty
- **Empty result**: filter matcha 0 items → items=[], has_more=false
- **Filter mudando durante iteration**: items adicionados/removidos entre calls — per consistency §2.6, comportamento é eventual
- **Invalid cursor**: cursor corrompido → erro 400 (`"invalid cursor"`)
- **Limit 0**: normalizado para 100
- **Limit > 1000**: capped para 1000
- **Cursor + filter combinado**: filter aplicado antes de pagination; cursor avança dentro do filtered set

### 6.2 Performance tests

- List com 100k items, pagination de 50 em 50 → query time deve ser O(log N + limit) por page
- List com 1M items + cursor → query time não degradada por offset-like behavior

Com índices corretos `(sort_field, id)`, cursor-based pagination é O(log N + limit) garantido.

## 7. Considerações de performance

### 7.1 Índices necessários

Cada tabela que é listada precisa ter índice composto `(sort_field, id)` para que tuple comparison seja eficiente:

```sql
CREATE INDEX manifests_updated_at_idx ON manifests (updated_at DESC, id DESC);
CREATE INDEX manifests_created_at_idx ON manifests (created_at DESC, id DESC);
CREATE INDEX manifests_name_idx ON manifests (name ASC, id ASC);
CREATE INDEX collaborators_slug_idx ON collaborators (slug ASC, id ASC);
-- etc.
```

Se a tabela já tem um índice simples em `sort_field`, adicionar o `, id` é um index replacement barato (mas não atômico — considerar blue-green).

### 7.2 Filter + cursor combinado

Quando filter e cursor são aplicados juntos, a ordem importa para performance:

```sql
-- Bom: filter + cursor no WHERE, index (kind, updated_at DESC, id DESC)
SELECT * FROM manifests
WHERE kind = 'product'
  AND (updated_at, id) < ($1, $2)
ORDER BY updated_at DESC, id DESC
LIMIT 50;
```

O ideal é ter índices compostos `(filter_field, sort_field, id)` para os filter+sort mais comuns. Provido no migration junto com a feature.

### 7.3 Sort alternativos

Cada sort alternativo precisa do seu próprio índice. Isso pode multiplicar índices por 3-5x em tabelas afetadas. **Trade-off:** mais storage + write latency, menos read latency.

**MVP decision:** criar apenas o índice para o sort default de cada RPC. Sorts alternativos podem ser adicionados sob demanda.

## 8. Compatibilidade com yggdrasil-core atual

### 8.1 O que muda

- **Todos os handlers de list RPC** precisam ser atualizados para aceitar o novo `pagination` field
- **Repositories** precisam implementar cursor-based queries (SQL pattern novo)
- **Índices** novos em migrations
- **JSON Schemas de contracts** atualizados

### 8.2 O que NÃO muda

- Schema das entidades (manifests, collaborators, etc.) intacto
- Filters existentes: intactos, apenas combinam com pagination
- Event stream: já paginado, não afetado

### 8.3 Surfaces afetadas

- `yggdrasil-api`: passa pagination params adiante, retorna pagination metadata
- `yggdrasil-console`: UI de listagens precisa lidar com "load more" ou paginação visual
- `ygg` CLI: comandos de list precisam suportar `--cursor`, `--limit`, ou iterar automaticamente

Surfaces são atualizadas em follow-up após o core support estar pronto.

## 9. Alternativas consideradas

### 9.1 Offset-based pagination

`SELECT * FROM manifests LIMIT 50 OFFSET 100`

**Rejeitado porque:**
- OFFSET é O(N+limit) no PostgreSQL (tem que skippear rows)
- Items podem duplicar ou ser pulados se a lista mudar durante iteration
- Menos consistente com event stream (que já usa cursor)

### 9.2 Page number pagination

`?page=3&size=50`

**Rejeitado porque:**
- Equivalente a offset-based (mesmos problemas)
- UX enganosa em listas mutáveis ("página 3" pode ter items diferentes dependendo de quando foi acessada)

### 9.3 Keyset pagination com timestamp único

Usar só `after: <timestamp>` sem id tiebreaker.

**Rejeitado porque:**
- Items com timestamps idênticos são skipados ou duplicados
- Tuple comparison `(timestamp, id)` resolve isso elegantemente

### 9.4 GraphQL-style Relay cursors

Cursors são globais, não per-field.

**Rejeitado porque:**
- Mais complexo de implementar
- REST/RPC style não precisa desse overhead
- Nosso modelo é suficiente para list use cases

## 10. Riscos e mitigações

| Risco | Impacto | Mitigação |
|---|---|---|
| Breaking change no default limit pega callers existentes de surpresa | Médio | Documentar claramente no CHANGELOG; beta release para surfaces conhecidas (api, console) primeiro |
| Índices novos adicionam write latency | Baixo | Índices compostos existem hoje em outras features; trade-off aceito |
| Cursor com tuple comparison tem bugs em casos edge (NULLs, collations) | Médio | Unit tests exaustivos cobrindo NULLs, strings com chars especiais, timestamps em UTC vs local |
| Filter ineficiente + cursor = slow query | Médio | Monitoring + EXPLAIN em queries comuns durante testing; índices compostos onde necessário |
| Sort key inválido crash o handler | Baixo | Whitelist de sorts válidos por RPC; default para sort padrão se inválido |

## 11. Critérios de aceitação

- ✅ Todas as 13 list RPCs aceitam `pagination` field opcional
- ✅ Default limit = 100, max limit = 1000 aplicado
- ✅ Cursor opaco (base64 JSON) consistente entre RPCs
- ✅ `next_cursor` + `has_more` retornados em todos os responses
- ✅ Backwards compatible: callers sem pagination recebem 100 items max
- ✅ Sort alternativos funcionais para RPCs que declaram suportar
- ✅ Índices PostgreSQL criados para sorts default
- ✅ Integration tests cobrindo pattern completo
- ✅ JSON Schemas atualizados em `docs/contracts/`
- ✅ CHANGELOG documenta o breaking change

## 12. Dependências

- **Depende de:** nada. Feature independente.
- **Habilita:** escalabilidade de qualquer consumer de list RPCs (console, CLI, integrations que listam state)

## 13. Pontos em aberto

- **Sort alternativos por RPC**: a tabela em §2.5 tem propostas, mas sort concreto deve ser decidido por RPC com base em demanda real. Começar só com defaults no MVP.
- **`total_estimate`**: implementação adiada. Se surfaces pedirem, adicionar via follow-up.
- **Deprecation path para callers old**: **sem** feature flag legacy. Breaking change único no changelog. Alternativa (feature flag) foi rejeitada para simplicidade.

## 14. Resumo executivo

- **Feature:** pagination cursor-based em todas as list RPCs do core
- **Padrão único:** `PaginationRequest{cursor, limit}` + `PaginationResponse{next_cursor, has_more, total_estimate?}`
- **Cursor opaco:** base64 de JSON com `(last_id, last_sort_value, sort_key)`
- **Backwards compatible:** callers sem pagination recebem default limit = 100
- **Performance:** O(log N + limit) com índices compostos corretos
- **Escopo:** 13 RPCs afetadas; event stream já era paginado
- **Breaking change:** callers antigos passam a receber max 100 items; documentado no CHANGELOG
