# Troubleshooting

## Logs

The proxy logs one line per request with the method, path, requested model,
provider, upstream model, project, session and status:

```sh
docker compose logs -f claudeproxy
```

When a provider answers with an error, its response body is logged as
`msg="upstream error"`, decoded if it is gzip, brotli or zstd compressed.
Set `server.log_level: debug` for more detail, and `log_format: json` to feed
the logs to another tool.

Check where a model goes without sending a request:

```sh
docker compose exec claudeproxy /claudeproxy route my-model
```

## Common problems

**The proxy does not start: `environment variable not set`.**
An enabled provider references a variable that is not defined. Define it in
`.env` or the environment, or comment the provider out. See
[Environment variables](configuration.md#environment-variables).

**The proxy does not start: `is not a loopback address`.**
`server.listen` is not on `127.0.0.1`, `::1` or `localhost`. Pass
`--allow-non-loopback` only if you understand the
[exposure](security.md#network-exposure).

**The proxy does not start: `field ... not found in type ...`.**
The configuration has a key the proxy does not know, often a typo. The error
gives its name and line.

**`docker compose up` fails on the configuration mount.**
`CLAUDEPROXY_CONFIG_FILE` points to a file that does not exist. Docker then
creates a directory in its place: delete it, create the file, and start
again.

**Claude Code does not go through the proxy.**
Nothing appears in the logs. Check that `ANTHROPIC_BASE_URL` is set in
`~/.claude/settings.json` or the environment, and restart Claude Code. With a
custom `CLAUDEPROXY_PORT`, the URL must use that port.

**`no route` in the logs, and the request fails.**
No route matches the model and `default_provider` is empty. Add a route or
set `default_provider`.

**A non-Claude model answers `400 Invalid Anthropic Messages API request`.**
Claude Code sent a mid-conversation system message the model rejects. Set
`flatten_mid_conversation_updates: true` on the provider.

**A provider rejects a header or a beta.**
Remove it with `drop_headers` on that provider.

**The dashboard says metrics are disabled or the database cannot be opened.**
`metrics.path` is not writable. With Docker Compose, create `./data` and make
sure `CLAUDEPROXY_UID` / `CLAUDEPROXY_GID` match its owner. Outside Docker,
remove the example's `/var/lib/claudeproxy` path or choose a writable one.

**Requests show project `unknown`.**
The request had no `X-Project` header and its prompt did not carry a working
directory. See [project and session attribution](dashboard.md#project-and-session-attribution).

**`count_tokens` fails for a provider.**
Not every provider implements `/v1/messages/count_tokens`. Providers with
`format: openai-chat` are answered locally with an estimate.
