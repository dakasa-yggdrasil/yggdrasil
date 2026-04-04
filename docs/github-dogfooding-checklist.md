# GitHub Dogfooding Checklist

Use this checklist to enable commit-driven Yggdrasil workflow emission across the
official repositories in the `dakasa-yggdrasil` GitHub organization.

## Baseline

Every repository that ships `.github/workflows/emit-deploy-event.yml` should
configure:

## Recommended GitHub scope strategy

To reduce duplication, the best default is:

- configure shared secrets at the `dakasa-yggdrasil` organization level
- configure the common workflow variables at the organization level
- keep only `YGGDRASIL_COMPONENT_KIND` and `YGGDRASIL_COMPONENT_NAME` as
  repository-level overrides when needed

That means most repositories can inherit the same baseline automatically.

### Required secrets

- `YGGDRASIL_CORE_BASE_URL`
  - Example: `https://core.yggdrasil.example.com`

### Optional secrets

- `YGGDRASIL_WORKFLOW_RUN_TOKEN`
  - Set this when `yggdrasil-core` protects `POST /api/v1/workflow-runs` with
    `YGGDRASIL_WORKFLOW_RUN_TOKEN`.

### Optional repository variables

- `YGGDRASIL_WORKFLOW_NAMESPACE`
  - Recommended: `global`
- `YGGDRASIL_WORKFLOW_NAME`
  - Recommended: `ecosystem-repository-commit`
- `YGGDRASIL_DEPLOY_WORKFLOW`
  - Recommended: `deploy.yml`
- `YGGDRASIL_COMPONENT_KIND`
  - Depends on the repository type
- `YGGDRASIL_COMPONENT_NAME`
  - Usually the repository name, except for the reference surfaces below
- `YGGDRASIL_DEPLOY_ENVIRONMENT`
  - Recommended: `production`

Recommended organization-level defaults:

- `YGGDRASIL_WORKFLOW_NAMESPACE=global`
- `YGGDRASIL_WORKFLOW_NAME=ecosystem-repository-commit`
- `YGGDRASIL_DEPLOY_WORKFLOW=deploy.yml`
- `YGGDRASIL_DEPLOY_ENVIRONMENT=production`

If a repository does not set the optional variables, the workflows still run
with sane defaults. The purpose of the table below is to make those defaults
explicit and record the recommended overrides.

## Reference repositories

| Repository | Component kind | Component name | Notes |
| --- | --- | --- | --- |
| `dakasa-yggdrasil/yggdrasil` | `product` | `yggdrasil` | Product monorepo |
| `dakasa-yggdrasil/surface-auth` | `surface` | `yggdrasil-auth-surface` | Set `YGGDRASIL_COMPONENT_NAME` explicitly because the repo is named `surface-auth` |
| `dakasa-yggdrasil/surface-console` | `surface` | `yggdrasil-console` | Set `YGGDRASIL_COMPONENT_NAME` explicitly because the repo is named `surface-console` |
| `dakasa-yggdrasil/surface-template` | `surface` | `surface-template` | Template repo |
| `dakasa-yggdrasil/integration-template` | `integration` | `integration-template` | Template repo |
| `dakasa-yggdrasil/integration-aws` | `integration` | `integration-aws` | Operations plugin |
| `dakasa-yggdrasil/integration-gcp` | `integration` | `integration-gcp` | Operations plugin |
| `dakasa-yggdrasil/integration-github` | `integration` | `integration-github` | Operations plugin |
| `dakasa-yggdrasil/integration-grafana` | `integration` | `integration-grafana` | Operations plugin |
| `dakasa-yggdrasil/integration-grafana-on-kubernetes` | `integration` | `integration-grafana-on-kubernetes` | Installation plugin |
| `dakasa-yggdrasil/integration-kubernetes` | `integration` | `integration-kubernetes` | Target-side operations plugin |
| `dakasa-yggdrasil/integration-rabbitmq` | `integration` | `integration-rabbitmq` | Operations plugin |
| `dakasa-yggdrasil/integration-rabbitmq-on-kubernetes` | `integration` | `integration-rabbitmq-on-kubernetes` | Installation plugin |

