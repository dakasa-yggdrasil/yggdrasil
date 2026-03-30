# yggdrasil

`yggdrasil` agora é o repositório do produto.

Ele junta os microsserviços centrais, o catálogo de plugins e a automação local em um único
workspace, mas preservando fronteiras de runtime entre os serviços.

Em outras palavras:

- o repo é único
- o produto é único
- o runtime continua dividido em microsserviços
- só o `yggdrasil-core` é parte obrigatória do coração do produto
- o console de referência fala direto com o `yggdrasil-core` por HTTP
- auth e consoles vivem como surfaces substituíveis
- plugins continuam opcionais e instaláveis

Com isso, evitamos o erro de um "monolito com pastas" e mantemos:

- cada microsserviço tem `docker-compose.yml`
- cada microsserviço tem `Taskfile.yml`
- a infraestrutura comum fica centralizada
- o stack completo pode subir como uma única composição
- integrations são instaladas sob demanda, como `git submodule`

## Estrutura

```text
yggdrasil/
├── dev/
│   └── compose/
├── services/
│   └── yggdrasil-core/
├── surfaces/
│   ├── yggdrasil-auth-surface/ # surface instalada quando presente
│   ├── yggdrasil-console/      # surface instalada quando presente
├── catalog/
│   ├── integrations.json
│   ├── surfaces.json
│   └── surfaces.active
├── cmd/
│   └── ygg/
└── integrations/
    └── <installed-submodules>
```

## Convenções

- infraestrutura compartilhada em [`dev/compose/infra.yml`](/Users/dakasa/projects/yggdrasil/dev/compose/infra.yml)
- variáveis globais em `.env`
- overrides por serviço em `./services/*/.env`, `./surfaces/*/.env` ou `./integrations/*/.env`
- `Taskfile.yml` local sempre usa o mesmo `COMPOSE_PROJECT_NAME`, para todas as slices
  compartilharem rede, broker e banco
- cada `task up` local sobe só o slice pedido e as dependências dele
- cada `task down` local para só o slice local, sem derrubar a infraestrutura compartilhada
- integrations instaladas são descobertas dinamicamente pelo root, sem lista hardcoded
- as surfaces ativas do runtime ficam em [`catalog/surfaces.active`](/Users/dakasa/projects/yggdrasil/catalog/surfaces.active)
- o catálogo de surfaces vive em [`catalog/surfaces.json`](/Users/dakasa/projects/yggdrasil/catalog/surfaces.json)
- as surfaces de referência são instaláveis separadamente, como repositórios próprios
- o catálogo de integrations vive em [`catalog/integrations.json`](/Users/dakasa/projects/yggdrasil/catalog/integrations.json)
- integrations pertencem ao core; surfaces consomem contratos do core em vez de falar com plugins diretamente

## Primeiros passos

Pré-requisitos:

- Docker Desktop com `docker compose`
- [`task`](https://taskfile.dev)

Valide o ambiente:

```bash
task doctor
task arch:check
```

Liste as integrations disponíveis:

```bash
task integrations:list
```

Liste as surfaces catalogadas:

```bash
task surfaces:list
task surfaces:active
```

Instale as surfaces de referência:

```bash
task surfaces:install NAME=yggdrasil-auth-surface
task surfaces:install NAME=yggdrasil-console
```

Crie uma nova surface a partir da base oficial [`surface-template`](https://github.com/dakasa-yggdrasil/surface-template).

Se você só quiser um scaffold rápido dentro do workspace local, ainda existe o atalho:

```bash
task surfaces:scaffold NAME=my-domain-api
```

Abra o installation manager em TUI:

```bash
task integrations:tui
```

Também dá para usar a CLI diretamente:

```bash
./scripts/ygg.sh integrations list
./scripts/ygg.sh integrations install rabbitmq
```

Inicialize os arquivos `.env`:

```bash
task env:init
```

Suba o stack completo:

```bash
task up
```

Importe os bootstrap manifests do core:

```bash
task bootstrap:core
```

Rode o smoke end-to-end do produto:

```bash
task smoke
```

Abra o console:

```bash
task open:console
```

## Operação por slice

Você também pode trabalhar por serviço:

```bash
cd services/yggdrasil-core && task up
cd surfaces/yggdrasil-auth-surface && task up
cd surfaces/yggdrasil-console && task up
```

E, se uma integration estiver instalada:

```bash
cd integrations/integration-github && task up
```

Cada slice sobe só o que precisa, mas reaproveita a mesma infraestrutura compartilhada.

As surfaces de referência do produto são repositórios separados. Se uma empresa
quiser, ela pode:

- substituir `yggdrasil-auth-surface`
- substituir `yggdrasil-console`
- adicionar APIs nichadas próprias por domínio

Desde que essas surfaces continuem falando com o [`yggdrasil-core`](/Users/dakasa/projects/yggdrasil/services/yggdrasil-core).

Para criar uma nova surface própria, o caminho preferencial agora é começar por
[`surface-template`](https://github.com/dakasa-yggdrasil/surface-template) e
depois instalar esse repositório no workspace.

O catálogo [`catalog/surfaces.active`](/Users/dakasa/projects/yggdrasil/catalog/surfaces.active)
define o runtime oficial que sobe com `task up`.

O ponto de arquitetura importante é este: mesmo quando uma surface precisar acionar plugins,
ela faz isso através do core. O vínculo de integrations continua sempre sendo `core <-> integration`.

Para detalhes de fluxo local, variáveis e convenções, veja
[`docs/local-development.md`](/Users/dakasa/projects/yggdrasil/docs/local-development.md).

## Boas práticas adotadas

- infraestrutura comum separada do runtime dos serviços
- compose em camadas, para evitar copiar broker e banco em todo lugar
- `.env.example` no root e por serviço
- `task up` por slice em vez de subir todos os serviços de cada arquivo por acidente
- `task down` por slice sem derrubar broker, banco e peers
- integrations fora do monorepo por padrão, instaladas como submodules quando necessário
- guardrails automáticos para evitar imports diretos entre microsserviços
- only-core-by-default: o único runtime central obrigatório é o `yggdrasil-core`
- portas e nomes de serviço estáveis
- banco local enxuto para o produto atual, sem seed obrigatório de surfaces legadas
- `Taskfile` local para `up`, `down`, `logs`, `ps`, `shell` e `test`
- smoke end-to-end versionado no próprio repo, reutilizável localmente e em CI

Para a regra arquitetural do produto, veja
[`docs/architecture/product-repo.md`](/Users/dakasa/projects/yggdrasil/docs/architecture/product-repo.md).
