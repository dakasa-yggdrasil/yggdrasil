# Bootstrap extras

Manifestos de bootstrap que **viviam dentro do `services/yggdrasil-core/docs/bootstrap/manifests/integrations/`** quando `services/yggdrasil-core/` era cópia local do código.

Quando o `yggdrasil-core` virou submodule apontando para `dakasa-yggdrasil/yggdrasil-core`, esses manifests precisavam de um lar fora do submodule (porque o standalone não os carrega — provavelmente porque cada integração tem seu próprio bootstrap em seu repo).

Os 4 manifests aqui são:

- `integrations/grafana-on-kubernetes-integration-type.json` — template do `integration_type` para o adapter Grafana-on-Kubernetes
- `integrations/grafana-on-kubernetes-platform-prod.json` — exemplo de `integration_instance` para prod
- `integrations/rabbitmq-on-kubernetes-integration-type.json` — idem para RabbitMQ-on-Kubernetes
- `integrations/rabbitmq-on-kubernetes-platform-prod.json` — exemplo de instance

Provavelmente devem ser portados para o repo `integration-grafana-on-kubernetes` e `integration-rabbitmq-on-kubernetes` (que hoje só têm o adapter Go, sem manifests JSON). Por enquanto, ficam aqui como referência histórica.
