# OpsPilot

OpsPilot is a Go monorepo for operating a small fleet of Linux hosts: a control
plane that owns the command queue, agents that execute bounded, registered tools
on each host, and an MCP server that exposes the whole thing to an AI assistant
without ever handing it a shell.

| Binary | Package | What it does |
| ------ | ------- | ------------ |
| `opspilot-central` | `cmd/central` | Control plane: JSON HTTP API, PostgreSQL, command queue, alerts, registration-token CLI |
| `opspilot-agent` | `cmd/agent` | Runs on each managed host: registers, heartbeats, reports health, executes leased commands |
| `opspilot-mcp` | `cmd/mcp` | MCP stdio server exposing central to an assistant, mode-gated and fail-closed |
| `opspilot-migrate` | `cmd/migrate` | Standalone migration CLI (`up`, `status`) |

```mermaid
flowchart LR
    subgraph Host
        A[opspilot-agent]
    end
    subgraph "Control plane"
        C[opspilot-central]
        P[(PostgreSQL)]
        M[opspilot-mcp]
    end
    A -- "HTTPS/JSON, HMAC-signed" --> C
    C -- SQL --> P
    M -- SQL --> P
    H[AI assistant] -- "MCP stdio" --> M
```

Design principles worth knowing before you configure anything:

- **The agent never runs arbitrary shell.** Every action is a registered tool
  with a JSON Schema, a confirmation level, and an execution-policy gate.
- **Mutations are fail-closed.** A command for an unknown or disabled capability
  is rejected before the row is written; write tools stay `pending` until an
  operator approves them, and commands created through MCP are *never*
  self-approved.
- **Secrets never travel in the clear.** Registration tokens are stored as HMAC
  hashes; every post-registration agent request is HMAC-signed with a per-agent
  signing key issued once at registration.

Deeper documentation: [`docs/architecture.md`](docs/architecture.md),
[`docs/implementation.md`](docs/implementation.md),
[`docs/roadmap.md`](docs/roadmap.md), [`docs/adr/`](docs/adr).

---

## Requirements

- Linux (amd64 or arm64) for the installers; the code builds on macOS for
  development, but the `system.*` tools are Linux-only.
- PostgreSQL 16 (not installed or managed by OpsPilot).
- `curl`, `systemd` and `openssl` on any host you install onto.
- Go 1.25+ **on your development machine only** — the servers never build from
  source, they install a released binary (see below).

---

## Deployment model: release first, then install

OpsPilot is deployed from GitHub Releases. Nothing is compiled on the central or
agent hosts — the installers pull a prebuilt binary.

```mermaid
flowchart LR
    D["dev machine<br/>git tag v0.2.5"] -- "push --tags" --> W["GitHub Actions<br/>release.yml"]
    W -- "6 assets" --> R[("GitHub Releases")]
    R -- "curl latest/download" --> C["central host<br/>install-central.sh"]
    R -- "curl latest/download" --> A["agent host<br/>install.sh"]
```

### 1. Cut a release (development machine)

```bash
# Bump the version constants if this release changes them.
$EDITOR pkg/version/version.go     # Central / MCP version strings

go test ./...                      # release gate — must be green
git tag v0.2.5
git push origin v0.2.5
```

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which builds with
`CGO_ENABLED=0 -trimpath -ldflags="-s -w"` and publishes six assets:

```
opspilot-central-linux-amd64    opspilot-central-linux-arm64
opspilot-agent-linux-amd64      opspilot-agent-linux-arm64
opspilot-mcp-linux-amd64        opspilot-mcp-linux-arm64
```

Wait for the workflow to finish before installing — the installers fetch
`releases/latest/download/<asset>` and will otherwise pick up the previous
release.

### 2. Get the installer onto the target host

The installer scripts are **not** release assets, so fetch them separately —
either straight from the repository:

```bash
curl -fsSL -o install-central.sh \
  https://raw.githubusercontent.com/tsee9iii/opspilot/main/scripts/install-central.sh
chmod +x install-central.sh
```

or by copying them from a checkout (`scp scripts/install-central.sh host:`).
Use `scripts/install.sh` for an agent host. Nothing else from the repository is
needed on the server.

### 3. Upgrading

