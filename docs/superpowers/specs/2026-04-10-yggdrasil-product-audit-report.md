# Yggdrasil Product Audit Report

**Data:** 2026-04-10
**Status:** Primeira versão, após survey holístico + re-revisão crítica
**Escopo:** Auditoria de produto do Yggdrasil como plataforma competitiva (vs Backstage, Crossplane, ArgoCD, Helm, Port, Terraform Cloud, Pulumi)
**Motivação:** DaKasa quer usar Yggdrasil para orquestrar sua infraestrutura cloud, e Yggdrasil é tratado como produto de primeira classe — não ferramenta auxiliar. Esta auditoria identifica onde Yggdrasil já é forte e onde precisa evoluir para ser adotável por outras empresas.

---

## 1. A Filosofia do Yggdrasil

Antes de qualquer análise de gaps ou benchmark, é essencial explicitar a filosofia que move o produto. A auditoria inteira deve ser lida sob essa lente.

### 1.1 Visão central

> **"Ecossistemas devem se tornar banais e descartáveis, tão fáceis quanto fazer deploy de um artefato só, mas muito mais granular."**

Yggdrasil existe para que subir, derrubar, e gerenciar ecossistemas inteiros de infraestrutura seja trivial. Um ecossistema — seja um produto como DaKasa com dezenas de microserviços, ou uma plataforma de dados com pipelines, ou um stack de observability — deve ser tratado como um **artefato composto**, versionável, reproduzível e descartável.

### 1.2 O quebra-cabeça extensível

Yggdrasil é um **quebra-cabeça**. A única peça que não é removível é `yggdrasil-core`. Tudo o mais é composição:

- **Core** (`yggdrasil-core`) — motor que gerencia estado de ecossistema com maestria. Não é removível.
- **Integrations** — peças externas que conectam com provedores de infra (AWS, GCP, Kubernetes, GitHub, RabbitMQ, Grafana, Heimdall, etc.). Plugáveis via RPC. Qualquer um pode escrever novas.
- **Surfaces** — "libs" na borda que dão interfaces humanas/API (yggdrasil-api REST, yggdrasil-console web UI, yggdrasil-identities JWT auth). **Também removíveis.** Um usuário pode substituir yggdrasil-identities pelo seu próprio sistema de auth e yggdrasil-console pelo seu próprio front-end.

Essa é uma decisão filosófica fundamental: **mesmo auth e console são edges removíveis**. Core não depende de nenhuma surface específica. Core é trust-boundary por design — não sabe (nem quer saber) como a surface autenticou o caller.

### 1.3 Core gerencia estado com maestria

Core **não é apenas um dispatcher RPC**. Core é responsável por **gerenciar o estado do ecossistema com maestria**:

- Estado de manifests (versionamento, validation, checksum)
- Estado de topology (nodes, edges, documents, build projects)
- Lifecycle de ephemerais (BuildProject com `ExpiresAt`)
- Estado de execução (workflow runs, product materializations, integration runtime state)
- **Trilha canônica de mutações de estado via event stream**
- Scheduler nativo (schedule-driven workflow dispatch)
- Event-driven dispatching (workflows disparados por eventos)

Integrations podem **produzir** eventos externos (webhook receivers, Kafka consumers), mas **a decisão de quando disparar** e **o canonical log de mutações** são core concerns. Um evento externo chega via integration, o core decide o que fazer com ele baseado em suas primitives internas.

### 1.4 Contract-first, não language-first

Yggdrasil nunca amarra extensibilidade a uma linguagem específica. Todo contrato — eventos, RPC requests/responses, manifest schemas — é definido como **JSON Schema** em `docs/contracts/`, não como tipos Go compartilhados. Plugin authors em Python, Rust, TypeScript, ou qualquer outra linguagem conseguem escrever integrations e surfaces sem importar `yggdrasil-core`. A barreira de entrada é baixa por design.

### 1.5 Implicação para design de features

Ao adicionar qualquer feature ao Yggdrasil, a primeira pergunta é: **"isso pode ser uma integration, uma surface, ou tem que estar no core?"**

