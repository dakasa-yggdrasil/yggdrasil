# Workflow → Product.apply com Target Overrides — yggdrasil-core

**Data:** 2026-04-10
**Status:** Spec em revisão
**Tipo:** Core composition primitive
**Prioridade:** 🔴 Alta — habilita multi-environment sem duplicar Products
**Escopo:** Estender workflows para dispatch de product operations (materialize, installation.apply, etc.) com target overrides, preservando a imutabilidade dos Products.
**Depende de:** nada (independent)
**Habilita:** multi-environment deployment com um único Product manifest, ephemeral environments dinâmicos, CD pipelines parametrizados
**Parte do audit report:** [`2026-04-10-yggdrasil-product-audit-report.md`](./2026-04-10-yggdrasil-product-audit-report.md) Gap 3

---

## 1. Contexto e motivação

### 1.1 O problema

Products no Yggdrasil são **artefatos compilados imutáveis** — decisão filosófica consciente. Products têm `target.integration_instance_ref` hardcoded no manifest, apontando para um cluster específico (ex: `kubernetes-dakasa-prod`).

Isso significa que **um único Product manifest só pode ser deployed em um target**. Para deploy em múltiplos environments (validation, staging, prod, ephemeral PRs), o modelo atual força uma de duas abordagens:

1. **Múltiplos Products por environment** — `dakasa-platform-validation`, `dakasa-platform-staging`, `dakasa-platform-prod`, etc. Copiados, mantidos em paralelo. Violação do princípio DRY e risco de drift entre envs.
2. **Aliases de integration_instance por convenção** — usar `kubernetes-default` em todos os Products e provisionar um `kubernetes-default` apontando para o cluster certo em cada env. Funciona mas **vaza complexidade para o usuário**: se precisa de alias, o produto está mal desenhado (feedback já aceito no [audit report §1](./2026-04-10-yggdrasil-product-audit-report.md)).

### 1.2 A solução filosófica

Products são imutáveis porque são **artefatos versionados e auditáveis**. Workflows são **a camada dinâmica** que parametriza execução em runtime.

Portanto: **a parametrização de target deve acontecer no workflow, não no product**. Quando um workflow dispara `product.installation.apply`, ele pode passar um **target_overrides map** que o core aplica sobre os targets do Product imutável.

```
Product (imutável):
  component "identities":
    target:
      integration_instance_ref: { name: "kubernetes-cluster" }
      namespace: "dakasa"

Workflow (dynamic) dispara:
  product.installation.apply(
    product_ref: "dakasa-app",
    target_overrides: {
      "kubernetes-cluster": { name: "kubernetes-dakasa-validation" },
      // ou em prod:
      // "kubernetes-cluster": { name: "kubernetes-dakasa-prod" }
    }
  )

Core resolve:
  Para cada component do Product:
    Se target.integration_instance_ref.name está no override map:
      Substitui pelo valor do override (só integration_instance_ref e opcionalmente namespace)
    Aplica o component com o target resolvido
```

O Product manifest nunca muda. A mesma checksum do Product é aplicada em múltiplos envs. **Auditability preservada, flexibility ganha no workflow layer.**

### 1.3 Por que é core

- **Não pode ser integration**: integrations executam operations externas, não orquestram products internos. Ninguém fora do core tem autoridade sobre `product.installation.apply`.
- **Não pode ser surface**: surfaces podem construir essa lógica acima do core, mas isso significaria cada surface reimplementando o override mechanism. Fragmentação.
- **Tem que ser core**: é composição entre dois primitives do core (workflow + product). Core é quem aplica products; core é quem executa workflows. Adicionar a capacidade de workflows override targets é uma feature intrínseca da composição.

### 1.4 Casos de uso habilitados

- **Multi-environment com único manifest**: `dakasa-app` único, dispatched via workflow para validation, staging, prod, conforme input
- **Ephemeral PR environments**: workflow recebe `pr_number` e aplica `dakasa-app` em um cluster ephemeral (ou namespace isolado)
- **Disaster recovery drills**: workflow dispara `dakasa-app` em um cluster de DR temporário, sem alterar manifests de produção
- **Blue/green deploys**: workflow aplica em blue, valida, aplica em green, via overrides de target_integration_instance
- **Multi-region rollout**: mesmo Product, múltiplos targets geograficamente distintos

