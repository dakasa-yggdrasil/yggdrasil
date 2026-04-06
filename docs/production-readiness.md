# Production Readiness

Este documento resume o baseline operacional do produto `yggdrasil`.

## Baseline atual

- `yggdrasil-core` expõe `GET /healthz` e `GET /readyz`
- `yggdrasil-auth-surface` expõe `GET /healthz` e `GET /readyz`
- `yggdrasil-console` possui build de produção e imagem dedicada
- o CI do produto valida:
  - fronteiras arquiteturais
  - compose do workspace
  - bootstrap do core
  - smoke end-to-end
  - build das imagens de produção

## O que uma operação deve usar

- readiness do core: `GET /readyz`
- liveness do core: `GET /healthz`
- readiness da auth surface: `GET /readyz`
- liveness da auth surface: `GET /healthz`
- liveness do console: `GET /healthz`

## Imagens de produção

As imagens locais de referência podem ser validadas com:

```bash
task build:images
```

Isso gera:

- `yggdrasil-core:local`
- `yggdrasil-auth-surface:local`
- `yggdrasil-console:local`

## Variáveis sensíveis

- auth providers devem preferir `client_secret` inline só no momento do cadastro
- o core materializa isso em `managed_secret` e persiste apenas `client_secret_ref`
- `integration_instance` segue a mesma disciplina para `credentials_ref`
- o fallback LLM do Heimdall segue a mesma disciplina; veja [`docs/heimdall-llm-activation.md`](/Users/dakasa/projects/yggdrasil/docs/heimdall-llm-activation.md)

## Regras para deploy

- surfaces falam com o core por HTTP síncrono
- integrations continuam pertencendo ao core
- plugins não devem receber tráfego de usuário diretamente
- segredos devem ser referenciados por `secret://...` sempre que possível

## Checklist mínimo antes de release

1. `task arch:check`
2. `task config`
3. `task smoke`
4. `task build:images`
5. revisar `catalog/surfaces.active` e integrations instaladas para o ambiente alvo