- Se pode ser **integration** (scheduler externo, audit log shipper para SIEM, backup automation, pull-based deployment agent, webhook event receiver) → **é integration**
- Se pode ser **surface** (CLI, web console, REST API, GraphQL, gRPC, OpenAPI export) → **é surface**
- Se **tem que estar no core** (manifest validation, RPC dispatcher, authorization pipeline, topology graph, event stream emission, lifecycle management, scheduled/event-driven dispatching) → **só então vai pro core**

Esta auditoria aplica rigorosamente esse filtro.

## 2. O que é Core, e por quê

Core é surpreendentemente completo. Após re-revisão crítica, identificamos que muitas coisas que pareciam "gaps" são na verdade **decisões arquiteturais intencionais**. Outras são **responsabilidades deliberadamente delegadas a integrations ou surfaces**. E algumas, enfim, são gaps reais.

### 2.1 O que core já faz bem

| Capability | Estado | Nota |
|---|---|---|
| **7 manifest kinds** (RBAC, Policy, Integration_Type, Integration_Instance, Resource, Product, Workflow) | ✅ Completo e polished | Schema bem validado, checksum, versioning, namespace isolation |
| **Topology system** (nodes, edges, documents, build_projects) | ✅ Completo | Estrutura hierárquica com parent chain, acesso herdado |
| **RBAC + Policy evaluation** (dual-layer authorization) | ✅ Completo | Wildcards, deny precedence, conditional policies, subject expansion recursiva |
| **Integration plugin architecture** (contract-first via JSON Schema) | ✅ Completo | 9 integrations production-ready, RPC via RabbitMQ (hoje) |
| **Transport pluggable** (`IntegrationAdapterSpec.Transport`) | ✅ Schema-level | Apenas `rabbitmq` implementado, mas arquitetura permite HTTP/gRPC/outros |
| **Product materialization + reconcile + apply + observe** | ✅ Completo | Lifecycle bem definido; product_materializations como audit snapshot |
| **Workflow execution** (templating, depends_on, retry, timeout) | ✅ Completo | Runtime de workflow é funcional para mode=manual |
| **HA via multiple workers** (consumir das mesmas RabbitMQ queues) | ✅ Natural | Múltiplas replicas funcionam sem leader election; limitação é acidental (deploy único) |
| **Heimdall AI governance** (LLM fallback com Claude/GPT) | ✅ Completo | Chamadas HTTP reais a Anthropic/OpenAI, policy-gated |
| **Manifest versioning com checksum** | ✅ Completo | Imutabilidade garantida, soft delete, labels para filter |

### 2.2 Escolhas de design propositalmente minimalistas

Estes itens parecem "faltando" à primeira vista, mas são **deliberadamente fora do core** por alinhamento com a filosofia:

| Item | Por que não está no core |
|---|---|
| **Plugin SDK compartilhado** | Contract-first > language-first. JSON Schemas em `docs/contracts/` permitem plugins em qualquer linguagem. SDK criaria dependency |
| **Plugin marketplace com approval workflow** | Catalog já é derivado de manifest labels. Marketplace seria uma **surface** acima, não core |
| **Custom manifest kinds** | Os 7 kinds são universais. Especialização por domínio acontece via `integration_type.resource_types` ou `metadata.labels` |
| **Custom renderers além de raw_k8s/kustomize/helm** | Os 3 renderers cobrem as delivery strategies principais. Novos renderers requerem contribuição ao core |
| **Product-level templating/parametrização** | Products são artefatos compilados imutáveis. Parametrização é concern de **workflow**, que tem templating `{{}}` nativo |
| **Backup automation built-in** | PostgreSQL/RabbitMQ backup é responsabilidade do substrato operacional. Core não reimplementa infra de backup |
| **Terraform / ArgoCD / Flux integration** | Yggdrasil é **competidor** destes, não extensão. Integrations para estes fariam sentido apenas como "bridges" |
| **WebSocket / streaming API no core** | Core oferece RPC síncrono. Streaming é concern de **surface** (console pode usar SSE, etc.) |
| **Rollback de products** | Delegado ao substrato Kubernetes (kubectl rollout undo) ou reaplicação de version anterior do manifest |
| **Auth como feature core** | Auth é surface removível. Core aceita identity forward opcional via `auth` field nas RPC requests |
| **OpenAPI/Swagger export no core** | Surface concern. yggdrasil-api pode (deveria) exportar seu próprio OpenAPI |