## 2. Design

### 2.1 Princípios

1. **Imutabilidade do Product preservada**: overrides não modificam o Product manifest. São aplicados em runtime, durante `installation.apply`. O Product manifest + checksum continuam sendo a fonte de verdade.
2. **Overrides são limitados a `target`**: você pode trocar `target.integration_instance_ref` e `target.namespace`. **Não** pode mexer em source, renderer, reconcile, requires, depends_on — esses são parte do artefato imutável.
3. **Overrides são explícitos via match**: o caller especifica "quando você encontrar este `integration_instance_ref.name`, substitua por aquele". Override key é o nome que já está no Product; value é o replacement.
4. **Default: sem override**: se o workflow não passar `target_overrides`, o Product é aplicado com seus targets originais. Feature é aditiva, non-breaking.
5. **Auditability rastreada**: a aplicação com overrides emite um event `product.installation.applied` no [event stream](./2026-04-10-event-stream-design.md) contendo os overrides usados. Audit log completo preserva qual Product foi aplicado em qual target via qual workflow.

### 2.2 Schema do override

```json
{
  "product_ref": {
    "name": "dakasa-app",
    "namespace": "dakasa"
  },
  "target_overrides": {
    "kubernetes-cluster": {
      "integration_instance_ref": {
        "name": "kubernetes-dakasa-validation",
        "namespace": "dakasa"
      },
      "namespace": "dakasa-validation-pr-42"
    },
    "rabbitmq-management": {
      "integration_instance_ref": {
        "name": "rabbitmq-dakasa-validation",
        "namespace": "dakasa"
      }
    }
  }
}
```

**Explicação:**
- `target_overrides` é um map onde **chave** = `target.integration_instance_ref.name` que está no Product, **valor** = replacement (novo `integration_instance_ref` + optional `namespace`)
- Se o Product tem um component com `target.integration_instance_ref.name == "kubernetes-cluster"`, esse component será applied usando `kubernetes-dakasa-validation` com namespace `dakasa-validation-pr-42`
- Outros components do Product (cujo target name não está no override map) usam seus targets originais

### 2.3 Matching rules

O override match é feito por **`integration_instance_ref.name`** (ignoring namespace). Razões:

- **Simplicidade**: chave única, fácil de raciocinar
- **Convenção Yggdrasil**: products tipicamente usam nomes como `kubernetes-cluster`, `grafana-api`, `rabbitmq-management` — convenções que funcionam bem como keys
- **Namespace é parte do override, não do match**: quando você override, pode trocar o namespace também

**Caso edge**: se o Product tem dois components apontando para o mesmo `integration_instance_ref.name` mas com namespaces diferentes, **ambos** são overridden quando a key casa. Não há match granular por `(name, namespace)`. Se o Product precisar disso, ele deve usar nomes distintos.

### 2.4 Validação de overrides

Core valida overrides antes de aplicar:

1. **Override key deve existir no Product**: se `target_overrides` tem uma key que nenhum component do Product usa, core retorna erro `override_key_not_found`. (Evita typos silenciosos.)
2. **Override value deve apontar para integration_instance existente**: core valida que a `integration_instance_ref` do override existe e está `status: active`.
3. **Override value não pode remover o target**: não dá para "nullar" o target; se você override, precisa fornecer `integration_instance_ref` válido.
4. **Overrides não podem mexer em `kind`**: `target.kind` permanece `kubernetes` (ou o que o Product declarou). Não dá para trocar de kubernetes para outro kind via override.

## 3. Integração com Workflows

### 3.1 Novo workflow step kind: `product`

Hoje, workflows só suportam `use.kind: "integration"`. Este spec adiciona `use.kind: "product"`, que permite steps dispatchar product operations.

**Exemplo de workflow step:**

