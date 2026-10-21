# Dashboard

The proxy serves a usage console at <http://127.0.0.1:8787/dashboard>. It is a
React app embedded in the binary, with seven pages: Overview, Projects, Models,
Requests, Sessions, Quota and Providers. It has a dark and a light theme, a 24h / 7d /
30d range selector and CSV export. Pages refresh live as requests are recorded,
and the header shows a Live indicator while the event stream is connected.

- [What is recorded](#what-is-recorded)
- [Project and session attribution](#project-and-session-attribution)
- [Cost](#cost)
- [Quota](#quota)
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
   (`Primary working directory: ...`), reduced to its repository so that a
   subdirectory or a worktree counts as the same project:
   - after a forge host, the next two segments:
     `~/src/github.com/owner/repo/internal` and
     `~/src/github.com/owner/repo/.claude/worktrees/x` both become
     `owner/repo`;
   - otherwise the last two segments before the first hidden directory:
     `~/work/proj/.claude/worktrees/x` becomes `work/proj`;
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

## Quota

Anthropic reports the Claude subscription quota in the `anthropic-ratelimit-*`
headers of every response. The proxy stores them, one row per request, in the
`ratelimit` table of the same database, so the history survives restarts. They
are the only response headers kept; credentials never are.

The **Quota** page charts the utilization of the 5-hour and weekly windows over
the selected range and shows which window is limiting. Each rise of a window's
utilization is charged to the project and session of the request that observed
it, which tells you where the quota goes. The split is approximate when sessions
run concurrently, and a drop (a window reset) is not charged. The sidebar shows
both windows as progress bars on every page.

Providers other than Anthropic send no such headers, so they add no rows.

## JSON API

The console reads read-only JSON under `/dashboard/api/`:

| Endpoint | Returns |
|---|---|
| `GET /dashboard/api/overview?range=24h` | Totals, token volume, top projects and models, recent requests |
| `GET /dashboard/api/projects` | Per-project aggregates and a request series |
| `GET /dashboard/api/models` | Per-model aggregates |
| `GET /dashboard/api/requests` | The request log (`q`, `status`, `stream`, `limit`) |
| `GET /dashboard/api/sessions` | Per-session aggregates |
| `GET /dashboard/api/quota` | Subscription quota: current 5h and weekly windows, utilization series, consumption per project and session |
| `GET /dashboard/api/providers` | Provider usage merged with the configured routes |

`range` is `24h` (default), `7d` or `30d`; chart buckets are hourly for 24h
and daily otherwise.

The console also listens to `GET /dashboard/api/events`, a Server-Sent Events
stream:

- it opens with a `: connected` comment and sends a `: ping` heartbeat comment
  every 25 seconds;
- each recorded request arrives as a `request` event whose `data` is a JSON
  object with the same fields as a `/dashboard/api/requests` row;
- events are not buffered: they are published only to connected listeners and
  dropped when nobody listens, and a slow listener drops events rather than
  slowing the proxy;
- the stream answers `404` when live events are unavailable.

The console refetches its data on each event, throttled to one refetch every
2 seconds, and after a reconnect (the browser reopens the stream itself; events
emitted while it was down are lost).

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