Re-running an installer after a new release reinstalls the binary and the unit
file and restarts the service. It is idempotent: an existing
`/etc/opspilot/central.yaml` is never overwritten, and an already-registered
agent is never re-registered unless you explicitly answer `y`. Central applies
any pending migrations automatically on startup.

> The installers always take the **latest** release — they cannot pin a version.
> To roll back, download the specific asset from the older release and replace
> `/usr/local/bin/opspilot-<component>` by hand, then restart the service.

---

## OpsPilot Central — setup guide

Central is the control plane. It owns the database, the command queue, the alert
evaluator, and the operator API.

### 1. Provide PostgreSQL

OpsPilot never installs or manages PostgreSQL. Create a database and a role that
owns the schema before installing:

```sql
CREATE ROLE opspilot LOGIN PASSWORD '<strong-password>';
CREATE DATABASE opspilot OWNER opspilot;
```

### 2. Install the released binary

On the central host, with the installer fetched as described above:

```bash
sudo ./install-central.sh
```

It downloads `opspilot-central-linux-<arch>` from the latest GitHub Release,
verifies the file is a non-empty Linux ELF binary, installs it to
`/usr/local/bin/opspilot-central`, creates the `opspilot` system user, writes a
`/etc/opspilot/central.yaml` template (mode `0600`, only when the file does not
already exist), and installs a hardened systemd unit (`NoNewPrivileges`,
`PrivateTmp`, `ProtectHome`, `ProtectSystem=strict` with only `/etc/opspilot`
writable). It then polls `http://127.0.0.1:8080/healthz` for up to 15 seconds
and warns — never fails — if central is not up yet.

The installer deliberately does **not** install PostgreSQL, create a database,
run migrations, or generate registration tokens.

### 3. Configure

Configuration resolves in precedence order: **built-in defaults → YAML file →
environment variables** (env always wins). The YAML file is
`/etc/opspilot/central.yaml`, overridable with `OPSPILOT_CONFIG`. A missing file
is not an error; an unreadable or invalid one is.

```yaml
server:
  host: 127.0.0.1        # must NOT be 0.0.0.0 in production
  port: 8080

database:
  host: localhost
  port: 5432
  database: opspilot
  username: opspilot
  password: <strong-password>
  sslmode: require       # must NOT be 'disable' in production

logger:
  level: info

auth:
  server_secret: <32+ random bytes>   # hashes registration tokens
  operator_token: <32+ random bytes>  # bearer token for operator routes

commands:
  lease_ttl_seconds: 60

alerts:
  enabled: true
  interval_seconds: 60
  agent_offline:
    enabled: true
    severity: critical
    max_offline_seconds: 300
  disk_usage:
    enabled: true
    severity: warning
    threshold_percent: 90
  health_report_stale:
    enabled: true
    severity: warning
    max_report_age_seconds: 600
  project_unhealthy:
    enabled: true
    severity: critical

webhook:
  enabled: false
  url: https://example.com/hooks/opspilot
  secret: <32+ random bytes>
  timeout_seconds: 5
```

Every key has an environment counterpart (`OPSPILOT_HTTP_PORT`,
`OPSPILOT_DB_PASSWORD`, `OPSPILOT_AUTH_SERVER_SECRET`,
`OPSPILOT_OPERATOR_TOKEN`, `OPSPILOT_ALERTS_ENABLED`,
`OPSPILOT_WEBHOOK_URL`, …). See implementation §9 for the full list.

> **Production is fail-closed.** With `OPSPILOT_ENV=production`, central refuses
> to start if `auth.server_secret`, `auth.operator_token` or the database
> password is missing or left at its development default, if `sslmode` is
> `disable`, if the server binds `0.0.0.0`, or if webhooks are enabled without an
> `https://` URL and a secret. Validation errors name the offending variable —
> never its value.

### 4. Migrate

**Nothing to do.** Migrations are embedded in the `opspilot-central` binary and
run automatically at startup, after database connectivity is verified and before
the HTTP server binds. Installing a newer release therefore migrates the schema
on the next service start. If a migration fails, central refuses to start with a
`bootstrap: run migrations:` error. There is no rollback.

A standalone `cmd/migrate` CLI (`up`, `status`) exists for development, but it
is **not** published as a release asset — build it locally if you need it:

```bash
go build -o bin/migrate ./cmd/migrate && ./bin/migrate status
```

### 5. Start and verify

```bash
sudo systemctl restart opspilot-central
systemctl status opspilot-central
curl -fsS http://127.0.0.1:8080/healthz     # -> ok
```

### 6. Issue a registration token

The CLI is the only supported interface — there is no token HTTP endpoint and
no web UI. The plaintext token is printed **exactly once**; only its
HMAC-SHA256 hash is stored.

```bash
opspilot-central token create --environment production --expires 30d
opspilot-central token list       # never shows token values
opspilot-central token revoke <token-id>
```

`--expires` accepts a Go duration (`24h`, `90s`) or whole days (`7d`, `30d`);
the default lifetime is 30 days. Registration consumes a token atomically and
deletes the row — a replay returns `409 token_already_used`.

### 7. Operator API

Operator routes need **both** headers:

```bash
curl -X POST https://central.example.com/api/v1/commands \
  -H "Authorization: Bearer $OPSPILOT_OPERATOR_TOKEN" \
  -H "X-Operator-Actor: alice@example.com" \
  -H 'Content-Type: application/json' \
  -d '{"agent_id":"<uuid>","tool":"system.uptime","payload":{}}'
```

`X-Operator-Actor` (1–128 chars) is persisted on the affected record, so
approval and acknowledge chains are attributable. Write tools return a command
in `confirmation_status: pending` — release it with:

```bash
curl -X POST https://central.example.com/api/v1/commands/approve \
  -H "Authorization: Bearer $OPSPILOT_OPERATOR_TOKEN" \
  -H "X-Operator-Actor: alice@example.com" \
  -H 'Content-Type: application/json' \
  -d '{"command_id":"<uuid>","approval_note":"deploy window 14:00"}'
```

Other operator routes: `GET /api/v1/commands/{id}`, `GET /api/v1/health`,
`GET /api/v1/alerts`, `POST /api/v1/alerts/{id}/acknowledge`.

---

## OpsPilot Agent — setup guide

The agent runs on each managed host. It registers once, then heartbeats, reports
health, advertises its capabilities, and leases and executes commands.

### 1. Install and register

On the agent host, with `install.sh` fetched as described above:

```bash
sudo ./install.sh
```

The installer is interactive and idempotent. It prompts for the **Central URL**
and a **Registration Token**, then:

1. downloads `opspilot-agent-linux-<arch>` from the latest GitHub Release,
   verifies it is a Linux ELF binary, and installs it to
   `/usr/local/bin/opspilot-agent`;
2. generates a random secret, registers with
   `POST /api/v1/agents/register`, and persists `agent_id` + `signing_key` into
   `/etc/opspilot/agent.yaml` (mode `0600`);
3. creates the `opspilot` system user and the systemd unit
   (`NoNewPrivileges`, `PrivateTmp` — the stricter `ProtectSystem`/`ProtectHome`
   set central uses is deliberately omitted, because the agent must write
   project repositories and reach the Docker/PM2 sockets);
4. verifies the install by sending HMAC-signed heartbeat and lease requests.

If the host is already registered, it asks `Re-register this agent? (y/N)` and
defaults to **No**, preserving the existing credentials — that is the upgrade
path: re-run the installer after a release and only the binary and unit file are
replaced. Re-registering consumes a fresh token and rotates the signing key.
Secrets and tokens are never printed or logged.

To install a locally built binary instead of a release asset — useful when
testing a change before tagging — point the installer at it:

```bash
GOOS=linux GOARCH=amd64 go build -o opspilot-agent ./cmd/agent   # dev machine
sudo OPSPILOT_LOCAL_BIN=./opspilot-agent ./install.sh            # agent host
```

`install-central.sh` has no such override; central always installs from the
release.

### 2. Configure

`/etc/opspilot/agent.yaml`; the path can be overridden with
`OPSPILOT_AGENT_CONFIG`. A fully commented template lives at
[`configs/agent.example.yaml`](configs/agent.example.yaml).