### 2.3 Matriz de separação core / integration / surface

| Concern | Onde fica | Justificativa |
|---|---|---|
| Manifest storage, versioning, validation | **Core** | Estado do ecossistema |
| Authorization evaluation (RBAC + Policy) | **Core** | Authorization pipeline |
| Topology graph management | **Core** | Estado do ecossistema |
| BuildProject lifecycle (incluindo expiration) | **Core** | Lifecycle de ephemerais |
| Event stream de mutações de estado | **Core** | Trilha canônica |
| Workflow execution engine | **Core** | State-bearing primitive |
| Scheduled workflow dispatching | **Core** | Parte do lifecycle |
| Event-driven workflow dispatching (core side) | **Core** | Parte do lifecycle |
| Product materialization/apply/observe | **Core** | Orquestração de estado |
| External provider operations (create S3 bucket, apply K8s, etc.) | **Integration** | Concerne o mundo externo |
| Webhook receiver (HTTP endpoint para eventos externos) | **Integration** | Mundo externo |
| Kafka/Pulsar event consumer | **Integration** | Mundo externo |
| Cron daemon externo (alternative scheduler) | **Integration** | Opcional; core tem scheduler próprio |
| Audit log shipper para SIEM/Splunk | **Integration** | Consumer do event stream |
| Backup automation | **Integration** | Talvez, ou delegado à infra |
| Slack/Discord/email notifications | **Integration** | Consumer do event stream |
| Prometheus metrics export | **Integration** ou **Surface** | Depende de quem consome |
| REST API / GraphQL | **Surface** | Interface humana/API |
| Web console / admin UI | **Surface** | Interface humana |
| CLI (`ygg`, etc.) | **Surface** | Interface humana |
| Auth provider (JWT, OAuth, SAML) | **Surface** | Auth é edge concern |
| OpenAPI schema export | **Surface** | Surface-specific |

## 3. Core Gaps (5)

Estes são os **únicos gaps reais no core** do Yggdrasil após a re-revisão crítica, alinhados com a filosofia:

### Gap 1: State Change Event Stream (fundacional)

**O que falta:** core não emite events de mudanças de estado, e não há mecanismo de subscription para consumers.

**Por que é core:** só core tem visão canônica de mutações. Integrations e surfaces precisam consumir esse stream para construir projeções (audit log, workflow history, activity feed, cost tracking, SIEM export, etc.). **Esta é a primitive mais fundacional** porque várias outras features (BuildProject lifecycle, workflow trigger system, audit log, history) podem se construir sobre ela.

**Projeções que consumers podem construir uma vez que a primitive existir:**
- Audit log para compliance (integration subscribe e envia para SIEM)
- Workflow run history (reconstrói de events `workflow.*`)
- BuildProject lifecycle history (de events `buildproject.*`)
- Manifest change history (de events `manifest.*`)
- Real-time activity feed no console (surface subscribe via API)
- Slack/email notifications em eventos críticos (integration)
- Prometheus metrics em contagem de events por tipo (integration/surface)

**Spec dedicada:** [`2026-04-10-event-stream-design.md`](./2026-04-10-event-stream-design.md)

### Gap 2: Pagination em list RPCs

**O que falta:** list queries (`manifest.list`, `product.list`, `workflow.list`, `collaborator.list`, etc.) retornam todos os resultados sem cursor/limit/offset.

**Por que é core:** core serve state queries; surfaces não podem paginar o que core não pagina (carregariam tudo primeiro). Essencial para escalabilidade em ecossistemas grandes.

**Escopo estimado:** pequeno. Adicionar `PaginationRequest { cursor, limit }` e `PaginationResponse { items, next_cursor }` ao schema dos list RPCs, implementar cursor-based pagination com ORDER BY estável no PostgreSQL.

**Spec dedicada:** será escrita no batch 2.

### Gap 3: Workflow → product.apply com target overrides

**O que falta:** hoje, workflows chamam integration operations diretamente. Não há um primitive que permita um workflow disparar `product.installation.apply` passando um `target_overrides` map (ex: `{ "kubernetes-default": "kubernetes-dakasa-validation" }`) para resolver multi-environment sem duplicar Products.

**Por que é core:** é composição entre primitives existentes do core (workflow + product). Preserva a imutabilidade dos Products (decisão filosófica: products são artefatos compilados) enquanto coloca a dinamicidade de env no layer correto (workflow).

