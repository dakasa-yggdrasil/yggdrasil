# Bootstrap Workflows

This directory stores bootstrap `workflow` manifests that the core can import as first-class orchestration definitions.

Current bootstrap workflows:

- `github-dispatch.json`

`github-dispatch` is a transitional wrapper around the GitHub integration. It lets edge services call `yggdrasil-core.workflow.run` while the legacy topology model still emits repository/workflow pairs.

Real deployment workflows are expected to live in the core as normal `workflow` manifests, typically labeled with:

- `workflow_type=deployment`
- `workflow_build_name=<build-name>`
- `workflow_env_type=<env-type>`
- `workflow_component_id=<component-uuid>`

Those manifests should store stable dispatch values in `spec.defaults` and accept only runtime overrides from callers.
