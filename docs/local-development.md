# Local Development

Este workspace foi montado para permitir dois modos de trabalho:

- `full stack`, quando queremos subir o ecossistema inteiro
- `slice`, quando queremos trabalhar só em um serviço ou plugin, reaproveitando a mesma infra

## Pré-requisitos

- Docker Desktop com `docker compose`
- [`task`](https://taskfile.dev)
- Go local é opcional. O installation manager cai para Docker automaticamente quando `go` não estiver instalado.

## Bootstrap inicial

No root do workspace:

```bash
task doctor
task arch:check
task surfaces:list
task surfaces:active
task surfaces:install NAME=yggdrasil-auth-surface
task surfaces:install NAME=yggdrasil-console
task integrations:list
task env:init
task up
task bootstrap:core
task open:console
task build:images
task install:smoke
```

Se quiser instalar integrations de forma interativa:

```bash
task integrations:tui
```

## Estrutura de ambiente

- `.env` no root: portas, imagens base, broker e banco compartilhados
- `services/*/.env`: variáveis específicas dos serviços core
- `surfaces/*/.env`: variáveis específicas das surfaces
- `integrations/*/.env`: variáveis específicas de cada plugin
- o console de referência usa o `yggdrasil-core` HTTP em `http://localhost:9080` por padrão no host
- as surfaces de referência são repositórios instaláveis, não código obrigatoriamente vendorizado no produto
- cada integration usa `docker-compose.yml` como base compatível com o monorepo e `docker-compose.standalone.yml` para o modo standalone do próprio repositório

Cada slice carrega:

1. `../../.env`
2. `./.env`

Com isso, o root define o baseline e o slice só sobrescreve o que precisar.

Os `.env.example` dos slices usam os nomes de variáveis que o próprio Compose consome.
Isso evita drift entre "arquivo de exemplo" e "ambiente que realmente sobe".

## Fluxo Full Stack

No root:

```bash
task up
task logs
task ps
task down
```

`task down` no root derruba o stack inteiro.

## Fluxo por Slice

Exemplos:

```bash
cd services/yggdrasil-core && task up
cd surfaces/yggdrasil-auth-surface && task up
cd surfaces/yggdrasil-console && task up
```

Depois de instalar uma integration:

```bash
cd integrations/integration-github && task up
```

Boas práticas do comportamento local:

- `task up` sobe só o serviço do slice e as dependências declaradas dele
- `task down` para só o serviço do slice
- `task restart` reinicia só o serviço do slice
- `task config` valida a composição daquele slice

Isso evita um problema comum em monorepos Docker: um serviço parar toda a infra compartilhada sem querer.

## Convenções adotadas

- infraestrutura comum em `dev/compose/infra.yml`
- banco e broker compartilhados pelo mesmo `COMPOSE_PROJECT_NAME`
- `docker-compose.yml` em cada slice
- `Taskfile.yml` em cada slice
- `Taskfile.yml` no root para orquestração do stack completo
- `services/` reservado para o coração do produto
- `surfaces/` reservado para APIs, auth, UIs e bordas substituíveis
- catálogo de surfaces em `catalog/surfaces.json`
- as surfaces ativas do runtime ficam em `catalog/surfaces.active`
- as surfaces de referência ativas hoje são `yggdrasil-auth-surface` e `yggdrasil-console`
- integrations instaladas como `git submodule`
- compose do root descobrindo automaticamente integrations instaladas
- compose do root carregando apenas as surfaces marcadas como ativas e instaladas
- base oficial para novas surfaces em [`surface-template`](https://github.com/dakasa-yggdrasil/surface-template)
- `task surfaces:scaffold` como espelho local dessa base para bootstrap rápido

## Banco local

O Postgres compartilhado cria:

- banco padrão compartilhado do workspace
- `yggdrasil_core`

## Dicas operacionais

- use `task config` antes do primeiro `task up` de um slice se estiver ajustando compose
- use `task arch:check` quando mexer em fronteiras entre serviços
- use `task bootstrap:core` depois do primeiro `task up` completo para carregar manifests bootstrap
- use `task install:smoke` para validar que surfaces e integrations do catálogo instalam corretamente a partir dos repositórios remotos
- use `task integrations:list` para ver o catálogo disponível
- use `task surfaces:list` para ver o catálogo de surfaces disponível
- use `task surfaces:install NAME=<slug>` para instalar uma surface de referência
- prefira criar novas surfaces a partir de [`surface-template`](https://github.com/dakasa-yggdrasil/surface-template)
- use `task surfaces:scaffold NAME=<slug>` só quando quiser um scaffold local rápido dentro do workspace
- use `task integrations:install NAME=<slug>` para instalar uma integration específica
- para desenvolvimento com cópias locais fora do monorepo, defina `YGGDRASIL_SURFACES_DEV_DIR` e `YGGDRASIL_INTEGRATIONS_DEV_DIR`; sem isso, o installation manager usa apenas os remotos do catálogo
- se precisar resetar tudo, faça isso no root com `task down` e depois remova volumes manualmente no Docker Desktop