**Motivação concreta:** no uso do DaKasa, queremos que um Product `dakasa-platform` materialize o ecossistema DaKasa em qualquer environment (validation, staging, prod, ephemeral) sem precisar duplicar o manifest por env. Um workflow `deploy-dakasa` recebe `env` como input e faz `product.installation.apply` com targets apontando para o cluster correto daquele env.

**Spec dedicada:** será escrita no batch 2.

### Gap 4: BuildProject lifecycle enforcement (auto-expiration + cleanup)

**O que falta:** `BuildProject` tem campos `Ephemeral: bool` e `ExpiresAt: string` no model, mas **nada enforca** a expiration. Um BuildProject ephemeral marcado para expirar em 48h permanece no sistema indefinidamente se ninguém limpar manualmente.

**Por que é core:** lifecycle management de ephemerais é core concern (filosofia: ecossistemas banais e descartáveis). Se ephemerais não expirarem automaticamente, a promessa filosófica se quebra — ecossistemas vazam.

**Mecanismo proposto:** background loop em core que periodicamente consulta BuildProjects com `Ephemeral=true AND ExpiresAt < now()`, dispara workflow de teardown (configurável por BuildProject), emite event `buildproject.expired`, e marca como deleted. Usa o event stream (Gap 1) para publicação.

**Spec dedicada:** será escrita no batch 2.

### Gap 5: Workflow trigger system (schedule + event)

**O que falta:** `supportedWorkflowTriggerModes = ["manual", "event", "schedule"]` está declarado em `manifest/workflow.go:14`, mas só `manual` funciona. Schedule e event são schema-reserved, sem implementação.

**Por que é core:** core gerencia quando workflows rodam (parte do lifecycle). Integrations podem produzir eventos (webhook receiver integration) ou fornecer scheduler alternativo (cron externo integration), mas a decisão de disparar é core.

**Mecanismo proposto:**
- **Schedule**: background loop em core avalia workflows com `trigger.mode=schedule` e `trigger.schedule=<cron expr>`. Quando chega a hora, emite event `workflow.schedule.fired` e dispara o workflow.
- **Event**: workflows com `trigger.mode=event` se registram como subscribers no event stream (Gap 1) com filtros (tipo de evento, campo do payload, etc.). Quando um matching event é emitido, core dispara o workflow.

Ambos dependem de **Gap 1 (event stream)** como infraestrutura subjacente.

**Spec dedicada:** será escrita no batch 2.

---

## 4. Design Philosophy Comparison (em vez de feature benchmark)

Originalmente, pretendia-se um feature-by-feature benchmark contra Backstage, ArgoCD, Crossplane, Helm, Terraform Cloud, Pulumi, etc. Após a re-revisão crítica, essa abordagem se mostrou errada: Yggdrasil **optou deliberadamente por composabilidade em vez de feature completeness**. Comparar via matriz de features puniria Yggdrasil injustamente por escolhas intencionais.

A comparação correta é **philosophy-level**. Abaixo, narrativa breve de posicionamento relativo:

### 4.1 vs Backstage (Spotify)

Backstage é um **developer portal monolithic React app** com ecossistema de plugins pesados. Software catalog + scaffolder + TechDocs tudo dentro de uma webapp. Seus plugins são instalados dentro do Backstage, não são serviços independentes.

**Yggdrasil é o oposto:** peças independentes (integrations e surfaces), com core minimalista. Backstage's "strength" é ter muita coisa pronta out-of-box; Yggdrasil's strength é que você pode substituir qualquer peça sem tocar no resto.

**Quando escolher Backstage:** quando você quer um developer portal pronto, opinativo, com a UX que Spotify desenhou.

**Quando escolher Yggdrasil:** quando você quer montar um developer portal (ou infrastructure orchestrator, ou data platform, ou qualquer ecossistema) com as peças que você escolher, na linguagem que você escolher, com a UX que você construir.

### 4.2 vs Crossplane

Crossplane é **Kubernetes-native infrastructure orchestration**. Toda a infraestrutura cloud é modelada como K8s CRDs. Tudo passa pelo K8s API.