## Suggested repository variable sets

### yggdrasil

- `YGGDRASIL_COMPONENT_KIND=product`
- `YGGDRASIL_COMPONENT_NAME=yggdrasil`
- `YGGDRASIL_DEPLOY_ENVIRONMENT=production`

### surface-auth

- `YGGDRASIL_COMPONENT_KIND=surface`
- `YGGDRASIL_COMPONENT_NAME=yggdrasil-auth-surface`
- `YGGDRASIL_DEPLOY_ENVIRONMENT=production`

### surface-console

- `YGGDRASIL_COMPONENT_KIND=surface`
- `YGGDRASIL_COMPONENT_NAME=yggdrasil-console`
- `YGGDRASIL_DEPLOY_ENVIRONMENT=production`

### surface-template

- `YGGDRASIL_COMPONENT_KIND=surface`
- `YGGDRASIL_COMPONENT_NAME=surface-template`
- `YGGDRASIL_DEPLOY_ENVIRONMENT=production`

### integration-template

- `YGGDRASIL_COMPONENT_KIND=integration`
- `YGGDRASIL_COMPONENT_NAME=integration-template`
- `YGGDRASIL_DEPLOY_ENVIRONMENT=production`

### integration-*

For the real integration repositories, the simplest rule is:

- `YGGDRASIL_COMPONENT_KIND=integration`
- `YGGDRASIL_COMPONENT_NAME=<repository-name>`
- `YGGDRASIL_DEPLOY_ENVIRONMENT=production`

That means:

- `integration-aws` -> `YGGDRASIL_COMPONENT_NAME=integration-aws`
- `integration-gcp` -> `YGGDRASIL_COMPONENT_NAME=integration-gcp`
- `integration-github` -> `YGGDRASIL_COMPONENT_NAME=integration-github`
- `integration-grafana` -> `YGGDRASIL_COMPONENT_NAME=integration-grafana`
- `integration-grafana-on-kubernetes` -> `YGGDRASIL_COMPONENT_NAME=integration-grafana-on-kubernetes`
- `integration-kubernetes` -> `YGGDRASIL_COMPONENT_NAME=integration-kubernetes`
- `integration-rabbitmq` -> `YGGDRASIL_COMPONENT_NAME=integration-rabbitmq`
- `integration-rabbitmq-on-kubernetes` -> `YGGDRASIL_COMPONENT_NAME=integration-rabbitmq-on-kubernetes`

## Core-side prerequisites

GitHub-side settings are not enough by themselves. The following must also be
true in the running platform:

- the bootstrap workflow `global/ecosystem-repository-commit` must be imported
  into `yggdrasil-core`
- the integration instance `global/github-caller` must exist in the core
- `global/github-caller` must have real GitHub credentials capable of dispatching
  repository workflows
- if the core sets `YGGDRASIL_WORKFLOW_RUN_TOKEN`, repositories must also set
  the matching secret `YGGDRASIL_WORKFLOW_RUN_TOKEN`

## Recommended github-caller instance settings

In `yggdrasil-core`, the integration instance `global/github-caller` should be
configured with:

- credential `token`
- config `api_base_url=https://api.github.com`
- config `default_ref=main`
- optional config `default_owner=dakasa-yggdrasil`
- optional config `default_workflow=deploy.yml`

The most important requirement is that the GitHub token can dispatch workflows
in the target repositories.

## Recommended rollout order

1. Configure `YGGDRASIL_CORE_BASE_URL` in all repositories.
2. Configure `YGGDRASIL_WORKFLOW_RUN_TOKEN` only if the core requires it.
3. Set explicit `YGGDRASIL_COMPONENT_NAME` for `surface-auth` and
   `surface-console`.
4. Import bootstrap manifests into the core.
5. Validate that `global/github-caller` can dispatch `deploy.yml`.
6. Trigger one manual `workflow_dispatch` of `emit-deploy-event.yml` in a test
   repository before relying on `push -> main`.
