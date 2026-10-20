# Dashboard

The proxy serves a usage console at <http://127.0.0.1:8787/dashboard>. It is a
React app embedded in the binary, with six pages: Overview, Projects, Models,
Requests, Sessions and Providers. It has a dark and a light theme, a 24h / 7d /
30d range selector and CSV export.

- [What is recorded](#what-is-recorded)
- [Project and session attribution](#project-and-session-attribution)
- [Cost](#cost)
- [JSON API](#json-api)
- [Disabling](#disabling)

## What is recorded

Each Messages API call (`POST /v1/messages...`) adds one row to the SQLite
database set by [`metrics.path`](configuration.md#metrics):

- time, project, session;
- provider, requested model and upstream model;
- input, output, cache read and cache creation tokens;
- HTTP status, duration, time to first token, and whether it was streamed;
- cost, when the provider reports one.

Prompts, responses and credentials are never stored. Other requests
(connectivity checks, model listings...) are proxied and logged but not
recorded.

Token counts are read from the response, streamed or not. The proxy asks
providers for an uncompressed response so it can read them.

## Project and session attribution

The **project** of a request is, in order:

1. the `X-Project` request header, when the client sends one (see
   [the `SessionStart` hook](installation.md#the-sessionstart-hook));
2. the working directory Claude Code reports in its prompt
   (`Primary working directory: ...`), reduced to its last two segments:
   `~/src/github.com/owner/repo` becomes `owner/repo`;
3. `unknown`.

The **session** is the Claude Code session id carried in the request's
`metadata.user_id`, or `unknown`.

Attribution is metadata only: routing never depends on it, and `X-Project` is
removed before the request is forwarded.

## Cost

When a provider reports a price in its response (OpenRouter does, through
`usage.cost`), it is recorded and shown as spend: in the Overview, per
project, model and session, and per request. A request without a reported
price shows `—`, so a known zero is never confused with "unpriced". The
proxy does not estimate prices.

## JSON API

The console reads read-only JSON under `/dashboard/api/`:

| Endpoint | Returns |
|---|---|
| `GET /dashboard/api/overview?range=24h` | Totals, token volume, top projects and models, recent requests |
| `GET /dashboard/api/projects` | Per-project aggregates and a request series |
| `GET /dashboard/api/models` | Per-model aggregates |
| `GET /dashboard/api/requests` | The request log (`q`, `status`, `stream`, `limit`) |
| `GET /dashboard/api/sessions` | Per-session aggregates |
| `GET /dashboard/api/providers` | Provider usage merged with the configured routes |

`range` is `24h` (default), `7d` or `30d`; chart buckets are hourly for 24h
and daily otherwise.

The configuration is never exposed: the providers endpoint reports only
whether an API key is present, never its value, and there is no settings
page.

`/dashboard` is served by the proxy itself and never forwarded upstream;
every other path reaches the providers untouched.

## Disabling

The console exists only while metrics are enabled. With
`metrics.disabled: true`, `/dashboard` answers `404` and the path never falls
through to a provider, so the console leaves no surface at all.

If metrics are enabled but the database cannot be opened, the console stays
up and shows the reason instead of empty charts.