**Yggdrasil não é Kubernetes-native.** Yggdrasil pode orquestrar Kubernetes (via `integration-kubernetes`), e pode orquestrar outras coisas (AWS diretamente, GitHub, Grafana, RabbitMQ, etc.) sem precisar modelar tudo como CRD. Yggdrasil não assume que o usuário quer K8s como substrato de orquestração.

**Quando escolher Crossplane:** quando seu time já vive dentro de Kubernetes e quer usar GitOps + kubectl + K8s controllers para gerenciar infra.

**Quando escolher Yggdrasil:** quando você quer orquestrar infra sem forçar tudo a ser K8s CRDs, e quer composabilidade em outras dimensões (auth, console, event stream, governance) além de apenas provisioning.

### 4.3 vs ArgoCD / Flux

ArgoCD é **pull-based GitOps** para Kubernetes. Um agent dentro do cluster puxa manifests de git e aplica. Focado em "desired state sync" contínuo.

**Yggdrasil é push-based hoje.** Core chama `integration-kubernetes` via RPC para fazer apply remotamente. Isso significa: core precisa acesso ao cluster K8s API (via kubeconfig ou service account). Vantagem: orquestração centralizada, mais fácil de coordenar multi-cluster. Desvantagem: precisa de conectividade outbound-to-clusters do core.

**Pull-based Yggdrasil é possível no futuro** como uma `integration-kubernetes-pull` adicional, onde um agent dentro do cluster puxa Product manifests do core e aplica localmente. Core não precisa escolher push ou pull — é uma decisão do operador, via qual integration ele instala.

**Quando escolher ArgoCD:** quando sua entire stack é Kubernetes e você quer o modelo GitOps puro, drift detection, e UX de "aplicação" focada em K8s workloads.

**Quando escolher Yggdrasil:** quando você quer orquestrar muito além de K8s (inclui provisioning cloud, gestão de integrations de terceiros, eventos, workflows, governance), e quer escolher push ou pull por workload.

### 4.4 vs Helm

Helm é **templating de Kubernetes manifests + package management para OCI registries**. Charts têm values.yaml, templates, hooks, releases.

**Helm é um renderer**, não um orquestrador. Yggdrasil usa Helm como um `renderer.kind` opcional dentro de Products — ou seja, Helm é **absorvido como peça do quebra-cabeça** do Yggdrasil quando necessário.

**Não é competidor** — é complementar. Yggdrasil orquestra; Helm renderiza (quando você escolhe Helm como renderer).

### 4.5 vs Terraform Cloud / Pulumi

Terraform e Pulumi são **IaC languages** com estado centralizado. Terraform Cloud / Pulumi Cloud adicionam collaboration, run history, policy enforcement (Sentinel/CrossGuard), secrets.

**Yggdrasil poderia ter um `integration-terraform`** que wrapeia terraform CLI e expõe plan/apply como operations. Nesse modelo, Yggdrasil é o orquestrador, Terraform é o provisioner. Composabilidade — não replacement.

**Quando escolher Terraform Cloud:** quando você já tem muita infra em HCL e quer a experiência Terraform-native completa (plan, apply, state, modules).

**Quando escolher Yggdrasil:** quando você quer orquestrar Terraform junto com outras 10 coisas (deploys K8s, configurações RabbitMQ, dashboards Grafana, webhooks GitHub, etc.) num mesmo plano declarativo, sem transformar tudo em HCL.

### 4.6 vs Port (getport.io)

Port é **developer portal SaaS moderno** com blueprints, entities, self-service actions. Mais moderno que Backstage em UX, mas ainda uma SaaS opinativa.

**Yggdrasil vs Port** tem trade-off similar a Backstage: Port tem UX pronta, Yggdrasil tem composabilidade. Port não é self-hostable. Yggdrasil é self-hostable desde o dia 1.

### 4.7 Posicionamento unificado

**Yggdrasil's pitch:**

> "Uma plataforma de orquestração de ecossistemas onde cada peça é removível. Core gerencia estado com maestria; integrations conectam com qualquer infraestrutura; surfaces dão a interface que você quiser. Ecossistemas se tornam artefatos banais e descartáveis. Escreva plugins em qualquer linguagem, via contracts JSON Schema. Substitua auth e console se quiser. Core continua sendo core."

**Ideal early adopter:**