```json
{
  "id": "apply-dakasa-to-validation",
  "use": {
    "kind": "product",
    "operation": "installation.apply"
  },
  "with": {
    "product_ref": {
      "name": "dakasa-app",
      "namespace": "dakasa"
    },
    "target_overrides": {
      "kubernetes-cluster": {
        "integration_instance_ref": {
          "name": "kubernetes-dakasa-{{ inputs.environment }}",
          "namespace": "dakasa"
        },
        "namespace": "dakasa-{{ inputs.environment }}"
      }
    },
    "wait_for_ready": true
  },
  "depends_on": ["provision-infra"],
  "timeout_seconds": 600
}
```

**Campos:**

- `use.kind: "product"` — novo kind
- `use.operation` — uma de: `materialize`, `installation.reconcile`, `installation.apply`, `installation.observe`, `installation_state.discover`
- `with.product_ref` — selector para o Product manifest
- `with.target_overrides` — map opcional de overrides (formato §2.2)
- `with.wait_for_ready` (opcional, default false) — para `installation.apply`, se true, core espera até observe report "ready" antes de step completar
- Templating `{{ inputs.environment }}` funciona normalmente no `with` (já suportado pelo workflow engine)

### 3.2 Operations suportadas

| Operation | O que faz | Supports target_overrides |
|---|---|---|
| `materialize` | Resolve integration sources → inline objects, cria ProductMaterialization record | ❌ Não — materialization é sobre resolver sources, não targets |
| `installation.reconcile` | Valida requirements + constrói plan | ✅ Sim |
| `installation.apply` | Aplica objects nos targets resolvidos | ✅ Sim |
| `installation.observe` | Observa state dos objects aplicados | ✅ Sim |
| `installation_state.discover` | Descobre state atual sem aplicar | ✅ Sim |

### 3.3 Execução

Quando core executa um workflow step com `use.kind: "product"`:

```go
// Pseudo-code
func executeProductStep(ctx, step) error {
    // 1. Parse with.product_ref
    productRef := parseProductRef(step.With["product_ref"])

    // 2. Parse with.target_overrides (optional)
    overrides := parseTargetOverrides(step.With["target_overrides"])

    // 3. Validate overrides against Product
    product, err := loadProduct(productRef)
    if err := validateOverrides(product, overrides); err != nil {
        return err
    }

    // 4. Dispatch to the appropriate product operation
    switch step.Use.Operation {
    case "installation.apply":
        return dispatchProductInstallationApply(ctx, ApplyProductInstallationRequest{
            ProductRef:      productRef,
            TargetOverrides: overrides,
            WaitForReady:    step.With["wait_for_ready"].(bool),
        })
    case "installation.reconcile":
        // ...
    case "materialize":
        // materialize doesn't use overrides
        return dispatchProductMaterialize(ctx, ...)
    // ...
    }
}
```

### 3.4 Override resolution dentro do product apply executor

O override resolution acontece no **product target executor** existente, estendido para aceitar o map:

```go
// In products.go handler (simplified)
func applyProductInstallation(ctx, req ApplyProductInstallationRequest) error {
    product, err := loadProduct(req.ProductRef)

    // Validate overrides against product components
    if len(req.TargetOverrides) > 0 {
        if err := validateOverrides(product, req.TargetOverrides); err != nil {
            return err
        }
    }

    for _, component := range product.Spec.Components {
        resolvedTarget := component.Target
        
        // Apply override if key matches
        if override, exists := req.TargetOverrides[component.Target.IntegrationInstanceRef.Name]; exists {
            resolvedTarget.IntegrationInstanceRef = override.IntegrationInstanceRef
            if override.Namespace != "" {
                resolvedTarget.Namespace = override.Namespace
            }
        }

        // Execute the component with the resolved target
        if err := applyComponent(ctx, component, resolvedTarget); err != nil {
            return err
        }
    }

    // Emit event with overrides captured in metadata
    events.Emit(tx, EmitEventRequest{
        Type: "product.installation.applied",
        Payload: map[string]any{
            "product_ref": req.ProductRef,
            "target_overrides_used": req.TargetOverrides,
            // ...
        },
    })

    return nil
}
```

## 4. Extensão do RPC schema

### 4.1 ApplyProductInstallationRequest

Schema atual (inferido do audit):

```json
{
  "product_ref": {
    "name": "string",
    "namespace": "string",
    "id": "uuid (optional)"
  }
}
```

Schema novo (aditivo):

