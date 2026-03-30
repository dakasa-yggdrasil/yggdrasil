# Yggdrasil Auth Surface

`yggdrasil-auth-surface` is the reference auth edge for the product.

It is intentionally a thin edge runtime, not part of the product heart. It
does not own identity truth, password storage, or session storage. Those now
live in `yggdrasil-core`.

The expected shape is:

- talk to `yggdrasil-core` over its synchronous API
- proxy browser or API entrypoints for collaborator authentication
- let the core own collaborators, teams, auth/session truth, authorization
  truth, and surface registry

## Current role

This surface forwards the public auth endpoints to the core:

- `POST /api/v1/auth/login`
- `GET /api/v1/auth/session`
- `POST /api/v1/auth/logout`

It intentionally does not expose the internal password bootstrap endpoint
`POST /api/v1/auth/passwords`. That remains a direct core capability for
trusted operator flows.

## Local development

```bash
task up
task logs
task test
```

The core base URL defaults to `http://yggdrasil-core:9080` and can be adjusted
with `YGGDRASIL_CORE_HTTP_URL`.

The example manifest for registering this surface in the core lives in
[`surface.manifest.json`](./surface.manifest.json).
