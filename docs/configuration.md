# Configuration

The configuration is a single YAML file. Start from
[configs/config.yaml.example](../configs/config.yaml.example), which documents
every option inline.

- [File location](#file-location)
- [`server`](#server)
- [`metrics`](#metrics)
- [`proxy`](#proxy)
- [Providers](#providers)
- [Routes](#routes)
- [Environment variables](#environment-variables)
- [Provider examples](#provider-examples)

Unknown keys are rejected at startup, so a typo fails loudly instead of being
ignored. Check a file and its routing with `claudeproxy route`:

```sh
claudeproxy route -c config.yaml claude-opus-5-5 minimax-m3 deepseek-v4.1-flash
```

## File location

The file is read from, in order:

1. the `-c` / `--config` flag;
2. `$CLAUDEPROXY_CONFIG` (the Docker image sets it to
   `/etc/claudeproxy/config.yaml`);
3. `claudeproxy/config.yaml` in the user configuration directory
   (`~/.config` on Linux, `~/Library/Application Support` on macOS).

The file is read once at startup: restart the proxy after editing it.

## `server`

| Key | Default | Description |
|---|---|---|
| `listen` | `127.0.0.1:8787` | Listen address. Must be a loopback address unless `serve` gets `--allow-non-loopback`. `--listen` overrides it. |
| `log_level` | `info` | `debug`, `info`, `warn` or `error` |
| `log_format` | `text` | `text` or `json` |
| `read_header_timeout` | `10s` | Time a client may take to send its request headers |
| `idle_timeout` | `120s` | Keep-alive idle timeout |
| `shutdown_timeout` | `30s` | Time in-flight streams get to finish on `SIGINT` / `SIGTERM` |

There is deliberately no read or write timeout: a streamed response can last
minutes.

## `metrics`

Usage recording is on by default.

| Key | Default | Description |
|---|---|---|
| `disabled` | `false` | `true` turns recording and the [dashboard](dashboard.md) off |
| `path` | `claudeproxy/usage.db` in the user configuration directory | SQLite database file |

If the database cannot be opened, the proxy keeps running without metrics and
logs a warning; the dashboard shows the reason.

The example sets `path: /var/lib/claudeproxy/usage.db`, the volume used by
Docker. Outside Docker, remove it or choose a writable path.

## `proxy`

| Key | Default | Description |
|---|---|---|
| `max_body_bytes` | `67108864` (64 MiB) | Largest Messages API body accepted. Bodies are buffered to read the model, images included. |
| `default_provider` | empty | Provider for requests that carry no model (`GET /v1/models`, telemetry...) and for models no route matches. Empty rejects unmatched models. |
| `providers` | | Map of provider name to [provider](#providers) |
| `routes` | | Ordered list of [routes](#routes) |

## Providers

Each entry of `proxy.providers` is an upstream API.

| Key | Required | Description |
|---|---|---|
| `base_url` | yes | Joined with the request path, e.g. `https://openrouter.ai/api` + `/v1/messages` |
| `auth` | yes | `passthrough`, `bearer` or `x-api-key`, see below |
| `api_key` | for `bearer` and `x-api-key` | Key sent to the provider, usually `${ENV_VAR}` |
| `format` | no | Empty for the Anthropic Messages API, `openai-chat` for an OpenAI chat-completions provider (see [OpenAI translation](other-models.md#openai-chat-completions-providers)) |
| `headers` | no | Extra headers to set, e.g. `User-Agent` |
| `drop_headers` | no | Headers to remove before forwarding |
| `flatten_mid_conversation_updates` | no | Rewrite the mid-conversation system messages Claude Code sends, which non-Claude models reject |
| `keep_claude_metadata` | no | Keep the body's `metadata` object and billing attribution block for a non-Anthropic provider |

**`auth`**

- `passthrough` forwards the client's credentials (OAuth token or API key)
  unchanged. Use it for Anthropic only: any other provider would receive
  your Anthropic credentials.
- `bearer` removes `Authorization`, `x-api-key` and `Cookie`, then sends
  `Authorization: Bearer <api_key>`.
- `x-api-key` removes the same headers, then sends `x-api-key: <api_key>`.

With `bearer` and `x-api-key`, the proxy also strips what identifies the
client as Claude Code; see [Security and privacy](security.md#what-a-third-party-provider-receives).

**`flatten_mid_conversation_updates`**

Claude Code sends system messages in the middle of a conversation, for
example when tools are added or the reasoning effort changes. Recent Claude
models accept them; most other models answer
`400 Invalid Anthropic Messages API request`. With this option:

- `tool_addition` blocks become entries of the request's `tools`;
- a message-level `output_config` (reasoning effort) replaces the request one.

Enable it on every provider that serves non-Claude models.

## Routes

`proxy.routes` is evaluated in order and the first match wins. A route has
either `match` or `regex`:

| Key | Description |
|---|---|
| `match` | Glob on the requested model. `*` matches any run of characters, `/` included; `?` matches one character. |
| `regex` | RE2 expression on the requested model |
| `provider` | Name of a provider in `proxy.providers` |
| `model` | Optional model name sent upstream. `$1`, `$2`... or `${name}` insert the wildcards or capture groups. |

The body is rewritten only when `model` changes the name.

Examples:

```yaml
routes:
  # Claude Code believes it uses Haiku; the request goes to Qwen.
  - match: "claude-haiku-*"
    provider: opencode-go
    model: qwen3.8-flash

  # Other Claude models go to Anthropic with your own login.
  - match: "claude-*"
    provider: anthropic

  # A prefix that is stripped upstream:
  # "openrouter/deepseek/deepseek-v4.1-flash" -> "deepseek/deepseek-v4.1-flash".
  - match: "openrouter/*"
    provider: openrouter
    model: $1

  # Several model families on one provider.
  - regex: "^(deepseek-|glm-|kimi-)"
    provider: opencode-go-openai

  # Everything else.
  - match: "*"
    provider: opencode-go
```

Order matters: `claude-haiku-*` must come before `claude-*`, which would
match it first. Put specific routes before general ones.

## Environment variables

`${VAR}` and `$VAR` are expanded in `base_url`, `api_key` and header values.
A referenced variable that is not set stops startup with the list of missing
names. An empty value counts as set: Docker Compose passes every provider key,
empty when you did not define it.

Only enabled (uncommented) providers are expanded, so the example starts with
no variable defined.

## Provider examples

Anthropic, with your own login:

```yaml
anthropic:
  base_url: https://api.anthropic.com
  auth: passthrough
```

[OpenCode Go](https://opencode.ai), Anthropic Messages models (`minimax-m3`,
`qwen3.8-flash`...):

```yaml
opencode-go:
  base_url: https://opencode.ai/inference/go/anthropic
  auth: x-api-key
  api_key: ${OPENCODE_API_KEY}
  flatten_mid_conversation_updates: true
```

OpenCode Go, chat-completions models (`deepseek-v4.1-flash`, `glm-*`,
`kimi-*`...). The Claude Code session id is forwarded as `X-Opencode-Session`:

```yaml
opencode-go-openai:
  base_url: https://opencode.ai/inference/go/openai
  format: openai-chat
  auth: bearer
  api_key: ${OPENCODE_API_KEY}
  flatten_mid_conversation_updates: true
  headers:
    User-Agent: claudeproxy/1.0
```

[OpenRouter](https://openrouter.ai), Anthropic Messages API, models named by
their `vendor/model` slug:

```yaml
openrouter:
  base_url: https://openrouter.ai/api
  auth: bearer
  api_key: ${OPENROUTER_API_KEY}
  flatten_mid_conversation_updates: true
```

OpenRouter, chat-completions endpoint, for models whose Anthropic
compatibility falls short:

```yaml
openrouter-openai:
  base_url: https://openrouter.ai/api
  format: openai-chat
  auth: bearer
  api_key: ${OPENROUTER_API_KEY}
  flatten_mid_conversation_updates: true
```