```json
{
  "product_ref": {
    "name": "string",
    "namespace": "string",
    "id": "uuid (optional)"
  },
  "target_overrides": {
    "<integration_instance_ref_name>": {
      "integration_instance_ref": {
        "name": "string",
        "namespace": "string"
      },
      "namespace": "string (optional)"
    }
  }
}
```

**Backwards compat:** `target_overrides` é opcional. Callers antigos não passam e continuam funcionando. Se ausente, nenhum override é aplicado.

### 4.2 JSON Schema (em `docs/contracts/rpc/product/v1/`)

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "ApplyProductInstallationRequest",
  "type": "object",
  "required": ["product_ref"],
  "properties": {
    "product_ref": {
      "$ref": "../common/manifest_selector.schema.json"
    },
    "target_overrides": {
      "type": "object",
      "description": "Map of target integration_instance_ref.name → override. When a component's target matches a key, the target is replaced at apply time. Does not modify the Product manifest.",
      "additionalProperties": {
        "type": "object",
        "required": ["integration_instance_ref"],
        "properties": {
          "integration_instance_ref": {
            "type": "object",
            "required": ["name"],
            "properties": {
              "name": { "type": "string" },
              "namespace": { "type": "string" }
            }
          },
          "namespace": {
            "type": "string",
            "description": "Optional namespace override. If omitted, the component's original namespace is used."
          }
        }
      }
    },
    "wait_for_ready": {
      "type": "boolean",
      "default": false
    }
  }
}
```

## 5. Extensão do Workflow step schema

### 5.1 Schema atual (inferido)

```json
{
  "id": "string",
  "use": {
    "kind": "integration",
    "instance_ref": { ... },
    "operation": "string",
    "capability": "string"
  },
  "with": { ... },
  "depends_on": [ "..." ],
  "retry": { ... },
  "timeout_seconds": 0
}
```

### 5.2 Schema novo (aditivo)

```json
{
  "id": "string",
  "use": {
    "kind": "integration | product",
    // If kind == "integration":
    "instance_ref": { ... },
    "operation": "string",
    "capability": "string",
    // If kind == "product":
    "operation": "materialize | installation.reconcile | installation.apply | installation.observe | installation_state.discover"
  },
  "with": { ... },
  "depends_on": [ "..." ],
  "retry": { ... },
  "timeout_seconds": 0
}
```

**Mudanças:**
- `use.kind` passa a aceitar `"product"` além de `"integration"`
- Para `kind: "product"`, `use.operation` é required; `instance_ref` não é usado
- `with` segue sendo free-form map, mas para `kind: "product"` espera campos `product_ref`, `target_overrides` (opcional), `wait_for_ready` (opcional, só para `installation.apply`)

### 5.3 Validation ao parsing do workflow

Core valida:

1. `use.kind` deve ser `"integration"` ou `"product"`
2. Para `kind: "product"`:
   - `use.operation` deve ser uma das 5 operations listadas
   - `with.product_ref` deve estar presente e ser ManifestSelector válido
   - Se `with.target_overrides` presente, deve match schema §4.2
3. Templating `{{ }}` continua funcionando em qualquer campo de `with` (resolved at runtime)

## 6. Events emitidos

Quando um workflow step dispara product operation (via this feature), core emite events no [event stream](./2026-04-10-event-stream-design.md):

- `workflow.step.started` — step começou
- `product.installation.applied` — aplicação completada (reuso do event type existente, adicionando `target_overrides_used` no payload se overrides foram usados)
- `workflow.step.completed` — step terminou com sucesso

Ou em caso de erro:
- `product.installation.apply_failed`
- `workflow.step.failed`

O payload de `product.installation.applied` ganha um campo opcional `target_overrides_used` para audit trail:

```json
{
  "product_ref": { ... },
  "components_applied": [ ... ],
  "target_overrides_used": {
    "kubernetes-cluster": { ... }
  },
  "dispatched_by": "workflow:deploy-dakasa/run-xyz-123"
}
```

Audit log pode reconstruir quem aplicou qual Product em qual target via qual workflow run.

## 7. Implementação

### 7.1 Arquivos afetados

- `manifest/workflow.go` — validar `use.kind: "product"` e seus required fields
- `model/workflow.go` — extender `WorkflowStepUse` para suportar `kind: "product"`
- `controllers/message/workflows.go` — no step executor, detectar `use.kind: "product"` e dispatchar para product operations
- `controllers/message/products.go` — estender `applyProductInstallation` (e reconcile/observe/discover) para aceitar `target_overrides` parameter
- `docs/contracts/rpc/product/v1/` — atualizar schemas de request
- `docs/contracts/events/v1/product/` — atualizar schema de `product.installation.applied` para incluir `target_overrides_used`
- `docs/contracts/workflows/v1/` — documentar o novo `use.kind: "product"`

### 7.2 Fases de implementação

#### Fase 1 — Core plumbing

- [ ] Estender `ApplyProductInstallationRequest` com `target_overrides` field
- [ ] Implementar `validateOverrides(product, overrides)` (§2.4)
- [ ] Implementar override resolution no product apply executor
- [ ] Emitir event `product.installation.applied` com `target_overrides_used` quando aplicável
- [ ] Unit tests cobrindo override resolution, validation errors, component matching

#### Fase 2 — Workflow integration

- [ ] Estender `WorkflowStepUse` para aceitar `kind: "product"`
- [ ] Validator de workflow manifest aceita novo kind + valida required fields
- [ ] Step executor dispatcha para product operations (reuse do ApplyProductInstallationRequest estendido)
- [ ] Integration tests cobrindo: workflow com step `kind: "product"` → product.installation.apply → overrides resolvidos → components aplicados nos targets overridden

#### Fase 3 — RPC para as outras product operations

- [ ] `reconcile` aceita `target_overrides`
- [ ] `observe` aceita `target_overrides` (para observar state no target overridden)
- [ ] `installation_state.discover` aceita `target_overrides`
- [ ] `materialize` **não** aceita overrides (não faz sentido, materialization resolve sources)

#### Fase 4 — Documentação e examples

- [ ] Atualizar JSON Schemas em `docs/contracts/`
- [ ] Exemplo completo de workflow multi-env em `docs/bootstrap/manifests/workflows/`
- [ ] Documentar pattern "one product, many environments" em README do core

## 8. Casos de uso concretos

### 8.1 Multi-environment deploy do DaKasa

```json
{
  "kind": "workflow",
  "metadata": {
    "name": "deploy-dakasa",
    "namespace": "dakasa"
  },
  "spec": {
    "trigger": { "mode": "manual" },
    "input_schema": {
      "required": ["environment"],
      "properties": {
        "environment": {
          "type": "string",
          "enum": ["validation", "staging", "prod"]
        }
      }
    },
    "steps": [
      {
        "id": "apply-infra",
        "use": { "kind": "product", "operation": "installation.apply" },
        "with": {
          "product_ref": { "name": "dakasa-infra", "namespace": "dakasa" },
          "target_overrides": {
            "kubernetes-cluster": {
              "integration_instance_ref": {
                "name": "kubernetes-dakasa-{{ inputs.environment }}",
                "namespace": "dakasa"
              },
              "namespace": "infra"
            }
          },
          "wait_for_ready": true
        }
      },
      {
        "id": "apply-app",
        "depends_on": ["apply-infra"],
        "use": { "kind": "product", "operation": "installation.apply" },
        "with": {
          "product_ref": { "name": "dakasa-app", "namespace": "dakasa" },
          "target_overrides": {
            "kubernetes-cluster": {
              "integration_instance_ref": {
                "name": "kubernetes-dakasa-{{ inputs.environment }}",
                "namespace": "dakasa"
              },
              "namespace": "dakasa-main"
            }
          },
          "wait_for_ready": true
        }
      }
    ]
  }
}
```

Dispatch: `workflow.dispatch(name: "deploy-dakasa", inputs: { environment: "validation" })`

Resultado: dakasa-infra e dakasa-app são aplicados **no cluster validation**. Mesmos Products, target dinâmico.

### 8.2 Ephemeral PR environment

```json
{
  "kind": "workflow",
  "metadata": { "name": "spin-up-pr-environment" },
  "spec": {
    "input_schema": {
      "required": ["pr_number"],
      "properties": { "pr_number": { "type": "integer" } }
    },
    "steps": [
      {
        "id": "create-buildproject",
        "use": { "kind": "integration", "instance_ref": { "name": "yggdrasil-core" }, "operation": "topology.build_project.create" },
        "with": {
          "name": "dakasa-pr-{{ inputs.pr_number }}",
          "ephemeral": true,
          "expires_at": "48h-from-now"
        }
      },
      {
        "id": "apply-to-pr-namespace",
        "depends_on": ["create-buildproject"],
        "use": { "kind": "product", "operation": "installation.apply" },
        "with": {
          "product_ref": { "name": "dakasa-app", "namespace": "dakasa" },
          "target_overrides": {
            "kubernetes-cluster": {
              "integration_instance_ref": {
                "name": "kubernetes-dakasa-nonprod",
                "namespace": "dakasa"
              },
              "namespace": "dakasa-pr-{{ inputs.pr_number }}"
            }
          }
        }
      }
    ]
  }
}
```

Cada PR tem seu namespace isolado no cluster nonprod. Mesmo Product, deploys isolados.

### 8.3 Disaster recovery drill

```json
{
  "id": "dr-apply-to-recovery-cluster",
  "use": { "kind": "product", "operation": "installation.apply" },
  "with": {
    "product_ref": { "name": "dakasa-platform", "namespace": "dakasa" },
    "target_overrides": {
      "kubernetes-cluster": {
        "integration_instance_ref": {
          "name": "kubernetes-dr-recovery",
          "namespace": "dakasa"
        }
      }
    },
    "wait_for_ready": true
  }
}
```

Dispatched ad-hoc quando precisa testar DR. Product de produção é aplicado em cluster de DR sem alterar nada.

## 9. Alternativas consideradas

### 9.1 Templating nos Products (via Helm values style)

Adicionar campo `parameters` ao Product manifest que permite substituições em runtime.

**Rejeitado porque:**
- Quebra a filosofia "Products são imutáveis compilados"
- Acopla Yggdrasil a um modelo de templating (Helm-style, GoTemplate, Jinja, etc.)
- Cria o risco de Products não-reproducíveis ("roda aqui mas não lá" dependendo dos params)
- Workflows já têm templating para esse propósito

### 9.2 Integration type "dakasa-renderer"

Criar uma integration custom que recebe input (env) e gera Products dinamicamente.

**Rejeitado porque:**
- Adiciona indirection desnecessária
- Products dinâmicos (integration-generated) já existem via `source.kind: "integration"`, mas não para overriding targets
- Integrations são pra gerar recursos externos, não para massajar manifests internos do core

### 9.3 Fork de Products por environment

Aceitar "múltiplos Products por env" como o modelo oficial.

**Rejeitado porque:**
- Viola DRY em alta velocidade de mudança
- Drift entre Products quando um env recebe fix e outros esquecem
- Explosão de manifests para cada env × ephemeral combo
- Complexidade de gestão (auditar qual env tem qual versão)

### 9.4 Target aliases no nível de integration_instance

Criar um recurso novo "target_alias" que resolve `kubernetes-cluster` → `kubernetes-dakasa-validation` por environment context.

**Rejeitado porque:**
- Introduz um conceito novo (alias) que complica o mental model
- Requer um "env context" implícito por RPC call (stateful)
- `target_overrides` no workflow step é mais explícito e rastreável

### 9.5 Estender Products com `target_placeholders`

Marcar targets como placeholders explícitos no Product manifest; workflows obrigatoriamente fornecem valores.

**Rejeitado porque:**
- Força todos os Products a declarar placeholders antecipadamente
- Products que não precisam de overrides ganham boilerplate inútil
- `target_overrides` opcional é mais flexível

## 10. Riscos e mitigações

| Risco | Impacto | Mitigação |
|---|---|---|
| Usuário faz override errado (troca o cluster errado por acidente) | Alto | Validação de override key (§2.4) previne typos; events emitidos registram override usado para audit posterior; dry-run opcional (futuro) |
| Override permite bypass de policy ("aplique em cluster de prod mesmo que minha role não permita") | Alto | Authorization no nível do workflow dispatch: RBAC checa `apply` action no Product + no target integration_instance. Override não escapa de authorization — override é verificado como se o target original tivesse mudado |
| Múltiplos components com mesmo `integration_instance_ref.name` são overriden juntos mesmo sem intenção | Médio | Documentar claramente; convenção: Products usam nomes distintos quando devem ser overridden separately |
| Ambiguidade: "qual cluster está rodando `dakasa-app`?" vira "depende do workflow que fez apply" | Médio | Event `product.installation.applied` registra target usado; audit trail completo via event stream |
| Override cria drift silencioso entre envs (mesmo Product, diferente target, mas config diferente via ConfigMap do target) | Baixo | Responsabilidade do autor do workflow; não é problema do core |

## 11. Compatibilidade com yggdrasil-core atual

### 11.1 O que muda

- `ApplyProductInstallationRequest` ganha field opcional `target_overrides` (backwards compat)
- `WorkflowStepUse` aceita `kind: "product"` adicional (backwards compat; workflows existentes só usam `kind: "integration"`)
- Product apply executor tem lógica de override resolution (só ativa quando overrides passados)
- Event `product.installation.applied` ganha field opcional `target_overrides_used` (backwards compat)

### 11.2 O que NÃO muda

- Product manifest schema intacto
- Componentes do Product continuam sendo tratados como imutáveis pelo core
- Workflows existentes com `kind: "integration"` continuam funcionando
- RPCs existentes sem `target_overrides` continuam funcionando (sem overrides)

## 12. Critérios de aceitação

- ✅ `target_overrides` field adicionado a `ApplyProductInstallationRequest`
- ✅ Validação de overrides (key exists, target exists, active) implementada
- ✅ Override resolution no product apply executor funcional
- ✅ `use.kind: "product"` funcional no workflow step schema
- ✅ Workflow step executor dispatcha para product operations
- ✅ Events `product.installation.applied` incluem `target_overrides_used` quando aplicável
- ✅ Backwards compatible: callers antigos funcionam sem mudanças
- ✅ JSON Schemas atualizados em `docs/contracts/`
- ✅ Exemplo de workflow multi-env em `docs/bootstrap/manifests/workflows/`
- ✅ Integration tests cobrindo: workflow → product step → apply com overrides → targets corretos

## 13. Dependências

- **Depende de:** nada. Feature independente.
- **Opcionalmente usa:** event stream (§6) para emitir events de audit. Se event stream não estiver implementado ainda, events podem ser adiados sem quebrar a feature.
- **Habilita:** deploy multi-environment sem duplicar Products, ephemeral PR environments, DR drills, blue/green deploys, multi-region rollouts

## 14. Pontos em aberto

- **Namespace override semantics**: se `target_overrides[...].namespace` é omitido, usamos `component.Target.Namespace` original. Alternativa seria erro ("ambiguous — either override namespace or don't"). **Decisão MVP:** default para original é mais conveniente; admins podem explicitar se quiserem.
- **Dry-run de overrides**: antes de aplicar, poder visualizar "this is what would be applied, with these target replacements". Feature futura, não MVP.
- **Overrides parciais por component**: hoje, override é por `integration_instance_ref.name`, afetando todos os components que casam. Feature futura poderia permitir override por `component.name`. Não priorizado porque convenção de nomes distintos resolve 90% dos casos.
- **Templating dentro de overrides**: `{{ inputs.environment }}` funciona? Sim, via workflow's existing templating engine. Testado em §8.1.

## 15. Resumo executivo

- **Feature:** workflows podem dispatch product operations (`installation.apply`, etc.) com `target_overrides` que o core aplica sobre os targets imutáveis do Product.
- **Preserva filosofia:** Products continuam imutáveis; dinamicidade é no workflow layer.
- **Casos habilitados:** multi-env deploy com único manifest, ephemeral PR envs, DR drills, multi-region rollouts.
- **Mudanças aditivas:** novos fields opcionais em RPC + novo `kind: "product"` no workflow step. Backwards compatible.
- **Validação rigorosa:** overrides são checados contra components existentes; errors explícitos evitam typos.
- **Audit trail:** event `product.installation.applied` registra `target_overrides_used`.
- **Implementação:** 4 fases incrementais; core plumbing → workflow integration → outras product ops → docs.
