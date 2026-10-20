# Installation

Installing takes two steps: run the proxy, then point Claude Code at it.

- [1. Run the proxy](#1-run-the-proxy)
  - [Docker Compose](#docker-compose)
  - [Prebuilt image](#prebuilt-image)
  - [From source](#from-source)
- [2. Connect Claude Code](#2-connect-claude-code)
  - [With `setup`](#with-setup)
  - [By hand](#by-hand)
  - [The `SessionStart` hook](#the-sessionstart-hook)
- [3. Check it works](#3-check-it-works)
- [Uninstall](#uninstall)

## 1. Run the proxy

### Docker Compose

From a clone of this repository:

```sh
cp .env.example .env    # optional, see the variables below
docker compose up -d --build
```

The proxy listens on `127.0.0.1:8787` and is not reachable from other hosts.
Out of the box it uses
[configs/config.yaml.example](../configs/config.yaml.example), which routes
Claude models to Anthropic and needs no API key.

To use your own configuration:

```sh
cp configs/config.yaml.example configs/config.yaml   # ignored by git
# edit configs/config.yaml: uncomment providers and routes
echo 'CLAUDEPROXY_CONFIG_FILE=./configs/config.yaml' >> .env
echo 'OPENCODE_API_KEY=...' >> .env                   # keys of the providers you enabled
docker compose up -d
```

Restart the container after each change to the file:
`docker compose restart claudeproxy`.

Variables read by [compose.yaml](../compose.yaml), from `.env` or the
environment:

| Variable | Default | Purpose |
|---|---|---|
| `CLAUDEPROXY_CONFIG_FILE` | `./configs/config.yaml.example` | Configuration file mounted read-only into the container |
| `CLAUDEPROXY_PORT` | `8787` | Host port, bound on `127.0.0.1` |
| `CLAUDEPROXY_UID` / `CLAUDEPROXY_GID` | `1000` | User the container runs as, so `./data` stays owned by you |
| `OPENCODE_API_KEY`, `OPENROUTER_API_KEY` | empty | Provider keys, referenced as `${VAR}` in the configuration |

The usage database is written to `./data/usage.db` on the host. The container
runs with a read-only root filesystem, no capabilities and
`no-new-privileges`.

### Prebuilt image

Images for linux/amd64 and linux/arm64 are published to GHCR:

```sh
docker run -d --name claudeproxy --restart unless-stopped \
  -p 127.0.0.1:8787:8787 \
  ghcr.io/jlugagne/claudeproxy:latest
```

The image ships with `configs/config.yaml.example`. To use your own
configuration, mount it and pass the keys of the providers it enables:

```sh
docker run -d --name claudeproxy --restart unless-stopped \
  -p 127.0.0.1:8787:8787 \
  -e OPENCODE_API_KEY=... \
  -v "$PWD/config.yaml:/etc/claudeproxy/config.yaml:ro" \
  -v "$PWD/data:/var/lib/claudeproxy" \
  --user "$(id -u):$(id -g)" \
  ghcr.io/jlugagne/claudeproxy:latest
```

Without the `data` volume the proxy still runs, but usage metrics are
disabled and the dashboard reports why.

Keep `127.0.0.1:` in `-p`: inside the container the proxy listens on all
interfaces, and the port mapping is what keeps it local. See
[Security and privacy](security.md).

### From source

Requires Go (see [go.mod](../go.mod) for the version).

```sh
cp configs/config.yaml.example configs/config.yaml
go run . serve -c configs/config.yaml          # listens on 127.0.0.1:8787
```

Or build a binary and install the configuration where it is looked up by
default:

```sh
make build                                     # bin/claudeproxy, needs Node for the console
# or: go build -o bin/claudeproxy .            # uses the committed console build
mkdir -p ~/.config/claudeproxy
cp configs/config.yaml.example ~/.config/claudeproxy/config.yaml
bin/claudeproxy serve
```

The configuration is read from `-c`, then `$CLAUDEPROXY_CONFIG`, then
`claudeproxy/config.yaml` in the user configuration directory
(`~/.config` on Linux, `~/Library/Application Support` on macOS).

The example sets `metrics.path` to `/var/lib/claudeproxy/usage.db`, the path
used inside the container. Outside Docker, remove that line to store usage in
`claudeproxy/usage.db` in the user configuration directory, or set a path you
can write to. Otherwise metrics are disabled and the dashboard says why.

`serve` refuses to bind an address other than loopback unless
`--allow-non-loopback` is passed; `--listen` overrides `server.listen`.

## 2. Connect Claude Code

Claude Code sends its API requests to `ANTHROPIC_BASE_URL`. Set it to the
proxy address.

### With `setup`

`setup` writes the base URL into `~/.claude/settings.json` and installs the
[`SessionStart` hook](#the-sessionstart-hook). It is idempotent and keeps your
other settings.

```sh
go run . setup --base-url http://127.0.0.1:8787
```

| Flag | Default | Purpose |
|---|---|---|
| `--base-url` | derived from `server.listen` in the configuration | URL written to `ANTHROPIC_BASE_URL` |
| `--claude-dir` | `~/.claude` | Claude Code configuration directory |

Without `--base-url`, `setup` reads the configuration (`-c`,
`$CLAUDEPROXY_CONFIG` or the default path) and fails if there is none. Pass
the URL explicitly when the proxy runs in Docker, with the host port you
chose in `CLAUDEPROXY_PORT`.

Run `setup` on the host, not inside the container: the container has its own
filesystem and cannot write to your `~/.claude`.

Restart Claude Code to apply the change.

### By hand

Add the base URL to `~/.claude/settings.json` so every session uses it:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://127.0.0.1:8787"
  }
}
```

Or export it for a single shell before starting Claude Code:

```sh
export ANTHROPIC_BASE_URL=http://127.0.0.1:8787
```

Claude models keep working exactly as before, with your own login.

### The `SessionStart` hook

The hook is optional. It names the project of a session from its git remote
(`owner/repo`), else from the repository directory name, else from the working
directory, and exports it as an `X-Project` header through
`ANTHROPIC_CUSTOM_HEADERS`.

The proxy does not depend on it. When no `X-Project` header arrives, it takes
the project from the working directory Claude Code reports in its prompt
(the last two path segments, e.g. `owner/repo` for
`~/src/github.com/owner/repo`). Claude Code applies the variables a
`SessionStart` hook exports to the commands of its Bash tool, so in practice
the prompt is the usual source. See
[project and session attribution](dashboard.md#project-and-session-attribution).

To install the hook by hand, save the script as
`~/.claude/claudeproxy/x-project.sh`:

```sh
#!/bin/sh
[ -n "$CLAUDE_ENV_FILE" ] || exit 0
dir="${CLAUDE_PROJECT_DIR:-$PWD}"
proj=$(git -C "$dir" remote get-url origin 2>/dev/null | sed -E 's#\.git$##; s#.*[:/]([^/]+/[^/]+)$#\1#')
proj=${proj:-$(basename "$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null)")}
proj=${proj:-$dir}
printf 'export ANTHROPIC_CUSTOM_HEADERS="X-Project: %s"\n' "$proj" >> "$CLAUDE_ENV_FILE"
```

Make it executable:

```sh
chmod 700 ~/.claude/claudeproxy/x-project.sh
```

Then register it in `~/.claude/settings.json`, next to your other hooks. Use
the absolute path; `~` is not expanded:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/home/you/.claude/claudeproxy/x-project.sh"
          }
        ]
      }
    ]
  }
}
```

To set the project for every request regardless of the prompt, export the
header before starting Claude Code:

```sh
ANTHROPIC_CUSTOM_HEADERS="X-Project: my-project" claude
```

The proxy removes `X-Project` before forwarding, so it never reaches a
provider.

## 3. Check it works

Check where each model goes:

```sh
docker compose exec claudeproxy /claudeproxy route claude-opus-5-5 qwen3.8-flash
go run . route -c configs/config.yaml claude-opus-5-5 qwen3.8-flash
```

`route` validates the configuration and prints, for each model, the provider
and the model sent upstream.

Start Claude Code, send a message, then:

- `docker compose logs claudeproxy` shows one line per request with the model,
  provider, upstream model, status, project and session;
- the [dashboard](dashboard.md) at <http://127.0.0.1:8787/dashboard> shows the
  request.

## Uninstall

1. Remove `ANTHROPIC_BASE_URL` from the `env` object of
   `~/.claude/settings.json`, and the `SessionStart` entry whose command is
   `.../claudeproxy/x-project.sh`.
2. Delete `~/.claude/claudeproxy/`.
3. Stop the proxy: `docker compose down`, or `docker rm -f claudeproxy`.
4. Delete the usage database if you do not need it: `./data/usage.db*` with
   Docker Compose, or `claudeproxy/usage.db*` in the user configuration
   directory otherwise.