- Equipes de plataforma que querem **construir** seu próprio developer portal/infra orchestrator em vez de adotar um pronto
- Organizações que **não querem** lock-in de Kubernetes-native (Crossplane) nem de SaaS (Port)
- Ecossistemas com **muitas dimensões** além de deploy (data, events, compliance, AI governance) que precisam de um primitive para integrar tudo
- Times multilingual (Go + Python + Rust + TypeScript) onde SDK-first products criam fricção

## 5. Oportunidades de Integrations (não-gaps, mas valiosos)

Estas são **integrations que fariam sentido existir** mas não são core gaps. Cada uma é uma oportunidade de contribuição ao ecossistema Yggdrasil sem mexer no core:

| Integration | Propósito | Prioridade |
|---|---|---|
| **integration-webhook-receiver** | Recebe HTTP webhooks de sistemas externos (GitHub, Stripe, Linear, etc.), publica como events no event stream do core | 🔴 Alta (habilita workflows event-driven com fontes externas) |
| **integration-event-audit-exporter** | Consome event stream do core, exporta para SIEM (Splunk, Datadog, CloudWatch Logs) | 🔴 Alta (compliance) |
| **integration-slack-notifier** | Consome event stream, posta em Slack channels quando eventos matchem filtros | 🟡 Média |
| **integration-terraform** | Wraps `terraform` CLI; operations `plan`, `apply`, `destroy`, `state-list`, etc. | 🟡 Média (posicionamento competitivo com Terraform Cloud) |
| **integration-kubernetes-pull** | Agent dentro do cluster que puxa Products do core e aplica localmente (pull-based mode) | 🟡 Média (posicionamento competitivo com ArgoCD) |
| **integration-pulumi** | Wraps Pulumi; operations para stacks Pulumi | 🟢 Baixa |
| **integration-backstage-bridge** | Surface existing Backstage entities como Yggdrasil resources (para migração) | 🟢 Baixa |
| **integration-kafka-consumer** | Consome tópicos Kafka, publica como events no stream do core | 🟢 Baixa |
| **integration-prometheus-metrics** | Expõe Prometheus metrics baseado no event stream | 🟢 Baixa |
| **integration-cost-tracker** | Subscribe em `product.applied` / `integration.execute` events, correlaciona com billing APIs dos clouds, registra custos por product | 🟢 Baixa |
| **integration-compliance-reporter** | Subscribe em todos events, gera relatórios SOC2/HIPAA/PCI-DSS formatados | 🟢 Baixa |

## 6. Oportunidades de Surfaces (não-gaps, mas valiosos)

Estas são **surfaces que fariam sentido existir** acima do core atual:

| Surface | Propósito | Prioridade |
|---|---|---|
| **OpenAPI/Swagger export no yggdrasil-api** | API docs machine-readable; permite code-gen de clients | 🔴 Alta (developer experience) |
| **CLI para products e workflows** (`ygg products apply`, `ygg workflows run`) | CLI atual só cobre integrations/surfaces management; falta apply/run direto | 🔴 Alta |
| **VSCode extension** | Syntax highlighting para manifests JSON/YAML, validation inline contra schemas | 🟡 Média |
| **GraphQL API surface** | Alternative ao yggdrasil-api REST, para clientes que preferem GraphQL | 🟢 Baixa |
| **gRPC API surface** | Alternative ao REST, para clientes de alta performance | 🟢 Baixa |
| **Mobile-responsive console** | yggdrasil-console hoje é desktop-only | 🟢 Baixa |
| **Dark mode no console** | Só light theme hoje | 🟢 Baixa |
| **Manifest validation CLI** (`ygg validate manifest.json`) | Validar manifests localmente antes de submeter ao core | 🟡 Média |

## 7. Roadmap sugerido

Ordem de execução recomendada. Core gaps primeiro (porque destravam o resto). Depois integrations prioritárias. Depois surfaces.

### Fase 1: Core foundation (prioridade máxima)

1. **Event stream** (Gap 1) — spec já escrita neste commit ([2026-04-10-event-stream-design.md](./2026-04-10-event-stream-design.md))
2. **Pagination** (Gap 2) — spec no batch 2
3. **Workflow → product target overrides** (Gap 3) — spec no batch 2

### Fase 2: Core lifecycle (depende da Fase 1)

