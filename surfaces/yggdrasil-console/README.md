# yggdrasil-console

`yggdrasil-console` is the human-facing admin plane for Yggdrasil.

This first slice focuses on the part that was most missing from the ecosystem: a real UI for
feeding the core without editing raw manifests by hand.

## Current scope

- overview of the Yggdrasil plugin ecosystem
- optional catalog discovery
- explicit plugin catalog grouped by `domain / section / entry`
- health-aware integration listing
- guided creation of `integration_instance` from plugin schema
- collaborator and team management
- team membership management
- product manifest authoring
- surface manifest authoring
- workflow manifest authoring

## Architecture

- frontend: this repository
- source of truth: [`/Users/dakasa/projects/yggdrasil/services/yggdrasil-core`](/Users/dakasa/projects/yggdrasil/services/yggdrasil-core)
- primary synchronous control plane: `yggdrasil-core` HTTP API
- optional custom surfaces or BFFs: replaceable edge runtimes chosen by the organization

The console currently talks to the core through:

- `GET /api/v1/console/integration-catalog`
- `GET /api/v1/console/integration-catalog/:domain/:section/:entry`
- `GET /api/v1/console/catalog-discovery`
- `POST /api/v1/console/integration-instances`
- `GET /api/v1/console/collaborators`
- `POST /api/v1/console/collaborators`
- `GET /api/v1/console/teams`
- `POST /api/v1/console/teams`
- `GET /api/v1/console/team-memberships`
- `POST /api/v1/console/team-memberships`
- `GET /api/v1/console/products`
- `POST /api/v1/console/products`
- `GET /api/v1/console/surfaces`
- `POST /api/v1/console/surfaces`
- `GET /api/v1/console/workflows`
- `POST /api/v1/console/workflows`

The new `Discover` area is deliberately separate from the explicit plugin
catalog. It shows what external discovery backends can see right now, while the
catalog continues to show what is actually registered in the core.

## Development

Install dependencies:

```bash
npm install
```

Run the console:

```bash
npm run dev
```

By default, Vite proxies `/api` to `http://localhost:9080`. In the reference setup, that target is
the `yggdrasil-core` HTTP API directly.

Optional environment variables:

- `VITE_YGGDRASIL_API_PROXY`: dev proxy target for Vite
- `VITE_YGGDRASIL_API_BASE_URL`: absolute core base URL when you do not want to use the proxy

## Demo fallback

When `yggdrasil-core` is unavailable during local development, the console falls back to mock
catalog data for read operations so the UI still renders. Write operations remain disabled in that
mode.

## Validation

```bash
npm run build
```
