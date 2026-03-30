# Product Repo Architecture

O `yggdrasil` é o repositório do produto.

Mas ele **não** é um monólito de runtime.

A regra central é:

- um repositório
- múltiplos microsserviços
- múltiplos plugins
- contratos explícitos
- acoplamento somente por rede, broker e schemas públicos

## O que isso significa na prática

O monorepo existe para:

- facilitar contribuição
- padronizar dev local
- versionar o produto como plataforma
- compartilhar documentação, catálogo e automação de workspace

Ele **não** existe para:

- permitir import direto entre microsserviços
- criar bibliotecas internas compartilhadas arbitrárias entre runtimes
- esconder integrações dentro do código do produto
- transformar o produto em um processo único acoplado

## Regra de fronteira

Cada runtime em `services/` ou `surfaces/` é uma unidade independente.

Ele deve ter:

- seu próprio `go.mod` ou stack equivalente
- seu próprio `docker-compose.yml`
- seu próprio `Taskfile.yml`
- seus próprios contratos operacionais

E ele **não deve**:

- importar código de outro microsserviço
- importar código de plugin de integração
- depender do package `internal/` do workspace raiz

## Regra para surfaces

`surfaces/` existe para todo runtime substituível de borda.

Exemplos:

- APIs generalistas
- APIs nichadas por domínio
- auth services
- admin consoles
- BFFs corporativos

Uma empresa pode substituir completamente:

- `yggdrasil-auth-surface`
- `yggdrasil-console`

Sem tocar no coração do produto, desde que respeite os contratos do `yggdrasil-core`.

As surfaces de referência ativas hoje são `yggdrasil-auth-surface` e `yggdrasil-console`, definidas em
[`catalog/surfaces.active`](/Users/dakasa/projects/yggdrasil/catalog/surfaces.active).

O catálogo instalável dessas surfaces fica em
[`catalog/surfaces.json`](/Users/dakasa/projects/yggdrasil/catalog/surfaces.json).

Isso significa que as surfaces de referência podem viver em repositórios próprios.
O ponto de partida oficial para novas surfaces é
[`surface-template`](https://github.com/dakasa-yggdrasil/surface-template).

O produto principal só precisa saber:

- quais surfaces existem no catálogo
- quais estão instaladas no workspace
- quais estão marcadas como ativas no runtime

Isso também significa que uma empresa pode optar por:

- manter as surfaces de referência
- trocar uma delas por uma surface própria
- dividir a borda em múltiplas APIs nichadas
- manter só o `yggdrasil-core` e construir toda a borda do zero

A regra complementar é: integrations não ficam acopladas a uma surface.

Elas pertencem ao `yggdrasil-core`, que:

- armazena `integration_type` e `integration_instance`
- faz o handshake e health check dos plugins
- executa workflows, products e operações de integration

As surfaces podem pedir essas ações ao core, mas não devem se tornar donas do ciclo de vida das integrations.

## Regra para plugins

As integrations em `integrations/` são extensões instaláveis do produto.

Elas:

- não fazem parte obrigatória do repositório por padrão
- entram como `git submodule`
- são descobertas dinamicamente pelo root
- continuam independentes do runtime dos serviços centrais

O mesmo raciocínio vale para surfaces de referência: elas podem ser repositórios
próprios e entrar no workspace como componentes instaláveis.

## Regra para compartilhamento

Se alguma coisa precisa ser compartilhada entre serviços, ela deve cair em uma destas categorias:

- contrato wire-level publicado
- schema versionado
- mensagem/evento RPC
- documentação

Não criamos "pacote utilitário compartilhado do produto" como atalho.

## Guardrails

O workspace já considera como violação:

- um serviço Go importar o módulo Go de outro serviço
- um serviço importar o módulo raiz do workspace
- um serviço importar o código de runtime de uma integration
- uma surface nova importar diretamente outra surface ou outro serviço Go

Essas verificações rodam por `task arch:check`.

## Leitura correta do produto

Portanto, o desenho do Yggdrasil fica assim:

- o repo é o produto
- os serviços são blocos de runtime
- as integrations são blocos instaláveis
- o root coordena, mas não vira dependência de runtime

É assim que evitamos repetir o erro de um "monolito com pastas".
