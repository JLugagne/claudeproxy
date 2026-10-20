# Security and privacy

claudeproxy is meant to run on your own machine, for your own Claude Code.

- [Network exposure](#network-exposure)
- [Credentials](#credentials)
- [What Anthropic receives](#what-anthropic-receives)
- [What a third-party provider receives](#what-a-third-party-provider-receives)
- [What is stored](#what-is-stored)

## Network exposure

**The proxy has no authentication.** Anyone who can reach it can send
requests billed to your provider keys.

It is therefore local by default:

- `serve` refuses a non-loopback `server.listen` unless
  `--allow-non-loopback` is passed;
- the Docker image listens on all interfaces inside the container, and the
  Compose file and documented `docker run` commands publish the port on
  `127.0.0.1` only.

Do not publish the port on `0.0.0.0` or expose the proxy beyond your machine
without an authenticating reverse proxy in front of it.

## Credentials

- Provider keys come from environment variables referenced as `${VAR}` in the
  configuration. Keep them in `.env` (ignored by git) or your environment, not
  in the configuration file.
- The dashboard reports only whether a provider has a key, never its value.
- Your Anthropic credentials (OAuth token or API key) are forwarded only to
  providers with `auth: passthrough`, which should be Anthropic alone. Every
  other provider gets them removed and receives the provider's own key.

## What Anthropic receives

With `auth: passthrough`, requests are forwarded as Claude Code sent them:
headers, credentials, body bytes. The proxy only:

- removes `X-Project`, its own attribution header;
- removes `Accept-Encoding`, so it can read token usage from the response;
- rewrites the `model` field when a route renames the model.

## What a third-party provider receives

With `auth: bearer` or `auth: x-api-key`, headers are filtered with an
allowlist. Of the headers Claude Code sends, the provider receives only the
protocol ones:

- `Accept`, `Anthropic-Version`, `Content-Encoding` and `Content-Type`.

Everything else is dropped: `Authorization`, `x-api-key`, `Cookie`,
`User-Agent`, `Anthropic-Beta`, `X-Stainless-*`, `X-Claude-Code-Session-Id`,
and any header a future Claude Code version adds. The proxy then sets:

- the provider key (`Authorization: Bearer ...` or `x-api-key`);
- the provider's `headers` from the configuration;
- `X-Opencode-Session` for `format: openai-chat` providers, see below.

In the body, the proxy removes the `metadata` object and the billing
attribution block Claude Code adds to the system prompt, unless
`keep_claude_metadata: true`.

The provider still receives the conversation itself: system prompt, messages,
tool definitions and tool results, including file contents Claude Code read.
Review the provider's data policy before routing work to it.

Providers with `format: openai-chat` receive the Claude Code session id as
`X-Opencode-Session`, as OpenCode Go expects. Add `drop_headers: [X-Opencode-Session]` to a provider that should not get it.

`headers` can set a different `User-Agent` for providers that require one.

## What is stored

The usage database holds one row per Messages API call: time, project,
session, provider, models, token counts, status, timings and cost. It never
holds prompts, responses or credentials. See
[Dashboard](dashboard.md#what-is-recorded).

Logs contain one line per request with the same metadata. Provider error
bodies are logged as `msg="upstream error"`; they may echo parts of the
request.