```yaml
central_url: https://central.example.com
registration_token: ""      # consumed at registration
secret: ""                  # installer-generated
signing_key: ""             # issued by central, signs every request
agent_id: ""                # issued by central
version: "0.1.0"

server:
  hostname: web-01
  environment: production

sse_enabled: true           # instant wake-ups; poll_interval becomes a fallback
poll_interval: 30           # seconds; lower to 2-5 if sse_enabled is false
health_report_interval: 60  # seconds; 0 disables health reporting

execution_policy:
  enabled: true
  timeout: 30s
  allowed_commands: []      # non-empty = allowlist
  denied_commands: []       # deny always wins

allow_insecure_central: false   # keep false: production requires https://

filesystem:
  allow_absolute_paths: false   # keep false: paths stay under project roots

http_check:
  allow_endpoints: []
  allow_hosts: []
  allow_cidrs: []
  allow_private: false
```

**Transport security.** With `server.environment: production` the agent refuses
to start unless `central_url` uses `https://`. `allow_insecure_central: true` is
a development-only escape hatch for a TLS-terminating proxy.

### 3. Define projects (optional)

Project profiles describe what is deployable on the host. They are consumed by
`deploy.project`, `workflow.deploy`, `workflow.diagnose`, `file.read` and
`filesystem.list`; loading a profile never executes anything.

```yaml
projects:
  - name: merchant-api
    path: /srv/merchant-api
    deploy:
      type: pm2              # docker-compose | pm2 | script
      process: merchant-api
    health:
      url: http://127.0.0.1:3000/health
```

Each project needs a unique name and an absolute path. With a `deploy` block,
the strategy's own field is required (`compose_file` / `process` / `script`).
Without one, the project falls back to the legacy tool pair and must define both
a `restart` and a `logs` tool reference.

### 4. Start and verify

```bash
sudo systemctl restart opspilot-agent
systemctl status opspilot-agent
journalctl -u opspilot-agent -n 50
```

On startup the agent registers (if needed), syncs its capabilities, then runs
three loops: heartbeat, health reporting, and the command poll loop. With SSE
enabled it also keeps a signed stream open to `GET /api/v1/agents/events`;
central pushes a wake-up whenever a leasable command appears, cutting
interactive latency from the poll interval to roughly one round trip. The SSE
channel carries **only** a "wake up and lease" notice — never payloads, secrets,
approvals or results.

### 5. Available tools

`system.uptime`, `system.memory`, `system.cpu`, `system.disk`,
`system.processes`, `pm2.list`, `pm2.logs`, `pm2.restart`, `docker.ps`,
`docker.logs`, `docker.restart`, `docker.inspect`, `systemctl.status`,
`systemctl.restart`, `journal.logs`, `git.status`, `git.current_commit`,
`git.branch`, `git.pull`, `http.check`, `file.read`, `filesystem.list`,
`workflow.diagnose`, `workflow.deploy`, `deploy.project`.

`pm2.restart`, `docker.restart`, `systemctl.restart`, `git.pull`,
`workflow.deploy` and `deploy.project` require operator confirmation; the rest
are read-only. Unavailable tools (missing `docker`, non-Linux host, …) stay
registered and report *why* they are unavailable — availability is advisory
metadata, not an execution gate.

Two tools are hardened beyond the policy gate:

- `file.read` / `filesystem.list` resolve paths against the configured project
  roots. `..` traversal and symlink escapes are rejected, and absolute paths are
  denied unless `filesystem.allow_absolute_paths: true`.
- `http.check` is SSRF-hardened: loopback, link-local (including the cloud
  metadata address `169.254.169.254`), RFC1918, CGNAT, multicast and reserved
  ranges are denied by default; every resolved IP is validated and the
  connection is pinned to it (defeating DNS rebinding); redirects are never
  followed; response bodies and headers are never returned.

### 6. Uninstall

```bash
sudo scripts/uninstall-agent.sh
```

It stops the service, attempts to unregister with central, removes the unit and
the binary, and asks before removing `/etc/opspilot/`. Logs are never removed.

> **Known issue.** The script still sends an *unsigned* unregister request, so
> central rejects it with `401`. The uninstall continues (it warns and asks
> `Continue uninstall? (Y/n)`), but the agent stays `online` in central and must
> be unregistered manually. See implementation §11.

---

## OpsPilot MCP — setup guide