4. **BuildProject lifecycle enforcement** (Gap 4) — depende do event stream, spec no batch 2
5. **Workflow trigger system** (Gap 5) — depende do event stream, spec no batch 2

### Fase 3: High-impact integrations (habilitam casos de uso reais)

6. **integration-webhook-receiver** — habilita workflows event-driven
7. **integration-event-audit-exporter** — desbloqueia compliance stories

### Fase 4: Developer experience

8. **OpenAPI export no yggdrasil-api** — desbloqueia SDK generation e integrações externas
9. **CLI completo** (`ygg products`, `ygg workflows`) — desbloqueia automação CI/CD

### Fase 5 em diante: ecosistema e polish

10+. **Integrations e surfaces adicionais** baseadas em demanda real

## 8. Riscos identificados

| Risco | Impacto | Mitigação |
|---|---|---|
| Event stream escala mal com muitos events por segundo | Alto | Spec do event stream (Gap 1) prevê retention configurável e cursor-based pull — consumers slow não bloqueiam producers. PostgreSQL como backend é tested a milhões de rows |
| BuildProject cleanup tem race condition com apply em progresso | Médio | Spec do Gap 4 prevê check de "in-flight operations" antes de teardown |
| Feature creep no core (cada usuário quer adicionar sua feature específica) | Alto | Filosofia explícita documentada na seção 1 deste audit serve como guardrail |
| Plugins em outras linguagens têm bugs sutis por má implementação de JSON schemas | Médio | Contracts rigorosamente versionados + test harness contract-level que plugin authors podem usar em CI |
| Target overrides quebram a imutabilidade dos products em casos edge | Baixo | Spec do Gap 3 limitará overrides a `target.integration_instance_ref` e `target.namespace`; source e renderer permanecem imutáveis |

## 9. Pontos em aberto para discussão futura

- **Global total ordering do event stream**: se precisar de cross-aggregate ordering, adicionar sequence monotônica global. Default deixa ordering per-aggregate.
- **Event replay de histórico longo**: consumers podem querer rebuildar projeções a partir do início. Retention configurável + backfill mechanism. Decidir em implementation time.
- **Integration vs Surface para Prometheus metrics**: depende de quem consome. Se é scraped pelo Prometheus server externo, é **surface** (HTTP endpoint). Se é pushed para Pushgateway, é **integration**.
- **Multi-tenancy real vs namespace isolation**: hoje manifests são namespaced mas tudo compartilha a mesma DB. Multi-tenant verdadeiro (per-tenant schema, row-level security) é uma feature grande que pode ou não ser priorizada.

## 10. Conclusão

Yggdrasil é **arquitetonicamente sólido** e **filosoficamente coerente**. A maioria das "limitações" que aparecem à primeira vista são **escolhas intencionais** alinhadas com a visão de "quebra-cabeça extensível com core minimalista".

Os gaps reais são cinco, bem delimitados, e todos se justificam como "core deve gerenciar estado com maestria":

1. **Event stream** (fundacional) — core emite mutações, outros consomem
2. **Pagination** — escalabilidade de state queries
3. **Workflow → product target overrides** — composição entre primitives existentes
4. **BuildProject lifecycle enforcement** — ephemerais não podem vazar
5. **Workflow trigger system** — core decide quando disparar

Uma vez fechados esses 5 gaps, Yggdrasil tem **fundação sólida** para ser apresentado como produto competitivo. O que sobra são oportunidades de ecossistema (novas integrations, novas surfaces) que podem crescer organicamente conforme adopters trazem suas necessidades.

O posicionamento competitivo correto é: **"composabilidade como diferencial principal, não feature completeness"**. Quem quer "tudo pronto" escolhe Backstage. Quem quer "tudo montável" escolhe Yggdrasil.

---

## Referências

- Spec dedicada do Gap 1 (Event Stream): [`2026-04-10-event-stream-design.md`](./2026-04-10-event-stream-design.md)
- Specs dedicadas dos Gaps 2-5: serão escritas no batch 2 deste trabalho
- Filosofia do Yggdrasil (memory): `yggdrasil_philosophy.md`
- Survey holístico que precedeu este audit: conduzido em 2026-04-10 (não publicado como doc standalone; o conteúdo foi absorvido neste report)
