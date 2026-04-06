# Heimdall LLM Activation

This is the shortest safe path to activate Heimdall's bounded GPT + Claude
fallback in a running Yggdrasil environment.

The activation script does two things in one step:

1. creates a new `integration_instance` version for `global/heimdall-guardian`
2. sends the GPT and Claude API keys inline so the core materializes them into
   `managed_secret` records and persists only `secret://...` refs in the
   instance config

That keeps the operational flow simple while still following the core secret
discipline.

## Required environment variables

- `HEIMDALL_GPT_API_KEY`
- `HEIMDALL_CLAUDE_API_KEY`

## Optional environment variables

- `YGGDRASIL_CORE_BASE_URL`
  - default: `http://127.0.0.1:${CORE_HTTP_PORT:-9080}`
- `HEIMDALL_GPT_MODEL`
  - default: `gpt-5.4-mini-2026-03-17`
- `HEIMDALL_CLAUDE_MODEL`
  - default: `claude-sonnet-4-20250514`
- `HEIMDALL_PROVIDER_ORDER`
  - default: `gpt,claude`
- `HEIMDALL_GPT_BASE_URL`
  - default: `https://api.openai.com/v1`
- `HEIMDALL_CLAUDE_BASE_URL`
  - default: `https://api.anthropic.com/v1`
- `HEIMDALL_CLAUDE_VERSION`
  - default: `2023-06-01`
- `HEIMDALL_LLM_MODE`
  - default: `fallback`

## Activation

```bash
export HEIMDALL_GPT_API_KEY="..."
export HEIMDALL_CLAUDE_API_KEY="..."
task heimdall:activate:llm
task heimdall:verify:llm
```

Or directly:

```bash
HEIMDALL_GPT_API_KEY="..." \
HEIMDALL_CLAUDE_API_KEY="..." \
./scripts/activate-heimdall-llm.sh
```

Verification:

```bash
./scripts/verify-heimdall-llm.sh
```

## Result

After activation, the script verifies that:

- `llm_enabled` is `true`
- `llm_provider_order` is set
- `llm_gpt_api_key` is persisted as a `secret://...` ref
- `llm_claude_api_key` is persisted as a `secret://...` ref

The verification task then confirms that:

- the active `integration_instance` still has `llm_enabled=true`
- both provider refs are present
- both managed secrets exist
- both secrets expose the expected `value` key

The managed secret names created by the core follow the standard
`<instance>-config-<field>` shape, so for the default instance you should see:

- `global/heimdall-guardian-config-llm-gpt-api-key`
- `global/heimdall-guardian-config-llm-claude-api-key`

## Operational note

This enables the LLM layer, but Heimdall still remains bounded by:

- `guardian_policy`
- blast-radius limits
- maintenance mode
- business hours / freeze windows
- approval mode when required

So this turns on deeper reasoning, not unrestricted autonomy.
