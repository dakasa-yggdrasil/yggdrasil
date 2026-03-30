# Bootstrap Integrations

This directory stores bootstrap manifests for ecosystem plugins that `yggdrasil-core` can consume.

Current bootstrap integrations are already organized around the plugin catalog convention.

Current catalog view:

- `gcp`
  - `operations/api` -> `gcp-integration-type.json`
- `github`
  - `operations/api` -> `github-integration-type.json`
- `grafana`
  - `operations/api` -> `grafana-integration-type.json`
  - `installations/kubernetes` -> `grafana-on-kubernetes-integration-type.json`
- `kubernetes`
  - `operations/api` -> `kubernetes-integration-type.json`
- `rabbitmq`
  - `operations/api` -> `rabbitmq-integration-type.json`
  - `installations/kubernetes` -> `rabbitmq-on-kubernetes-integration-type.json`

Current bootstrap instances:

- `kubernetes-platform-prod.json`
- `github-caller.json`
- `gcp-platform.json`
- `grafana-platform-api.json`
- `grafana-on-kubernetes-platform-prod.json`
- `rabbitmq-platform-api.json`
- `rabbitmq-on-kubernetes-platform-prod.json`

These manifests describe how the core can reach the adapters over RabbitMQ RPC. They do not deploy the adapter workers themselves.

Some integrations can also act as optional discovery sources. The first generic
convention is `catalog_discover`, which lets the core ask an integration
instance for candidate plugins or surfaces. This is intentionally provider
agnostic:

- GitHub can implement it
- GitLab can implement it
- filesystem or OCI scanners can implement it later

The intended flow is:

1. an installation-focused integration such as RabbitMQ generates or reconciles desired objects
2. the Kubernetes target integration applies and observes those objects in the target cluster
