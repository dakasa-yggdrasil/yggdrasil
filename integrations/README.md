# Integrations

As integrations do Yggdrasil não ficam mais vendorizadas nem hardcoded no monorepo.

Esse diretório é reservado para integrations instaladas como `git submodule`, sob demanda.

Exemplos:

```bash
go run ./cmd/ygg integrations list
go run ./cmd/ygg integrations install rabbitmq
go run ./cmd/ygg integrations tui
```

Ou, usando `task` no root:

```bash
task integrations:list
task integrations:install NAME=rabbitmq
task integrations:tui
```
