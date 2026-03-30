# Surfaces

`surfaces/` contém os serviços de borda instalados no workspace do produto.

Aqui entram:

- APIs públicas ou internas
- BFFs
- consoles web
- serviços de autenticação de colaboradores
- APIs custom de empresas que consumirão o Yggdrasil

Esses serviços **não** são o coração do produto.

O coração continua sendo `services/yggdrasil-core`.

As surfaces de referência do produto hoje são:

- `yggdrasil-auth-surface`
- `yggdrasil-console`

O runtime oficial é controlado por
[`catalog/surfaces.active`](/Users/dakasa/projects/yggdrasil/catalog/surfaces.active).

O catálogo instalável das surfaces fica em
[`catalog/surfaces.json`](/Users/dakasa/projects/yggdrasil/catalog/surfaces.json).

Você pode instalar uma surface de referência com:

```bash
task surfaces:install NAME=yggdrasil-auth-surface
task surfaces:install NAME=yggdrasil-console
```

Você pode criar uma nova surface a partir da base oficial
[`surface-template`](https://github.com/dakasa-yggdrasil/surface-template).

Se quiser apenas gerar uma cópia rápida dentro do workspace local, use:

```bash
task surfaces:scaffold NAME=my-domain-api
```

O `task surfaces:scaffold` é um espelho local dessa base oficial, útil para
experimentação e bootstrap rápido. A convenção preferida para surfaces reais é:

1. criar um repositório a partir de `surface-template`
2. publicar a surface no GitHub da empresa
3. registrar/instalar essa surface no workspace do produto

## Regra

Tudo em `surfaces/` é substituível.

Uma empresa pode:

- usar as surfaces de referência do produto
- remover as surfaces de referência
- adicionar surfaces próprias
- criar APIs nichadas por domínio
- manter múltiplas surfaces coexistindo para times ou domínios diferentes

Desde que essas surfaces falem com o `yggdrasil-core` pelos contratos corretos.

## Relação com integrations

Integrations não pertencem às surfaces.

O vínculo normativo é:

- `surface -> core`
- `core -> integration`

Se uma surface precisar acionar plugin, ela faz isso através do core, nunca tratando a integration
como dependência estrutural própria.
