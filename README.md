# claudeproxy

An HTTP proxy that sits between Claude Code and its providers.
It sends each request to a provider based on the model name in the request:

- **Anthropic models** are forwarded untouched. Headers, OAuth token, body bytes
  and SSE stream all pass through as they are.
- **Other models** are routed to an Anthropic-compatible provider such as
  OpenCode Go (`https://opencode.ai/inference/go/anthropic`). The proxy uses its
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
cp .env.example .env    # set OPENCODE_API_KEY (and CLAUDEPROXY_PORT if 8787 is taken)
docker compose up -d --build
```

Or with the prebuilt image (linux/amd64 and linux/arm64), published to GHCR by
the [docker workflow](.github/workflows/docker.yml):

```sh
docker run -d --name claudeproxy --restart unless-stopped \
  -p 127.0.0.1:8787:8787 \
  -e OPENCODE_API_KEY=... \
  -v "$PWD/configs/config.yaml:/etc/claudeproxy/config.yaml:ro" \
  ghcr.io/jlugagne/claudeproxy:latest
```

The image ships with [configs/config.yaml](configs/config.yaml); the volume is
only needed to use your own. Restart the container after you edit the file.
The proxy is published on `127.0.0.1` only.

Or from source:

```sh
export OPENCODE_API_KEY=...
go run . serve -c configs/config.yaml          # listens on 127.0.0.1:8787
```

Check where each model goes before using it:

```sh
go run . route -c configs/config.yaml claude-opus-5-5 qwen3.8-flash
docker compose exec claudeproxy /claudeproxy route claude-opus-5-5 qwen3.8-flash
```

`route` validates the configuration and prints the provider and upstream model.
If `-c` is not given, the configuration is read from `$CLAUDEPROXY_CONFIG`,
then from `~/.config/claudeproxy/config.yaml`.

### 2. Point Claude Code at the proxy

Point Claude Code at the proxy. `setup` does it for you:

```sh
go run . setup                 # or: docker compose exec claudeproxy /claudeproxy setup
```

`setup` writes `ANTHROPIC_BASE_URL` into `~/.claude/settings.json` (derived from
`server.listen`) and installs a `SessionStart` hook that exports the `X-Project`
header (`git` `owner/repo`, else the project path) so usage metrics are
attributed per project. It is idempotent and preserves your other settings;
`--claude-dir` and `--base-url` override the defaults.

Or do it by hand — add the base URL to `~/.claude/settings.json` so every session uses it:

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
a provider model id. Create `~/.claude/agents/qwen-flash.md`:

```markdown
---
name: qwen-flash
description: Runs a task on qwen3.8-flash through claudeproxy. Use when the user explicitly asks for Qwen.
model: qwen3.8-flash
---

Do exactly what the task asks, concisely.
```

Restart Claude Code so it loads the agent, then ask for it by name ("run the
qwen-flash agent to ..."). The catch-all `*` route sends the request to
OpenCode Go, and `flatten_mid_conversation_updates` makes the subagent's
requests acceptable to a non-Claude model (see [Configuration](#configuration)).
Without it, the provider may answer `400 Invalid Anthropic Messages API request`.

Other ways to send work to another model:

- **Name the model directly** wherever Claude Code accepts a model id, e.g.
  `ANTHROPIC_DEFAULT_SONNET_MODEL=qwen3.8-flash`,
  `ANTHROPIC_DEFAULT_HAIKU_MODEL=...` or `CLAUDE_CODE_SUBAGENT_MODEL=...`.
- **Divert a Claude tier in the proxy.** Add a route such as
  `match: "claude-haiku-*"` with `provider: opencode-go` and
  `model: qwen3.8-flash`. Claude Code is unchanged and still
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

## Dashboard

The proxy serves a usage console at
[http://127.0.0.1:8787/dashboard](http://127.0.0.1:8787/dashboard). It is a
React app embedded in the binary: Overview, Projects, Models, Requests,
Sessions and Providers, with a dark and a light theme.

The source lives in [web/](web/) and is built by Vite straight into the Go
embed directory, `internal/proxy/inbound/dashboard/dist`. `make run` and
`make build` rebuild it first, so the UI embedded in the binary always matches
`web/` — there is no separate step. The Dockerfile does the same in a single
file: a Node stage builds the console, then the Go builder embeds it. `dist/`
is also committed, so a bare `go build` and `make test` work without Node.

`/dashboard` is served by the composition root and never forwarded upstream;
every other path reaches the proxy untouched.

The console reads the **usage SQLite database** (metrics are on by default; see
`metrics` in [configs/config.yaml](configs/config.yaml)). It exposes read-only
JSON under `/dashboard/api/`:

| endpoint | returns |
|---|---|
| `GET /dashboard/api/overview?range=24h` | totals, token volume, top projects and models, recent requests |
| `GET /dashboard/api/projects` | per-project aggregates and a request series |
| `GET /dashboard/api/models` | per-model aggregates |
| `GET /dashboard/api/requests` | the request log (`q`, `status`, `stream`, `limit`) |
| `GET /dashboard/api/sessions` | per-session aggregates |
| `GET /dashboard/api/providers` | provider usage merged with the configured routes |

`range` is `24h` (default), `7d` or `30d`; the chart buckets are hourly for
24h and daily otherwise.

**Disabling.** The console exists only while metrics are enabled. With
`metrics.disabled: true`, `/dashboard` answers `404` and the path never falls
through to the proxy, so the console leaves no surface at all. If metrics are
enabled but the database cannot be opened, the console stays up and reports the
reason instead of drawing empty charts.

**Cost.** When a provider reports a price in the response (OpenRouter does,
through `usage.cost`), it is recorded in `cost_usd` and shown as spend — in the
Overview, per project/model/session and per request. A request whose provider
reported no price shows `—`, so a known zero is never confused with "unpriced".
The configuration is never exposed: the providers endpoint reports only
whether an API key is present, never its value, and there is no settings page.

## Limits

- Providers speak the Anthropic Messages API, or set `format: openai-chat` to
  be reached through a translation to OpenAI chat completions. The translation
  covers text, images, tool calls and streaming; reasoning content and
  `thinking` blocks are dropped, and `count_tokens` is answered locally with an
  estimate (about 4 bytes per token).
- `count_tokens` calls go to the routed provider. Not every provider
  implements that endpoint.

## Layout (hexagonal)

```
cmd/claudeproxy/            cobra: serve, route, config file loading
internal/proxy/init.go      composition root (direct DI)
internal/proxy/domain/      Router, Route (glob/regex + rewrite), Decision
internal/proxy/app/         routeuc.RouteQueries implementation
internal/proxy/inbound/http model extraction, rewrite, dispatch, logging
internal/proxy/inbound/dashboard embedded console + read-only /dashboard/api JSON
web/                        React sources for the console (Vite, TypeScript)
internal/proxy/outbound/upstream  httputil.ReverseProxy per provider, auth policy
internal/proxy/outbound/usage     SQLite usage store: recording and the metrics read side
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
