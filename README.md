# claudeproxy

> claudeproxy is an independent project, not affiliated with, endorsed or
> sponsored by Anthropic. Claude and Claude Code are trademarks of Anthropic, PBC.

An HTTP proxy that sits between Claude Code and its providers.
It sends each request to a provider based on the model name in the request:

- **Anthropic models** are forwarded as they are: headers, OAuth token, body
  bytes and SSE stream. Your Claude login keeps working unchanged.
- **Other models** are routed to an Anthropic-compatible provider such as
  OpenCode Go or OpenRouter, or to an OpenAI chat-completions provider through
  a built-in translation. The proxy uses its own API key for these, and the
  client's Anthropic credentials and Claude identifiers are always removed.
- **Usage** is recorded per project, session and model, and shown in an
  embedded dashboard.

Overhead is kept low:

- Only `POST /v1/messages*` bodies are buffered.
- The `model` field is located by a single-pass scanner, with no JSON decoding
  and 1 allocation. Claude Code sends `model` first, so in practice the
  scanner reads only a few bytes.
- The body is re-spliced only when a route renames the model.
- Responses are streamed and flushed immediately.

## Quick start

```sh
git clone https://github.com/JLugagne/claudeproxy.git && cd claudeproxy
docker compose up -d --build                       # listens on 127.0.0.1:8787
go run . setup --base-url http://127.0.0.1:8787    # points Claude Code at the proxy
```

Without Go, add `"env": {"ANTHROPIC_BASE_URL": "http://127.0.0.1:8787"}` to
`~/.claude/settings.json` instead of running `setup`.

Out of the box only Claude models are routed, to Anthropic. To use other
models, enable a provider in your configuration: see
[Configuration](docs/configuration.md) and [Using other models](docs/other-models.md).
The usage console is at <http://127.0.0.1:8787/dashboard>.

## Documentation

| Guide | Contents |
|---|---|
| [Installation](docs/installation.md) | Docker Compose, prebuilt image, from source, macOS, connecting Claude Code, uninstalling |
| [Configuration](docs/configuration.md) | File location, every setting, providers, routes, provider examples |
| [Using other models](docs/other-models.md) | Subagents, model environment variables, diverting a Claude tier, OpenAI translation |
| [Dashboard](docs/dashboard.md) | Usage console, project and session attribution, JSON API, cost |
| [Security and privacy](docs/security.md) | Network exposure, credentials, what is stripped, what is stored |
| [Troubleshooting](docs/troubleshooting.md) | Logs and common errors |
| [Development](docs/development.md) | Code layout, make targets, building the console, CI and releases |

## License

Licensed under the [Apache License 2.0](LICENSE).
