# Development

## Layout

The code follows a hexagonal (ports and adapters) layout with direct
dependency injection, checked by [go-arch-lint](../.go-arch-lint.yml).

```
main.go                                 entry point
cmd/claudeproxy/                        cobra commands: serve, route, setup; configuration loading
internal/proxy/init.go                  composition root
internal/proxy/config/                  "proxy" and "metrics" configuration sections
internal/proxy/domain/                  Router, Route (glob/regex + rewrite), Decision, usage types
internal/proxy/domain/usecases/         ports: route and usage use cases, with test mocks
internal/proxy/app/                     use case implementations
internal/proxy/inbound/http/            model extraction, rewrite, dispatch, logging, usage capture
internal/proxy/inbound/dashboard/       embedded console and read-only /dashboard/api JSON
internal/proxy/outbound/upstream/       httputil.ReverseProxy per provider, auth policy, OpenAI translation
internal/proxy/outbound/usage/          SQLite usage store: recording and metrics queries
internal/pkg/config/                    "server" configuration section
internal/pkg/jsonscan/                  zero-decode top-level JSON field locator
internal/pkg/apierror/                  Anthropic-shaped error responses
internal/pkg/setup/                     the setup command: the base URL in settings.json
web/                                    React sources of the console (Vite, TypeScript)
tests/e2e-api/                          end-to-end routing tests with fake providers
```

## Make targets

| Target | Does |
|---|---|
| `make test` | Runs the tests with the race detector |
| `make lint` | `go vet` and `go-arch-lint check` |
| `make build` | Builds the console, then `bin/claudeproxy` |
| `make run` | Builds the console, then runs `serve -c configs/config.yaml` |
| `make dashboard` | Builds the console only |
| `make fuzz` | Fuzzes the JSON scanner for 30 s |
| `make bench` | Benchmarks the JSON scanner |
| `make tidy` | `go mod tidy` |

`make run` expects `configs/config.yaml`: copy it from
`configs/config.yaml.example` first.

## The console

The console sources live in [web/](../web/). Vite builds them straight into
the Go embed directory, `internal/proxy/inbound/dashboard/dist`, so the UI in
the binary always matches `web/`:

- `make build` and `make run` rebuild it first;
- the Dockerfile builds it in a Node stage, then embeds it in the Go stage;
- `dist/` is committed, so a bare `go build` and `make test` work without
  Node.

Commit the rebuilt `dist/` with any change to `web/`.

## CI and releases

[.github/workflows/docker.yml](../.github/workflows/docker.yml) runs on every
push and pull request:

1. `go vet` and the tests with the race detector;
2. the image build for linux/amd64 and linux/arm64.

Pushes to `main` publish `ghcr.io/jlugagne/claudeproxy:latest` and a
`sha-<commit>` tag. A `v*` tag publishes the matching semver tags
(`1.2.3`, `1.2`). Pull requests build without publishing.