`opspilot-mcp` speaks MCP over stdio and connects **directly to PostgreSQL** —
it never calls the Central REST API. Run it under a least-privilege database
role that can `SELECT` the platform tables and `INSERT` into `commands`, never a
superuser or the schema owner.

There is **no installer script for the MCP server** — it is a stdio process
started by the MCP client, not a service. Install the release asset by hand on
whichever host runs the client:

```bash
curl -fsSL -o opspilot-mcp \
  https://github.com/tsee9iii/opspilot/releases/latest/download/opspilot-mcp-linux-amd64
sudo install -m 0755 opspilot-mcp /usr/local/bin/opspilot-mcp
```

It reads the same `pkg/config` as central, so it takes the same YAML file and
environment variables. The one setting you must choose is the mode:

| `OPSPILOT_MCP_MODE` | Exposes | Contacts agents? |
| ------------------- | ------- | ---------------- |
| `inventory` (default) | `ping`, `list_servers`, `list_agents`, `list_commands`, `get_command`, `get_agent_health`, `list_agent_health`, `list_unhealthy_agents`, `list_alerts`, `get_alert` | No — pure reads of central state |
| `investigate` | the above plus `file_read`, `filesystem_list`, `docker_inspect`, `workflow_diagnose`, `pm2_list`, `pm2_logs`, `docker_list`, `docker_logs`, `journal_logs`, `git_status`, `git_current_commit`, `git_branch` | Yes — bounded, read-only commands |
| `operate` | the above plus `workflow_deploy` | Yes — the only mutating tool, always operator-approved |

Modes are strictly cumulative and default to the most restrictive tier.
`inventory` is the safest choice for broad assistant access precisely because
none of its tools reach a managed host. The deprecated
`OPSPILOT_MCP_READ_ONLY` flag maps `true → inventory` and `false → operate`; an
explicit mode always wins.

Example client configuration:

```json
{
  "mcpServers": {
    "opspilot": {
      "command": "/usr/local/bin/opspilot-mcp",
      "env": {
        "OPSPILOT_DB_HOST": "localhost",
        "OPSPILOT_DB_NAME": "opspilot",
        "OPSPILOT_DB_USER": "opspilot_mcp",
        "OPSPILOT_DB_PASSWORD": "<password>",
        "OPSPILOT_DB_SSLMODE": "require",
        "OPSPILOT_MCP_MODE": "inventory"
      }
    }
  }
}
```

Regardless of mode, the log tools bound `lines` to 1–1000, repository paths are
relayed to the agent's own validated input model, and `pm2.restart`,
`docker.restart`, `systemctl.restart`, `git.pull` and `deploy.project` are
absent from every tier.

---

## Development

These targets are for the development machine only — servers install release
binaries (see [Deployment model](#deployment-model-release-first-then-install)).

```bash
make build          # bin/central, bin/agent, bin/mcp
make test           # go test ./...
make fmt vet        # gofmt + go vet
make dev-up         # PostgreSQL 16 on localhost:5432 (opspilot/opspilot)
make dev-down
make sqlc-generate  # regenerate gen/postgresql from sql/queries + sql/migrations
make help           # list targets
```

`go test ./...` on an unrestricted Linux runner is the authoritative
verification command and the release gate.

- HTTP integration tests use `httptest.NewServer`, which binds a loopback port.
  In a sandbox that blocks loopback networking they fail for **environment**
  reasons, not code reasons.
- PostgreSQL-backed tests read `OPSPILOT_TEST_DATABASE_URL` (default
  `postgres://opspilot:opspilot@localhost:5432/opspilot?sslmode=disable`) and
  skip themselves when the database is unreachable.
- Installer tests: `scripts/install-tests.sh` runs `scripts/install.sh` against
  a mock central with stubbed system tools.

CI (`.github/workflows/ci.yml`) runs on Ubuntu with a PostgreSQL 16 service and
executes `gofmt` validation, `go vet ./...` and `go test ./...`. Pushing a `v*`
tag triggers `.github/workflows/release.yml`, which publishes six binaries —
agent, central and mcp for `linux-amd64` and `linux-arm64`. Version strings
reported by the services live in `pkg/version/version.go`; bump them there before
tagging when a release changes them.
