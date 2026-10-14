# claudeproxy

An HTTP proxy that sits between Claude Code and its providers.
It sends each request to a provider based on the model name in the request:

- **Anthropic models** are forwarded untouched. Headers, OAuth token, body bytes
  and SSE stream all pass through as they are.
- **Other models** are routed to an Anthropic-compatible provider such as
  OpenRouter (`https://openrouter.ai/api`), DeepSeek or Kimi. The proxy uses its
  own API key for these, and the client's Anthropic credentials are always removed.

Overhead is kept low:

- Only `POST /v1/messages*` bodies are buffered.
- The `model` field is located by a single-pass scanner, with no JSON decoding
  and 1 allocation. Claude Code sends `model` first, so in practice the
  scanner reads only a few bytes.
- The body is re-spliced only when a route renames the model.
- Responses are streamed and flushed immediately.

## Install

### 1. Run the proxy

With Docker Compose, from a clone of this repository:

```sh
cp .env.example .env    # set OPENROUTER_API_KEY (and CLAUDEPROXY_PORT if 8787 is taken)
docker compose up -d --build
```

Or with the prebuilt image (linux/amd64 and linux/arm64), published to GHCR by
the [docker workflow](.github/workflows/docker.yml):

```sh
docker run -d --name claudeproxy --restart unless-stopped \
  -p 127.0.0.1:8787:8787 \
  -e OPENROUTER_API_KEY=sk-or-... \
  -v "$PWD/configs/config.yaml:/etc/claudeproxy/config.yaml:ro" \
  ghcr.io/jlugagne/claudeproxy:latest
```

The image ships with [configs/config.yaml](configs/config.yaml); the volume is
only needed to use your own. Restart the container after you edit the file.
The proxy is published on `127.0.0.1` only.

Or from source:

```sh
export OPENROUTER_API_KEY=sk-or-...
go run . serve -c configs/config.yaml          # listens on 127.0.0.1:8787
```

Check where each model goes before using it:

```sh
go run . route -c configs/config.yaml claude-opus-5-5 deepseek/deepseek-v4.1-flash
docker compose exec claudeproxy /claudeproxy route claude-opus-5-5 deepseek/deepseek-v4.1-flash
```

`route` validates the configuration and prints the provider and upstream model.
If `-c` is not given, the configuration is read from `$CLAUDEPROXY_CONFIG`,
then from `~/.config/claudeproxy/config.yaml`.

### 2. Point Claude Code at the proxy

Add the base URL to `~/.claude/settings.json` so every session uses it:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:8787"
  }
}
```

Or export it for a single shell: `export ANTHROPIC_BASE_URL=http://127.0.0.1:8787`.
Claude models keep working exactly as before, with your own login.

### 3. Add a subagent for another model

Keep Opus as the main model and delegate work to a subagent whose `model:` is
a provider model id. Create `~/.claude/agents/deepseek-flash.md`:

```markdown
---
name: deepseek-flash
description: Runs a task on deepseek/deepseek-v4.1-flash through claudeproxy. Use when the user explicitly asks for DeepSeek.
model: deepseek/deepseek-v4.1-flash
---

Do exactly what the task asks, concisely.
```

Restart Claude Code so it loads the agent, then ask for it by name ("run the
deepseek-flash agent to ..."). The `*/*` route sends the request to
OpenRouter, and `flatten_mid_conversation_updates` makes the subagent's
requests acceptable to a non-Claude model (see [Configuration](#configuration)).
Without it, OpenRouter answers `400 Invalid Anthropic Messages API request`.

Other ways to send work to another model:

- **Name the model directly** wherever Claude Code accepts a model id, e.g.
  `ANTHROPIC_DEFAULT_SONNET_MODEL=deepseek/deepseek-v4.1-flash`,
  `ANTHROPIC_DEFAULT_HAIKU_MODEL=...` or `CLAUDE_CODE_SUBAGENT_MODEL=...`.
- **Divert a Claude tier in the proxy.** Add a route such as
  `match: "claude-haiku-*"` with `provider: openrouter` and
  `model: deepseek/deepseek-v4.1-flash`. Claude Code is unchanged and still
  believes it is talking to Haiku.

### Troubleshooting

`docker compose logs claudeproxy` shows one line per request with the
requested model, provider, upstream model and status. When a provider answers
with an error, its response body is logged as `msg="upstream error"`, decoded
if it is gzip, brotli or zstd compressed.

## Configuration

See [configs/config.yaml](configs/config.yaml).

**Routes**
- Routes are evaluated in order and the first match wins.
- `match` is a glob: `*` matches any run of characters, including `/`.
  `regex` takes an RE2 expression.
- The optional `model` field renames the model sent upstream. It can use `$1`
  or `${name}` to insert captured groups.
- `default_provider` receives requests that carry no model (such as
  `/v1/models` or telemetry) and models that no route matches.

**Providers**
- `auth` is required:
  - `passthrough` forwards the client's credentials unchanged. Use it for
    Anthropic only.
  - `bearer` and `x-api-key` remove `Authorization`, `x-api-key` and `Cookie`,
    then send `api_key`.
- `headers` sets extra headers; `drop_headers` removes headers.
- `flatten_mid_conversation_updates: true` rewrites the mid-conversation
  system messages Claude Code sends, which models other than recent Claude
  ones reject: `tool_addition` blocks become entries of `tools`, and a
  message-level `output_config` (reasoning effort) replaces the request one.
- `${ENV}` references are expanded in `api_key`, `base_url` and header values.
  A variable that is not set stops startup.

Unknown keys in the file are rejected at startup.

## Limits

- Providers must speak the Anthropic Messages API. The proxy does not
  translate to the OpenAI format.
- `count_tokens` calls go to the routed provider. Not every provider
  implements that endpoint.

## Layout (hexagonal)

```
cmd/claudeproxy/            cobra: serve, route, config file loading
internal/proxy/init.go      composition root (direct DI)
internal/proxy/domain/      Router, Route (glob/regex + rewrite), Decision
internal/proxy/app/         routeuc.RouteQueries implementation
internal/proxy/inbound/http model extraction, rewrite, dispatch, logging
internal/proxy/outbound/upstream  httputil.ReverseProxy per provider, auth policy
internal/pkg/jsonscan       zero-decode top-level JSON field locator
internal/pkg/apierror       Anthropic-shaped error responses
tests/e2e-api/              end-to-end routing tests with fake providers
```

`make test` runs the tests with the race detector. `make fuzz` fuzzes the
scanner, and `make bench` benchmarks it.

CI ([.github/workflows/docker.yml](.github/workflows/docker.yml)) runs vet and
the tests, then builds the image for linux/amd64 and linux/arm64. Pushes to
`main` publish `ghcr.io/jlugagne/claudeproxy:latest`, `v*` tags publish the
matching semver tags, and pull requests build without pushing.
