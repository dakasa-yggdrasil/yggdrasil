# __SURFACE_DISPLAY_NAME__

This is a scaffolded Yggdrasil surface.

It is intentionally an edge runtime, not part of the product heart. The expected
shape is:

- talk to `yggdrasil-core` over its synchronous API
- consume core contracts
- let the core own integrations, workflows, products, and authorization truth

## Local development

```bash
task up
task logs
task test
```

The example manifest for registering this surface in the core lives in
[`surface.manifest.json`](./surface.manifest.json).
