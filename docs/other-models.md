# Using other models

Once a provider is enabled in the [configuration](configuration.md), any model
name a route sends to it can be used from Claude Code. Keep a Claude model as
the main model and send selected work elsewhere.

- [Subagents](#subagents)
- [Model environment variables](#model-environment-variables)
- [Diverting a Claude tier](#diverting-a-claude-tier)
- [OpenAI chat-completions providers](#openai-chat-completions-providers)
- [Limits](#limits)

## Subagents

A subagent whose `model:` is a provider model id runs on that model while the
main conversation stays on Claude. Create `~/.claude/agents/qwen-flash.md`:

```markdown
---
name: qwen-flash
description: Runs a task on qwen3.8-flash through claudeproxy. Use when the user explicitly asks for Qwen.
model: qwen3.8-flash
---

Do exactly what the task asks, concisely.
```

Restart Claude Code so it loads the agent, then ask for it by name ("run the
qwen-flash agent to ..."). The request carries `model: qwen3.8-flash`, which
the routes send to the provider serving it.

The provider needs `flatten_mid_conversation_updates: true`; without it, it may
answer `400 Invalid Anthropic Messages API request` (see
[Configuration](configuration.md#providers)).

## Model environment variables

Name a provider model wherever Claude Code accepts a model id:

```sh
ANTHROPIC_DEFAULT_SONNET_MODEL=qwen3.8-flash claude
ANTHROPIC_DEFAULT_HAIKU_MODEL=minimax-m3 claude
CLAUDE_CODE_SUBAGENT_MODEL=deepseek-v4.1-flash claude
```

These can also go in the `env` object of `~/.claude/settings.json`.

## Diverting a Claude tier

Rename a Claude model in the proxy, so Claude Code is unchanged and still
believes it uses that model:

```yaml
routes:
  - match: "claude-haiku-*"
    provider: opencode-go
    model: qwen3.8-flash
  - match: "claude-*"
    provider: anthropic
```

Every request for a Haiku model now goes to Qwen.

## OpenAI chat-completions providers

A provider with `format: openai-chat` speaks the OpenAI chat-completions API.
The proxy translates each Messages API request to `/v1/chat/completions` and
the response back, so Claude Code sees an Anthropic response.

The translation covers:

- text and images;
- tool definitions, tool calls and tool results;
- streaming (SSE) and non-streaming responses;
- usage counters, so the [dashboard](dashboard.md) records tokens.

It drops reasoning content and `thinking` blocks. `count_tokens` is answered
by the proxy with an estimate of about 4 bytes per token, without calling the
provider.

## Limits

- `count_tokens` requests to an Anthropic Messages provider go to that
  provider, and not every provider implements the endpoint.
- Features specific to Claude models (extended thinking, prompt caching
  semantics, server tools) depend on what the provider supports.
- Claude Code tunes its prompts for Claude; other models may follow tool
  instructions less reliably.
