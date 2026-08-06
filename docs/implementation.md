# opspilot — Repository Reference

Reference of what is currently implemented. Unimplemented areas are stated
explicitly as **Not Implemented**. Planned features are never documented as
implemented.

This document describes the current implementation only.

Architectural decisions are documented in:

- docs/architecture.md
- docs/adr/
  Planned features are tracked in:
- docs/roadmap.md

## 1. Feature matrix

| Feature                                          | Central                         | Agent                    | Status          |
| ------------------------------------------------ | ------------------------------- | ------------------------ | --------------- |
| HTTP server with graceful shutdown               | `cmd/central`                   | —                        | Implemented     |
| Agent registration (HMAC token, Argon2id secret) | `POST /api/v1/agents/register`  | startup                  | Implemented     |
| Agent heartbeat                                  | `POST /api/v1/agents/heartbeat` | 30s loop                 | Implemented     |
| Agent unregister (lifecycle)                     | `POST /api/v1/agents/unregister` | —                        | Implemented     |
| Command creation                                 | `POST /api/v1/commands`         | —                        | Implemented     |
| Command leasing (atomic, FIFO)                   | `POST /api/v1/commands/lease`   | poll loop                | Implemented     |
| Command lifecycle transitions                    | `start` / `complete` / `fail`   | reports them             | Implemented     |
| Execution policy                                 | —                               | gates tools              | Implemented     |
| Tool metadata (name, version, description, schema, confirmation) | `capabilities.parameter_schema`, `capabilities.confirmation_level` | advertised at startup | Implemented     |
| Confirmation policy (none / required)            | persisted per capability        | metadata on each tool    | Implemented     |
| Confirmation enforcement                          | `commands.confirmation_status`  | —                        | Implemented     |
| Payload schema validation                        | —                               | `gojsonschema` in executor | Implemented     |
| Capability availability                          | `capabilities.available`, `capabilities.unavailable_reason` | per-tool `Availability` check, synced at startup | Implemented |
| Tool Registry | — | `Register`/`Find`/`List` | Implemented |
| `system.uptime` tool | — | `/usr/bin/uptime` | Implemented |
| `system.memory` tool | — | `/proc/meminfo` | Implemented |
| `system.cpu` tool | — | `/proc/stat` | Implemented |
| `system.disk` tool | — | `statfs(2)` on `/` | Implemented |
| `system.processes` tool | — | `/proc/<pid>/*` | Implemented |
| `pm2.list` tool | — | `pm2 jlist` | Implemented |
| `pm2.logs` tool | — | `pm2 logs --nostream --raw` | Implemented |
| `pm2.restart` tool | — | `pm2 restart` | Implemented |
| `docker.ps` tool | — | `docker ps --format '{{json .}}'` | Implemented |
| `docker.logs` tool | — | `docker logs --tail <n>` | Implemented |
| `docker.restart` tool | — | `docker restart` | Implemented |
| `systemctl.status` tool | — | `systemctl show --property=...` | Implemented |
| `systemctl.restart` tool | — | `systemctl restart` | Implemented |
| `journal.logs` tool | — | `journalctl -u <svc> -n <n>` | Implemented |
| `git.status` tool | — | `git status --porcelain=v1 --branch` | Implemented |
| `git.current_commit` tool | — | `git log -1 --pretty=format:%H%n%h%n%an%n%ae%n%ad%n%s` | Implemented |
| `git.branch` tool | — | `git branch --show-current` + `git rev-parse @{u}` | Implemented |
| `git.pull` tool | — | `git pull --ff-only` | Implemented |
| `http.check` tool | — | HTTP GET health check | Implemented |
| Project Profiles (foundation) | — | config + discovery of `projects:` profiles | Implemented |
| Workflow engine (foundation) | — | step state machine, simulated execution | Implemented |
| Deploy workflow (execution) | — | git.pull → restart → optional http.check via RegistryExecutor | Implemented |
| Diagnose workflow (execution) | — | system.* + platform + logs + optional http.check, continue-on-failure | Implemented |
| Agent installer (Phase 2, auto-register)          | —                               | `scripts/install.sh`     | Implemented     |
| Central installer (Phase 1)                      | `scripts/install-central.sh`    | —                        | Implemented     |
| GitHub release pipeline                          | `.github/workflows/release.yml` | tag push `v*`            | Implemented     |
| Embedded migration framework                     | `internal/migration/` + `cmd/migrate` | auto-migrate on central startup | Implemented     |
| Config loading (YAML + env precedence)           | `pkg/config` (`/etc/opspilot/central.yaml`, `OPSPILOT_CONFIG`) | —            | Implemented     |
| Capability registration                          | `POST /api/v1/capabilities`     | startup sync             | Implemented     |
| WebSocket / Telegram / AI                        | —                               | —                        | Not Implemented |
| Docker SDK, sandboxing                           | —                               | —                        | Not Implemented |
| Command results query API                        | `GET /api/v1/commands/{id}`     | —                       | Implemented     |
| Registration token CLI                           | `opspilot-central token <create/list/revoke>` | —             | Implemented     |
| Token rotation, metrics                          | —                               | —                        | Not Implemented |

## 2. HTTP API

All JSON. Errors use `{"error":{"code","message"}}`.

| Method | Path                        | Auth            | Success                                  | Error codes                                                                  |
| ------ | --------------------------- | --------------- | ---------------------------------------- | ---------------------------------------------------------------------------- |
| GET    | `/healthz`                  | —               | `200 ok`                                 | —                                                                            |
| POST   | `/api/v1/agents/register`   | token           | `201 {agent_id,status,signing_key}`      | 400 `validation_error`, 401 `invalid_token`, 409 `token_already_used`, 500   |
| POST   | `/api/v1/agents/heartbeat`  | HMAC signing    | `200 {status,next_heartbeat}`            | 400, 401 `invalid_signature`, 500                                            |
| POST   | `/api/v1/agents/unregister` | HMAC signing    | `200 {status:"unregistered"}`           | 400, 401 `invalid_signature`, 404 `agent_not_found`, 500                     |
| POST   | `/api/v1/commands`          | —               | `201 {command_id,status}`                | 400, 500                                                                     |
| POST   | `/api/v1/commands/lease`    | HMAC signing    | `200 {command_id,tool,payload}` or `204` | 400, 401 `invalid_signature`, 500                                            |
| POST   | `/api/v1/commands/start`    | HMAC signing    | `200 {command_id,status}`                | 400, 401, 403 `command_not_owned`, 404 `not_found`, 409 `invalid_transition`, 500 |
| POST   | `/api/v1/commands/complete` | HMAC signing    | `200 {command_id,status}`                | same as `start`                                                              |
| POST   | `/api/v1/commands/fail`     | HMAC signing    | `200 {command_id,status}`                | same as `start`                                                              |
| POST   | `/api/v1/commands/approve`  | operator bearer | `200 {status}`                           | 400 `validation_error`, 401 `unauthorized`, 404 `command_not_found`, 500      |
| GET    | `/api/v1/commands/{id}`    | operator bearer | `200 {command}`, see below               | 400 `invalid_command_id`, 401 `unauthorized`, 404 `command_not_found`, 500    |
| POST   | `/api/v1/capabilities`      | HMAC signing    | `200 {status,count}`                     | 400, 401 `invalid_signature`, 500                                            |

HMAC signing (see §8): every agent request carries `X-Agent-Id`,
`X-Agent-Timestamp`, `X-Agent-Nonce`, `X-Agent-Signature` computed over
`agent_id "\n" timestamp "\n" nonce "\n" method "\n" path "\n" body` with the
agent's per-agent signing key. Registration itself is unsigned and returns the
signing key once.

Request bodies:

- `register`: `registration_token`, `secret`, `version`, `server{hostname,environment}`
- `heartbeat`: `agent_id`
- `unregister`: `agent_id`
- `create`: `agent_id`, `tool`, `payload` (JSON object)
- `lease`: `agent_id`
- `start`/`complete`/`fail`: `agent_id`, `command_id` (+ `result` for complete, `error` for fail)
- `approve`: `command_id`
- `capabilities`: `agent_id`, `capabilities[{tool_name,version,description,parameter_schema,confirmation_level,available,unavailable_reason}]`

`GET /api/v1/commands/{id}` response body:

```json
{
  "id": "…uuid",
  "agent_id": "…uuid",
  "status": "pending|leased|running|completed|failed",
  "confirmation_status": "pending|approved",
  "tool": "system.uptime",
  "parameters": { "interval": 5 },
  "result": { "uptime_seconds": 42 },
  "error": "…",            // omitted when empty
  "created_at": "…RFC3339",
  "leased_at": "…RFC3339", // omitted until leased
  "completed_at": "…RFC3339" // omitted until completed
}
```

`parameters` and `result` are opaque JSON returned exactly as stored; the
result is never recalculated or deserialized. `result` is omitted until the
command completes.

## 3. Database schema

Migrations (`sql/migrations/0001..0011`):

- `0001_init.sql` — `servers`, `agents`, `commands`
- `0002_agent_auth.sql` — `registration_tokens`
- `0003_command_lease.sql` — adds `commands.leased_at`, `commands.lease_owner`
- `0004_command_execution.sql` — adds `commands.started_at`, `commands.completed_at`
- `0005_capabilities.sql` — `capabilities`
- `0006_capability_parameter_schema.sql` — adds `capabilities.parameter_schema`
- `0007_capability_confirmation.sql` — adds `capabilities.confirmation_level`
- `0008_command_confirmation.sql` — adds `commands.confirmation_status` (default
  `approved`) and `commands.confirmed_at`
- `0009_capability_availability.sql` — adds `capabilities.available` (default
  `true`) and `capabilities.unavailable_reason` (default `''`)
- `0010_agent_unregister.sql` — restricts `agents.status` to
  `online` / `offline` / `unregistered` via a CHECK constraint and backfills
  existing agents to `online`
- `0011_agent_signing_key.sql` — adds `agents.signing_key`, the per-agent HMAC
  signing key issued at registration and used to sign every agent request

The migration runner (see §22) creates a `schema_migrations` bookkeeping table
(`version TEXT PRIMARY KEY`, `applied_at TIMESTAMPTZ NOT NULL DEFAULT now()`)
when it runs; it is not part of the numbered migrations.

Tables:

- **servers** — `id`, `name`, `hostname`, `environment`, `status`, `created_at`,
  `updated_at`; `UNIQUE (hostname, environment)`.
- **agents** — `id`, `server_id` FK, `secret` (Argon2id hash, see §8), `version`,
  `status` (`online` | `offline` | `unregistered`), `last_heartbeat`,
  `created_at`, `updated_at`.
- **commands** — `id`, `agent_id` FK, `tool_name`, `payload` JSONB, `status`,
  `result` JSONB, `error`, `leased_at`, `lease_owner`, `started_at`,
  `completed_at`, `confirmation_status` (`approved` | `pending`, default
  `approved`), `confirmed_at`, `created_at`, `updated_at`. State machine:
  `pending → leased → running → completed | failed`, enforced by atomic
  `UPDATE ... WHERE status = '<expected>'`. Commands are only leased when
  `confirmation_status = 'approved'`; `pending` commands wait for
  `POST /api/v1/commands/approve`.
- **registration_tokens** — `id`, `token_hash` UNIQUE (HMAC-SHA256 hex),
  `environment`, `expires_at`, `revoked_at`, `created_at`. Tokens are deleted
  on consumption; no `used_at` column.
- **capabilities** — `id`, `agent_id` FK, `tool_name`, `version`,
  `description`, `parameter_schema` JSONB (JSON Schema document),
  `confirmation_level` TEXT (`none` | `required`), `available` BOOLEAN
  (default `true`), `unavailable_reason` TEXT (default `''`; non-empty only
  when `available = false`), `created_at`,
  `updated_at`; `UNIQUE (agent_id, tool_name)`.

Indexes: `idx_agents_server_id`, `idx_commands_agent_id`, `idx_commands_status`,
`idx_capabilities_agent_id`.

## 4. Agent runtime

`cmd/agent` runs `internal/agent.Agent`.

- Config from YAML (§9), path via `OPSPILOT_AGENT_CONFIG` (default `agent.yaml`).
- **Startup**: if `agent_id` is empty, register (persist `agent_id` back to the
  config file, mode `0600`), then sync capabilities (§6), then start the
  heartbeat loop and the command poll loop.
- **Heartbeat loop** (goroutine): `POST /api/v1/agents/heartbeat` every 30s,
  then sleeps the server-provided `next_heartbeat`.
- **Command poll loop** (main): every `poll_interval` (default 5s):
  lease one command → `start` it → execute via the executor → `complete` with
  the result or `fail` with the error. A `204` (empty queue) sleeps until the
  next tick. The executor validates the leased payload against the tool's
  parameter schema before running it; an invalid payload fails the command via
  the normal `fail` transition.
- SIGINT/SIGTERM cancel the context; the loops exit cleanly.

## 5. Tool Registry

- `internal/agent/registry.go` — `Tool` interface (`Name`, `Version`,
  `Description`, `ParameterSchema`, `ConfirmationLevel`,
  `Availability(ctx) (bool, string)`, `Execute(ctx, payload)`)
  and a concurrency-safe `Registry` with `Register`, `Find`, `List` (sorted
  names). `ParameterSchema` returns the tool's accepted payload as a JSON
  Schema document; tools that take no payload return
  `{"type":"object","properties":{}}`. `ConfirmationLevel` returns the tool's
  confirmation metadata: `agent.ConfirmationNone` (`"none"`) for read-only
  tools and `agent.ConfirmationRequired` (`"required"`) for write tools.
  `Availability` reports whether the tool can run in the current environment
  (see §14); there is no execution-behavior change — availability is metadata
  only.
- `internal/agent/registry_executor.go` — the agent's `Executor`. It never
  switches on tool names: `Find → policy gate → validate payload → Execute`. An
  unregistered name returns `tool not implemented`.
- `internal/agent/schema.go` — validates a command payload against the tool's
  `ParameterSchema()` before execution using `github.com/xeipuuv/gojsonschema`.
  Supported keywords: `type`, `required`, `properties`, `enum`, `minimum`,
  `maximum`, `additionalProperties`. An empty payload is treated as `{}`. A
  failed validation short-circuits `Execute` (the tool never runs) and returns
  a stable, human-readable error (e.g. `required property "process" missing`,
  `property "lines" must be integer`) that the command loop reports via the
  existing `fail` transition.
- Tools are grouped into category packages under `internal/agent/tools/` and
  registered once in `cmd/agent/main.go`:
  - `internal/agent/tools/system/` — `system.*` tools: `uptime.go`, `memory.go`,
    `cpu.go`, `disk.go` (+ `diskstat_linux.go`/`diskstat_other.go` build-tagged
    `statfs(2)`), `processes.go`.
  - `internal/agent/tools/pm2/` — `pm2.*` tools: `list.go`, `logs.go`,
    `restart.go`.
  - `internal/agent/tools/docker/` — `docker.*` tools: `ps.go`, `logs.go`,
    `restart.go`, plus `common.go` (shared `docker --version` check,
    `docker ps` listing, and container-existence verification).
  - `internal/agent/tools/systemctl/` — `systemctl.*` tools: `status.go`,
    `restart.go`.
  - `internal/agent/tools/journal/` — `journal.*` tools: `journal.go`.
  - `internal/agent/tools/git/` — `git.*` tools: `status.go`, `commit.go`,
    `branch.go`, `pull.go`, plus `common.go` with reusable Git helpers
    (`ensureGit`, `ensureRepository`, `runGit`/`runGitRaw`,
    `parseRepositoryRequest`, `parseBranchName`, `currentBranch`,
    `parseBranchHeader`, `parsePorcelainStatus`) for future Git tools.
  - `internal/agent/tools/http/` — `http.*` tools: `check.go`, plus
    `common.go` with reusable HTTP helpers (`buildClient`, `performRequest`,
    `validateURL`, `classifyRequestError`) for future HTTP tools.
- Current tools (read-only tools — `system.*`, `pm2.list`, `pm2.logs`,
  `docker.ps`, `docker.logs`, `systemctl.status`, `journal.logs` — advertise
  `confirmation_level = none`; write tools `pm2.restart`, `docker.restart`,
  and `systemctl.restart`, and `git.pull` advertise
  `confirmation_level = required`):
  - `system.uptime` — runs `/usr/bin/uptime` via `exec.CommandContext`;
    returns `{"stdout","stderr","exit_code"}`.
  - `system.memory` — parses `/proc/meminfo` (Linux only) and returns
    `{"total_bytes","available_bytes","used_bytes","used_percent"}`. `used`
    and `used_percent` derive from `MemTotal` − `MemAvailable` (values are
    kB, converted to bytes); `used_percent` is rounded to two decimals.
  - `system.cpu` — parses `/proc/stat` (Linux only) and returns
    `{"user_percent","system_percent","idle_percent"}`. Percentages come from
    two samples taken ~200 ms apart (per-interval deltas), bucket user+nice /
    system+irq+softirq / idle+iowait, and are rounded to two decimals.
  - `system.disk` — calls `statfs(2)` on `/` (Linux only) and returns
    `{"total_bytes","used_bytes","available_bytes","used_percent"}`.
    `used` = total − free (`bfree`), `available` = `bavail`; `used_percent`
    is `used / total` rounded to two decimals.
  - `system.processes` — scans `/proc/<pid>/{comm,stat,status}` (Linux only)
    and returns the top 10 processes by CPU usage as
    `{"pid","name","cpu_percent","memory_bytes"}`. Two samples taken ~200 ms
    apart give per-interval CPU deltas; `cpu_percent` is expressed as a
    fraction of a single core (like `top`, can exceed 100 for multi-threaded
    processes) and `memory_bytes` is `VmRSS`. Processes present in only one
    sample are skipped.
  - `pm2.list` — runs `pm2 jlist` (Linux only, requires `pm2` on `PATH`) and
    returns the running PM2 processes as
    `{"name","status","pid","cpu_percent","memory_bytes","uptime"}`. Fields
    map from `name`, `pm2_env.status`, `pid`, `monit.cpu`, `monit.memory`,
    and `uptime` is derived from `pm2_env.pm_uptime` (start epoch, ms) in
    seconds.
  - `pm2.logs` — runs `pm2 logs <process> --lines <n> --nostream --raw --out`
    and `--err` (Linux only, requires `pm2` on `PATH`) and returns
    `{"process","stdout","stderr","lines"}`. The payload is validated against
    its parameter schema: `process` is required; `lines` defaults to 100 and
    must be 1..1000. The process is confirmed to exist via `pm2 jlist` first;
    missing `pm2`, an unknown process, or a non-zero exit all surface as
    errors.
  - `pm2.restart` — verifies the process via `pm2 jlist` then runs
    `pm2 restart <process>` (Linux only, requires `pm2` on `PATH`) and returns
    `{"process","status":"restarted"}`. The payload is validated against its
    parameter schema (`process` required); missing `pm2`, an unknown process,
    or a restart failure all surface as errors.
  - `docker.ps` — runs `docker ps --format '{{json .}}'` (requires `docker` on
    `PATH`) and returns `{"containers":[...]}` where each container is
    `{"id","name","image","state","status","ports"}`. Fields map from the
    Docker CLI's `ID`, `Names` (joined), `Image`, `State`, `Status`, and
    `Ports`. An empty output yields `{"containers":[]}`. Missing `docker`, a
    non-zero exit, or an invalid JSON line all surface as errors.
  - `docker.logs` — verifies `docker` is installed and that the container
    exists (reusing `docker.ps` parsing logic), then runs
    `docker logs --tail <lines> <container>` and returns
    `{"container","stdout","stderr","lines"}`. The payload is validated against
    its parameter schema: `container` is required (name or ID); `lines`
    defaults to 100 and must be 1..1000. Missing `docker`, an unknown
    container, or a non-zero `docker logs` exit all surface as errors.
  - `docker.restart` — verifies `docker` is installed (`docker --version`) and
    that the container exists (reusing `docker.ps` parsing logic), then runs
    `docker restart <container>` and returns
    `{"container","status":"restarted"}`. The payload is validated against its
    parameter schema (`container` required, name or ID). Missing `docker`, an
    unknown container, a non-zero `docker restart` exit, or an execution
    failure all surface as errors.
  - `systemctl.status` — verifies `systemctl` is available
    (`systemctl --version`), then runs
    `systemctl show <service> --property=Id --property=Description
    --property=LoadState --property=ActiveState --property=SubState
    --property=UnitFileState --property=MainPID --property=ExecMainStatus
    --no-pager` and parses the key=value output (not the human-readable
    `systemctl status`), returning
    `{"service","description","load_state","active_state","sub_state",
    "unit_file_state","main_pid","exit_status"}`. The payload is validated
    against its     parameter schema (`service` required). Missing `systemctl`, an
    unknown service, a non-zero exit, or malformed key=value output all
    surface as errors.
  - `journal.logs` — verifies `journalctl` is available
    (`journalctl --version`), then runs
    `journalctl -u <service> -n <lines> --no-pager -o short-iso` and returns
    `{"service","stdout","stderr","lines"}`. The payload is validated against
    its parameter schema: `service` is required; `lines` defaults to 100 and
    must be 1..1000. Missing `journalctl`, an unknown service (empty journal
    output), or a non-zero `journalctl` exit all surface as errors.
  - `systemctl.restart` — verifies `systemctl` is available
    (`systemctl --version`), verifies the service exists (reusing the
    `systemctl.status` parsing logic), then runs `systemctl restart <service>`
    and returns `{"service","status":"restarted"}`. The payload is validated
    against its parameter schema (`service` required). Missing `systemctl`, an
    unknown service, a non-zero `systemctl restart` exit, or an execution
    failure all surface as errors.
  - `git.status` — verifies `git` is installed (`git --version`), verifies the
    repository path exists and is inside a Git work tree
    (`git -C <repository> rev-parse --is-inside-work-tree`), then runs
    `git -C <repository> status --porcelain=v1 --branch` and returns
    `{"repository","branch","detached","ahead","behind","dirty","changes"}`.
    Each `changes` entry is `{"path","index_status","worktree_status"}` with
    git's porcelain status codes preserved verbatim and file ordering intact;
    `dirty` is `true` when there is at least one change. The payload is
    validated against its parameter schema (`repository` required, absolute
    path). Missing `git`, a non-existent repository, a path outside a work
    tree, a non-zero `git status` exit, or malformed porcelain output all
    surface as errors. No raw git output is ever returned.
  - `git.current_commit` — verifies `git` is installed (`git --version`),
    verifies the repository path exists and is inside a Git work tree
    (`git -C <repository> rev-parse --is-inside-work-tree`), then runs
    `git -C <repository> log -1 --date=iso-strict --pretty=format:%H%n%h%n%an%n%ae%n%ad%n%s`
    and returns
    `{"repository","commit","short_commit","author_name","author_email",
    "author_date","subject"}`. The full hash, short hash, ISO-8601 author
    date, and subject are preserved as reported by Git; the subject is the
    commit subject line only. The payload is validated against its parameter
    schema (`repository` required). Missing `git`, a non-existent repository,
    a path outside a work tree, a repository with no commits, a non-zero
    `git log` exit, or malformed output (not exactly six fields) all surface
    as errors. No raw git output is ever returned.
  - `git.branch` — verifies `git` is installed (`git --version`), verifies the
    repository path exists and is inside a Git work tree
    (`git -C <repository> rev-parse --is-inside-work-tree`), then runs
    `git -C <repository> branch --show-current` and
    `git -C <repository> rev-parse --abbrev-ref --symbolic-full-name @{u}`,
    returning
    `{"repository","branch","detached","tracking","upstream"}`. An empty
    branch means detached HEAD (`detached: true`, `branch: ""`); a non-zero
    exit from the `@{u}` lookup means no upstream is configured and is **not**
    an error (`tracking: false`, `upstream: ""`). The upstream is preserved
    exactly as reported by Git. The payload is validated against its parameter
    schema (`repository` required). Missing `git`, a non-existent repository,
    a path outside a work tree, a non-zero `git branch` exit, or malformed
    output (multi-line branch/upstream names) all surface as errors. No raw
    git output is ever returned.
  - `git.pull` — the first write-capable Git tool
    (`confirmation_level = required`). Verifies `git` is installed
    (`git --version`), verifies the repository path exists and is inside a
    Git work tree, resolves the checked-out branch and its upstream (via
    `git branch --show-current` and `git rev-parse @{u}`, sharing
    `currentBranch` with `git.branch`), then runs
    `git -C <repository> pull --ff-only`. Detached HEAD and a missing
    upstream are hard errors. The command never merges, rebases, forces, or
    resets — only fast-forwards are allowed. The structured result is
    `{"repository","updated","branch","upstream","message"}` with message
    `Already up to date.` or `Fast-forward completed.`. Missing `git`, a
    non-existent repository, a path outside a work tree, detached HEAD, no
    upstream configured, a non-fast-forwardable history
    (`fast-forward not possible`), local changes that would be overwritten
    (`merge required`), any other non-zero `git pull` exit, or unrecognized
    output (malformed) all surface as errors. No raw git output is ever
    returned.
  - `http.check` — performs a read-only HTTP health check using Go's
    `net/http` (no binary dependency, so it is always available). The payload
    is validated against its parameter schema: `url` is required
    (`http://` or `https://` only), `expected_status` defaults to 200
    (100..599) and `timeout_seconds` defaults to 10 (1..60). It sends a
    single `GET` with the given timeout, **never follows redirects**, and
    returns `{"url","reachable","status_code","expected_status","healthy",
    "duration_ms"}` where `healthy = status_code == expected_status` and
    `duration_ms` is always reported. The response body and headers are never
    returned. An invalid or non-http(s) URL, a timeout, a connection refused,
    a DNS lookup failure, or a TLS failure all surface as clear errors; the
    request itself failing is the only way `reachable` would be false.
- Availability per tool (`Availability(ctx)`): the `system.*` tools report
  `unsupported platform` on non-Linux hosts and are available on Linux; the
  `pm2.*`/`docker.*`/`systemctl.*` tools and `journal.logs` report
  `<binary> is not installed` / `<binary> is not runnable` based on a
  `--version` check; the `git.*` tools report `git is not installed` /
  `git is not runnable` based on `git --version`;   `http.check` is always
  available (see §14).
- `internal/agent/project/` — Project Profiles (configuration and discovery
  only, no execution): `profile.go` (`Project`, `ToolReference`),
  `config.go` (YAML shape of the `projects:` section), `loader.go`
  (`Loader`, `New`, `Projects`, `FindProject`). See §15.
- `internal/agent/workflow/` — the workflow engine: `workflow.go`
  (`Workflow`, `Step`, `NewWorkflow`, `BuildDeployWorkflow`,
  `BuildDiagnoseWorkflow`), `result.go`
  (`Result`, `StepResult`, `StepStatus`), `executor.go` (`Executor`).
  See §16.
- `internal/agent/tool.go` — shared helpers used by the tool packages:
  `CommandResult{Stdout,Stderr,ExitCode}`, `EmptyParameterSchema` (the
  `{"type":"object","properties":{}}` schema constant), `RunCommand`
  (context-aware `exec.CommandContext` wrapper; context expiry surfaces as a
  `tool timed out` error), and `BinaryAvailable` (runs `<binary> --version`;
  returns `binary is not installed` on `exec.ErrNotFound` and
  `binary is not runnable` on any other failure).

## 6. Capability registration

- Agent startup calls `registry.List()` and sends each tool's `name`, `version`,
  `description`, `parameter_schema`, `confirmation_level`, `available`, and
  `unavailable_reason` to
  `POST /api/v1/capabilities` (one request, batch body).
- Central (`internal/application/capability`) authenticates the agent (by id +
  secret) then upserts each capability (`ON CONFLICT (agent_id, tool_name) DO
  UPDATE`), returning the number persisted. The parameter schema is stored in
  `capabilities.parameter_schema` (JSONB), the confirmation level in
  `capabilities.confirmation_level` (TEXT, validated to be `none` or
  `required`), and the runtime availability in `capabilities.available` /
  `capabilities.unavailable_reason`.

## 7. Execution policy

- `internal/agent/policy.go` — `ExecutionPolicy{Enabled, Timeout,
AllowedCommands, DeniedCommands, WorkingDirectory}`.
- `Allow(name)` order: disabled → denied list (deny wins) → allow list
  (non-empty allow list requires membership).
- Applied by `RegistryExecutor` before a tool runs; `Timeout` bounds tool
  execution via a derived context.
- `working_directory` is parsed and stored but **not applied** in the current
  registry-driven flow (no shell executor uses it).

## 8. Security model

- **Registration tokens**: stored only as `HMAC-SHA256(OPSPILOT_AUTH_SERVER_SECRET,
token)` hex, never plaintext. Registration consumes the row atomically; a
  replay returns `409 token_already_used`. Expired/revoked tokens return
  `401 invalid_token`.
- **Agent request signing**: every post-registration agent request (heartbeat,
  unregister, lease, start, complete, fail, capabilities) is HMAC-SHA256 signed
  with the agent's per-agent signing key (`agents.signing_key`), issued once at
  registration. The signature covers `agent_id "\n" timestamp "\n" nonce "\n"
  method "\n" path "\n" body` (shared `internal/agentsign` contract); central
  verifies constant-time, rejects timestamps outside the 5-minute window and
  rejects replayed nonces (`401 invalid_signature` / `expired_timestamp` /
  `replay_detected`). The Argon2id `secret` hash is retained at rest for
  backwards compatibility but is no longer verified per request. Agents whose
  status is `unregistered` are rejected with `401 invalid_credentials` on
  heartbeat and capability sync (§18).
- **Operator endpoints**: `POST /api/v1/commands/approve` and
  `GET /api/v1/commands/{id}` require a bearer token
  (`OPSPILOT_OPERATOR_TOKEN`, YAML `auth.operator_token`), verified
  constant-time (`401 unauthorized`).
- Command create is **not authenticated**; it is an internal enqueue endpoint.

## 9. Configuration

Central — `pkg/config`, resolved with precedence: built-in defaults → YAML file →
environment variables (highest). The YAML file is `/etc/opspilot/central.yaml`
(override with `OPSPILOT_CONFIG`); a missing file is not an error, and environment
variables always override YAML (see §23).

Environment variables: `OPSPILOT_ENV`, `OPSPILOT_HTTP_HOST`/`PORT`,
`OPSPILOT_DB_HOST`/`PORT`/`USER`/`PASSWORD`/`NAME`/`SSLMODE`,
`OPSPILOT_LOG_LEVEL`, `OPSPILOT_AUTH_SERVER_SECRET`
(default `dev-only-secret-change-me`), `OPSPILOT_OPERATOR_TOKEN`
(default `dev-operator-token-change-me`), `OPSPILOT_COMMAND_LEASE_TTL_SECONDS`
(default `60`).

Agent — YAML (`configs/agent.example.yaml`):
`central_url`, `registration_token`, `secret`, `signing_key`, `version`,
`server{hostname,environment}`, `agent_id`, `poll_interval`,
`execution_policy{enabled,timeout,allowed_commands,denied_commands,working_directory}`,
and the optional `projects` section (see §15).

## 10. Project structure

```
cmd/central/            central binary (HTTP server + token CLI)
cmd/central/token/      registration token CLI subcommands
cmd/agent/              agent binary
gen/postgresql/         sqlc-generated query code (checked in)
internal/
  agent/                agent runtime, registry, executor, policy, tools
  application/          use cases: agent, command, capability
  bootstrap/            central composition root, lifecycle
  domain/               entities: agent, server, command, registrationtoken
  infrastructure/
    postgres/           pool factory + repositories
    security/           HMAC and Argon2id hashers
  transport/http/       router, handlers, DTOs
pkg/config/             env-based config
pkg/logger/             zap logger
sql/migrations/         0001..0011 (+ embed.go)
sql/queries/            annotated SQL for sqlc
deployments/            docker-compose (PostgreSQL 16)
docs/                   architecture, implementation, roadmap, adr/
```

## 11. Technical debt

- `agents.secret` still names the Argon2id hash column `secret` (renaming to
  `secret_hash` was deferred in migration `0002`). The hash is retained at rest
  for backwards compatibility; per-request verification now uses HMAC request
  signing via `agents.signing_key`.
- `execution_policy.working_directory` is inert in the registry-driven flow.
- Leases expire lazily at lease time (task: lazy lease expiry), not via a
  background scheduler.
- `assertTransition` reads the command, then updates with an atomic `WHERE`;
  state is re-checked in SQL but the read is an extra round-trip.
- Migrations are embedded in the binary and run automatically at startup
  (§22); there is no down/rollback support.
- `docs/implementation.md` is maintained manually alongside the code.

## 12. Not Implemented

- `cmd/agent` WebSocket and the shared `pkg/protocol` package
- Telegram; Hermes runtime; DeepSeek/AI client
- Middleware (auth, logging, recovery) — handlers authenticate explicitly
- Registration-token admin endpoints (`Create`/`Revoke` exist in the
  repository but are not exposed over HTTP)
- Command results query / history API; capability and agent listing endpoints
- Audit and alert tables/domains
- Additional tools (Docker SDK); tool sandboxing; tool version matrix
- The `deploy.project` (and `diagnose.project`/`restart.project`) tool that
  wires the Project Profiles and workflow engine (§15, §16) into the registry
  and command pipeline
- Token rotation; metrics and observability beyond zap logs
- Migration `down`/rollback support (`cmd/migrate` only has `up` and `status`)

## 13. Confirmation enforcement

- Commands targeting write tools are gated on operator approval before they
  can be leased. The capability's `confirmation_level` (see §6) is metadata;
  enforcement is central-side only and the agent runtime is unchanged.
- **Creation** (`internal/application/command/create.go`): `CreateUseCase`
  resolves the target tool's confirmation level from the agent's capabilities
  (via `ConfirmationResolver` — implemented by the postgres
  `CapabilityRepository.ConfirmationLevel`). Level `required` sets
  `confirmation_status = pending`; level `none` (or a missing capability)
  sets `confirmation_status = approved`.
- **Leasing** (`sql/queries/lease_command.sql`): `LeaseNextCommand` adds
  `AND c.confirmation_status = 'approved'` to its sub-select, so pending
  commands are never leased and never reach the agent. Existing rows without
  the column default to `approved`.
- **Approval** (`internal/application/command/approve.go` +
  `POST /api/v1/commands/approve`): `ApprovalUseCase` validates the command id,
  then the repository atomically updates
  `confirmation_status = 'approved', confirmed_at = now()` where the command
  is   still `pending`. Approving an already-approved command is an idempotent
  success; a missing command returns `404 command_not_found`. Approval is
  independent of the command's lifecycle status.

## 14. Capability availability

- Every tool implements `Availability(ctx) (available bool, reason string)`.
  Contract: `available = true` implies `reason = ""`; `available = false`
  requires a non-empty reason.
- `agent.BinaryAvailable(ctx, run, binary)` (in `internal/agent/tool.go`) is the
  shared check for CLI-backed tools: it runs `<binary> --version`. A missing
  binary (`exec.ErrNotFound`) yields `<binary> is not installed`; any other
  failure (non-zero exit, spawn error) yields `<binary> is not runnable`; exit
  code 0 yields available.
- Per-tool mapping: `system.*` use `platformSupported()`
  (`internal/agent/tools/system/availability.go`) — available on Linux,
  `unsupported platform` elsewhere; `pm2.*`, `docker.*`, `systemctl.*`, and
  `journal.logs` check `pm2`, `docker`, `systemctl`, and `journalctl`
  respectively; the `git.*` tools check `git`; `http.check` has no binary
  dependency and is always available. The repository path is validated
  at execution time (`ensureRepository`), not in `Availability`, because
  availability carries no payload.
- The checks are intentionally cheap (a single `--version` probe); they run
  once per capability sync and never during execution, so the command
  lifecycle and tool execution are unaffected.
- At sync time (`registerCapabilities`, `internal/agent/capability.go`) the
  agent calls each tool's `Availability` and includes the result in the
  capability payload; central persists it (see §6). Unavailable tools remain
  registered and leaseable — availability is advisory metadata for the
  planner, not an execution gate.

## 15. Project Profiles

- **Purpose**: the configuration layer describing deployable projects on an
  agent. This feature is configuration and discovery only — no workflow, no
  tool execution, no deploy/restart/diagnose implementation. Future features
  (`deploy.project`, `diagnose.project`, `restart.project`) will consume the
  profiles.
- **Configuration**: an optional `projects` section in the agent YAML (see
  §9). Each entry has `name`, `repository` (absolute path), optional
  `health_url`, and a `tools` map. A tool reference names a registered tool
  via the `tool` key; every other key in the entry is a tool parameter, kept
  as arbitrary JSON. Example:

  ```yaml
  projects:
    - name: backend
      repository: /srv/backend
      health_url: http://localhost:3000/health
      tools:
        restart:
          tool: docker.restart
          container: backend
        logs:
          tool: docker.logs
          container: backend
  ```

- **Package** (`internal/agent/project/`): `profile.go` defines `Project`
  (`Name`, `Repository`, `HealthURL *string`, `Tools map[string]ToolReference`)
  and `ToolReference` (`Tool`, `Parameters json.RawMessage`). `config.go`
  defines the YAML shape (`Config`, `ToolConfig`); the tool parameters use a
  yaml `,inline` map so they round-trip through the agent config's `Save`.
  `loader.go` provides `New(cfgs) (*Loader, error)`, `Projects() []Project`
  (copy, configuration order), and `FindProject(name) (Project, bool)`.
- **Loading**: profiles load together with the existing agent configuration.
  `agent.LoadConfig` parses the `projects:` section into `[]project.Config`,
  builds the loader via `project.New`, and fails config loading on invalid
  profiles. `agent.Config` exposes `Projects()` and `FindProject(name)`
  (delegating to the loader; nil-safe when the section is absent or the
  config was constructed directly).
- **Validation**: unique project names (`duplicate project name: <name>`);
  absolute repository path (`filepath.IsAbs`); valid health URL when provided
  (`http`/`https` with a host); the `restart` tool reference exists; the
  `logs` tool reference exists; each tool reference names a tool. Tool
  **parameter schemas are not validated** here — that remains the existing
  JSON Schema validation framework's responsibility, and the registered tools
  are not consulted during loading.
- **Non-execution**: loading a profile never runs a tool, never consults the
  tool registry, and the agent runtime (Tool interface, Registry, capability
  registration, command lifecycle, JSON Schema validation, confirmation
  framework) is unchanged.

## 16. Workflow engine

- **Purpose**: the execution framework used by project operations (deploy,
  restart, diagnose, rollback). It executes real tools through the normal
  command execution pipeline.
- **Package** (`internal/agent/workflow/`): `workflow.go` defines `Workflow`
  (`Name`, `Steps []Step`) and `Step` (`Name`, `Tool project.ToolReference`)
  — each step references an existing project tool reference. `NewWorkflow`
  builds a workflow. `result.go` defines `Result{Workflow, Project,
  StartedAt, FinishedAt, Steps, Success}` and `StepResult{Name, Tool,
  Parameters, Status, StartedAt, FinishedAt, Result, Error}`, with `StepStatus`
  values `pending`, `running`, `completed`, `failed`, `skipped`.
  `executor.go` defines `Executor`.
- **Executor**: `NewExecutor(agent.Executor)` (optionally chained with
  `StopOnFailure(bool)`) and
  `Execute(ctx, p project.Project, wf Workflow) Result`. Each step is executed
  by calling the injected agent executor with the step's tool name and raw
  parameters — in production this is the `RegistryExecutor`, so every step
  goes through the exact pipeline as a leased command: registry lookup →
  execution-policy gate → JSON Schema payload validation → tool `Execute`
  (§5). The registry is never bypassed. A step transitions
  `pending → running → completed`; on success the tool's raw result bytes are
  stored in `StepResult.Result`.
- **Failure rules**:
  - **Stop-on-failure** (default, deploy): a failing step is marked `failed`
    with `Error`, the workflow stops immediately, every remaining step is
    marked `skipped`, and `Result.Success` is `false`. Execution never
    continues after a failure.
  - **Continue-on-failure** (`StopOnFailure(false)`, diagnose): every step
    executes regardless of failure. A failed step records `failed` + `Error`
    and the workflow moves on; steps are never skipped. The workflow succeeds
    (`Success = true`) when at least one step completed successfully.
- **Deploy workflow** (`BuildDeployWorkflow(p)`): builds the dynamic deployment
  workflow for a project — step 1 is always `git.pull` with
  `{"repository":"<project.repository>"}`; step 2 is `project.Tools["restart"]`
  copied exactly as stored (parameters unmodified); step 3 is `http.check`
  with `{"url":"<health_url>"}`, included only when the project has a
  `health_url`. The workflow is named `deploy`. The `deploy.project` tool that
  wires this into the registry is **not** implemented (§12).
- **Diagnose workflow** (`BuildDiagnoseWorkflow(p)`): builds the dynamic
  diagnostic workflow for a project — it gathers operational data only (no AI
  analysis). Always `system.cpu`, `system.memory`, `system.disk`,
  `system.processes` (all with `{}`), then exactly one platform step chosen by
  the project's restart tool: `docker.ps` (`{}`) when
  `project.Tools["restart"].Tool == "docker.restart"`, `pm2.list` (`{}`) when
  `pm2.restart`, or `systemctl.status` reusing the restart tool's exact
  parameters when `systemctl.restart`. Then the project's logs tool
  (`project.Tools["logs"]`) exactly as stored when a logs tool exists, and
  finally `http.check` with `{"url":"<health_url>"}` when a `health_url` is
  configured. The workflow is named `diagnose` and is executed with
  `StopOnFailure(false)`. The `diagnose.project` tool that wires this into the
  registry is **not** implemented (§12).
- **Project integration**: the executor operates on a `project.Project`
  produced by the Project Loader (§15); it never reads YAML directly.
- **Confirmation**: no approval handling is implemented — a workflow assumes
  it is already approved. The existing confirmation framework (§13) is reused
  exactly as-is.

## 17. Agent installer (Phase 2: automatic registration)

- **Purpose**: a production installer (`scripts/install.sh`) that downloads the
  latest released `opspilot-agent` binary from GitHub Releases, registers the
  agent with a Central server, persists the returned credentials into
  `/etc/opspilot/agent.yaml`, installs it as a systemd service, and verifies the
  install by polling the heartbeat and lease endpoints with HMAC-signed
  requests. The installer is idempotent and only mutates installer-owned config
  fields.
- **Platform detection**: Linux only; `uname -m` maps `x86_64 → amd64` and
  `aarch64 → arm64`. Anything else fails with
  `unsupported OS` / `unsupported architecture`. Root is required unless
  `OPSPILOT_ALLOW_NON_ROOT=1` (test only).
- **Release assets** (published on GitHub Releases per architecture):
  `opspilot-agent-linux-amd64`, `opspilot-agent-linux-arm64`. The asset is
  fetched via `curl -fsSL --retry 3`. A downloaded file that is empty or not a
  Linux ELF binary (checked via the `\x7fELF` magic) aborts the install.
  `OPSPILOT_LOCAL_BIN` may point at an existing binary instead of downloading
  (used by tests).
- **Binary install**: installed to `/usr/local/bin/opspilot-agent` with mode
  `0755` via `install -m 0755`.
- **Registration decision**: if `agent.yaml` contains both a non-empty
  `agent_id` and `secret`, the agent is considered already registered and the
  operator is asked `Re-register this agent? (y/N)` (default No). No → skip
  registration and preserve `agent_id`/`secret`/`signing_key`/
  `registration_token`, continuing with binary + service update (exit 0). Yes →
  re-register, consuming a fresh token and replacing the credentials (including
  a fresh `signing_key`).
- **Registration protocol**: builds the request body in a 0600 temp file (so
  secrets never appear in `ps`/argv) and POSTs it with curl `--data-binary @file`
  to `{central_url}/api/v1/agents/register`. The body carries
  `registration_token`, a client-generated `secret` (`rand_hex 32`, openssl or
  `/dev/urandom` fallback), `version`, and `server.{hostname,environment}`.
  Success is HTTP **201** returning `{"agent_id", "signing_key", ...}`; the
  `agent_id` and `signing_key` are parsed from it and a missing value aborts.
  Non-201 responses abort with the `message` extracted from the JSON error
  body. The Central URL and registration token are prompted; an existing
  `central_url` is the prompt default, and on a re-register a fresh token is
  required (the old token is consumed by the previous registration).
- **Input normalization**: every prompted value is normalized before use —
  leading/trailing whitespace and a trailing CR/LF are stripped; for the Central
  URL, trailing slashes are also removed (`http://host:9090/` →
  `http://host:9090`). Normalization uses `printf -v` in the current shell (no
  subshell) so it is safe between reads of a shared stdin pipe. The normalized
  Central URL is only printed when `OPSPILOT_DEBUG=1`.
- **Config persistence**: when `agent.yaml` does not exist, a full 0600 template
  is written; when it exists, only installer-owned fields are replaced
  (`central_url`, `registration_token`, `secret`, `signing_key`, `agent_id`,
  `version`) while operator-owned `server.hostname`/`server.environment` and
  extra sections (e.g. `projects`, `poll_interval`) are preserved. All rewrites
  keep mode `0600`.
- **Secrets hygiene**: the generated `secret`, the signing key and the
  registration token are never printed or logged; request/response bodies live
  only in a temp dir that is removed via `trap ... EXIT`.
- **System user**: the `opspilot` system user is created only when missing
  (`useradd --system --no-create-home --user-group --shell /bin/false
  opspilot`), guarded by `id opspilot`. The config directory and file are
  `chown`ed to `opspilot:opspilot` (best-effort, `|| true`).
- **systemd service**: `/etc/systemd/system/opspilot-agent.service` with
  `After=network-online.target` + `Wants=network-online.target`,
  `ExecStart=/usr/local/bin/opspilot-agent`, `Restart=always`, `RestartSec=5`,
  `User=opspilot`, `Group=opspilot`, `WorkingDirectory=/etc/opspilot`,
  `WantedBy=multi-user.target`. The service is started via
  `systemctl daemon-reload; enable; start`.
- **Verification**: after `start`, the installer signs each agent request with
  the persisted `signing_key` (HMAC-SHA256 over the canonical
  `agent_id "\n" timestamp "\n" nonce "\n" method "\n" path "\n" body`, sent via
  the `X-Agent-Id` / `X-Agent-Timestamp` / `X-Agent-Nonce` / `X-Agent-Signature`
  headers — the same protocol as the production agent, implemented in
  `sign_agent_request`). It polls the heartbeat endpoint
  (`{central_url}/api/v1/agents/heartbeat`) up to 5 times; on HTTP 200 it logs
  `agent heartbeat verified`, then performs a signed lease request
  (`{central_url}/api/v1/commands/lease`), logging `agent lease verified` on
  200/204. Signing requires `openssl`. On persistent failure it prints
  `systemctl status` + `journalctl` and aborts on the register path (warn-only
  on the skip-registration path). A legacy config without a `signing_key` skips
  the HMAC verification with a warning that re-registration is needed.
- **Idempotency**: safe to re-run — the binary and unit file are reinstalled and
  the service re-enabled, and an already-registered agent is never re-registered
  unless the operator answers Yes.
- **Testability**: installer path overrides `OPSPILOT_CONFIG_DIR`,
  `OPSPILOT_BIN_PATH`, `OPSPILOT_SERVICE_PATH`, `OPSPILOT_LOCAL_BIN`,
  `OPSPILOT_ALLOW_NON_ROOT`. `scripts/install-tests.sh` exercises the installer
  against a mock Central (validating the HMAC signatures) + stubbed system
  tools, covering successful registration, `signing_key` persistence,
  HMAC-signed heartbeat and lease verification, response parsing,
  invalid/unreachable/invalid-URL failures, config preservation, service
  start/stop behavior, re-registration prompts, and secret/token not being
  printed.
- **Tests**: `scripts/install-tests.sh` passes 60 assertions (including input
  normalization for trailing CR/LF, spaces, and a trailing slash, plus a
  normalize unit test); `scripts/install.sh`
  is clean under `shellcheck` and `bash -n`; the Go suite
  (`go build`, `go vet`, `go test`, plus `GOOS=linux` build/vet) is green.

## 18. Agent lifecycle: unregister

- **Purpose**: complete the agent lifecycle with a central-side unregister
  endpoint (`POST /api/v1/agents/unregister`) and a host-side uninstall script
  (`scripts/uninstall-agent.sh`). Registration, installation, and the agent
  runtime are unchanged.
- **Authentication**: the endpoint reuses the existing agent authentication —
  `agent_id` + `secret` verified against the stored Argon2id hash (same path as
  heartbeat and capability sync, §8). Unknown agents return
  `404 agent_not_found`; a secret mismatch returns `401 invalid_credentials`.
- **Success behavior** (`internal/application/agent/unregister.go` +
  `internal/infrastructure/postgres/agent_repository.go`):
  - `agents.status` transitions to `unregistered`.
  - The agent's capabilities are deleted (`DELETE FROM capabilities`).
  - Project metadata is removed. Today project profiles are agent-side only
    (§15) and have no central storage, so the unregister transaction deletes
    what central owns (capabilities) and the unregistered status gate rejects
    any future project-sync request through the same authentication path.
  - The transition and capability deletion run in a single transaction via
    `AgentRepository.UnregisterAgent`.
  - Historical data is preserved: command rows (including results, errors, and
    timestamps) are never touched. `commands` rows are kept because the agent
    row itself is never deleted (deletion would cascade to commands).
- **Idempotency**: unregistering an already-unregistered agent is a success
  (`200 {"status":"unregistered"}`). The repository `MarkAgentUnregistered`
  UPDATE is a no-op for an already-unregistered agent and the capability
  deletion is naturally idempotent.
- **Rejection of unregistered agents**: heartbeat and capability sync now
  reject agents whose status is `unregistered` with
  `401 invalid_credentials` (`internal/application/agent/heartbeat.go`,
  `internal/application/capability/capability.go`). This is the same gate any
  future project-sync endpoint will go through.
- **Database** (`sql/migrations/0010_agent_unregister.sql`): adds a CHECK
  constraint limiting `agents.status` to `online` / `offline` / `unregistered`
  and backfills existing agents to `online`.
- **Uninstall script** (`scripts/uninstall-agent.sh`, run as root):
  1. `systemctl stop opspilot-agent`
  2. `POST {central_url}/api/v1/agents/unregister` using `central_url`,
     `agent_id`, and `secret` from `/etc/opspilot/agent.yaml`
  3. On success it continues; on failure it prints a warning and asks
     `Continue uninstall? (Y/n)` — declining aborts the uninstall.
  4. `systemctl disable` + removes the systemd unit + `daemon-reload`
  5. Removes `/usr/local/bin/opspilot-agent`
  6. Asks `Remove configuration? (Y/n)` — only a `Y` removes `/etc/opspilot/`
  7. Logs are never removed.
- **Future work** (not implemented): `install-agent.sh` bootstrap, bootstrap
  tokens, the Add-Server workflow, and Hermes integration.
- **Verification**: `shellcheck` and `bash -n` clean; unregister success,
  unregister failure (continue / abort), configuration removal, and
  configuration preservation were exercised in an `ubuntu:24.04` container.

## 19. Command result retrieval (Phase 1)

- **Purpose**: expose a read-only endpoint for the command result/execution
  history without touching the agent or the execution pipeline.
- **Endpoint**: `GET /api/v1/commands/{id}`
  (`internal/transport/http/command.go` + `command_result_response.go`).
- **Use case** (`internal/application/command/get.go`): `GetCommandUseCase`
  validates the id (invalid id → `ErrInvalidCommandID`) and delegates to
  `Repository.GetCommand`; a missing command surfaces as
  `ErrCommandNotFound`.
- **Repository** (`internal/infrastructure/postgres/command_repository.go`):
  `GetCommand` runs the sqlc query `GetCommandResult` (returns the full row:
  id, agent_id, tool_name, payload, status, result, error, confirmation_status,
  leased_at, started_at, completed_at, confirmed_at, created_at, updated_at)
  and maps `pgx.ErrNoRows` → `ErrCommandNotFound`. No write path is touched
  and no new migration is required.
- **Result semantics**: `parameters` and `result` are `json.RawMessage` values
  passed through exactly as stored — the result is never recalculated or
  deserialized into tool-specific structures (workflow results are unchanged).
- **Response**: `200` returns the command state; `result` is omitted until the
  command completes, `error` until it fails, and `leased_at` / `completed_at`
  until set. Errors: `400 invalid_command_id`, `404 command_not_found`.
- **Verification**: use-case tests (pending / completed / failed passthrough,
  unknown, invalid id), postgres integration test (`GetCommand` lifecycle,
  opaque JSON, not found), and a transport test pinning the response contract
  (omitempty optional fields, raw JSON passthrough) — all green, plus
  `gofmt`, `go vet`, `go test ./...`, and `GOOS=linux go build ./...`.

## 20. Central installer (Phase 1)

- **Purpose**: a production installer (`scripts/install-central.sh`) that
  downloads the latest released `opspilot-central` binary from GitHub Releases
  and installs it as a systemd service on a Linux host.
- **Scope**: the installer does **not** install PostgreSQL, does **not** create
  a database, does **not** run migrations, and does **not** generate
  registration tokens. No Go code is involved; the config template is created
  for the operator to fill in, after which the service must be restarted.
- **Platform detection**: Linux only; `uname -m` maps `x86_64 → amd64` and
  `aarch64 → arm64`. Anything else fails with
  `unsupported OS` / `unsupported architecture`.
- **Release assets** (published on GitHub Releases per architecture):
  `opspilot-central-linux-amd64`, `opspilot-central-linux-arm64`. The asset is
  fetched from
  `https://github.com/tsee9iii/opspilot/releases/latest/download/<asset>`
  via `curl -fsSL --retry 3` — the same release strategy as the agent installer
  (§17). A downloaded file that is empty or not a Linux ELF binary (checked via
  the `\x7fELF` magic) aborts the install.
- **Binary install**: installed to `/usr/local/bin/opspilot-central` with mode
  `0755` via `install -m 0755`.
- **Config**: `/etc/opspilot/` is created if missing; a minimal
  `/etc/opspilot/central.yaml` template is written (mode `0600`) **only** when
  it does not already exist — an existing, operator-edited config is never
  overwritten. The template documents `server.host` / `server.port`
  (`0.0.0.0` / `8080`) and the `database` connection block.
- **System user**: the `opspilot` system user is created only when missing
  (`useradd --system --no-create-home --user-group --shell /bin/false
  opspilot`), guarded by `id opspilot`. The config directory and file are
  `chown`ed to `opspilot:opspilot`.
- **systemd service**: `/etc/systemd/system/opspilot-central.service` with
  `After=network-online.target` + `Wants=network-online.target`,
  `ExecStart=/usr/local/bin/opspilot-central`, `Restart=always`, `RestartSec=5`,
  `User=opspilot`, `Group=opspilot`, `WorkingDirectory=/etc/opspilot`,
  `WantedBy=multi-user.target`. The service is started via
  `systemctl daemon-reload; enable; start`.
- **Health check**: after starting, the installer polls
  `http://127.0.0.1:8080/health` every second for up to 15 seconds. If healthy
  it prints success; if not it prints a warning. A failed health check never
  fails the installation.
- **PostgreSQL verification**: the installer only checks whether `psql` is
  available. If not, it prints
  `PostgreSQL is not installed.` / `Central may not start until PostgreSQL is
  configured.` It never installs PostgreSQL, creates databases, or runs
  migrations.
- **Idempotency**: safe to re-run — re-running does not re-create the user and
  does not overwrite an existing config; the binary and unit file are simply
  reinstalled and the service re-enabled.
- **Completion output**: always prints the exact `Installation completed.` /
  `Next steps:` block directing the operator to edit
  `/etc/opspilot/central.yaml`, ensure PostgreSQL is available, restart, and
  verify the service.
- **Uninstall**: not implemented.
- **Verification**: `shellcheck` and `bash -n` clean; the full install flow
  (platform detection, download + ELF check, binary/user/config/service install,
  enable + start), config preservation, health-check success and warning paths,
  and the PostgreSQL check were exercised in an `ubuntu:24.04` container.

## 21. GitHub release pipeline

- **Purpose**: automatically build and publish release binaries whenever a Git
  tag is pushed. These binaries are the artifacts consumed by the installers
  (`scripts/install.sh`, §17, and `scripts/install-central.sh`, §20) via their
  `/releases/latest/download/<asset>` URLs.
- **Trigger** (`.github/workflows/release.yml`): runs only on
  `push: tags: - "v*"`.
- **Targets**: exactly `cmd/agent` and `cmd/central`, for
  `linux-amd64` and `linux-arm64`.
- **Build**: `CGO_ENABLED=0`, `GOOS=linux`, `GOARCH=amd64|arm64`,
  `-trimpath`, `-ldflags="-s -w"`; the Go toolchain version comes from the
  repository's `go.mod` (`go-version-file`).
- **Produced assets** (four, published per tag):
  `opspilot-agent-linux-amd64`, `opspilot-agent-linux-arm64`,
  `opspilot-central-linux-amd64`, `opspilot-central-linux-arm64`.
- **Verification before upload**: each built binary is checked to be
  executable (`test -x`) and non-empty (`test -s`); a missing or invalid
  binary fails the job and therefore the workflow.
- **Two jobs**:
  - **`build`** (matrix `agent`/`central` × `amd64`/`arm64`): builds and
    verifies each binary exactly as above, then uploads it to the workflow run
    with `actions/upload-artifact@v4` (`if-no-files-found: error`). Nothing is
    uploaded to GitHub Releases from this job.
  - **`release`** (`needs: build`): downloads all artifacts with
    `actions/download-artifact@v4` (`merge-multiple`), then
    `softprops/action-gh-release@v2` creates the Release for the pushed tag if
    it does not exist (or reuses the existing one) and uploads all four
    binaries in a single step. `overwrite: true` replaces assets on a re-run,
    so the workflow works whether or not the Release already exists.
- **Permissions**: uses the built-in `GITHUB_TOKEN` (`contents: write` — the
  minimum permission) in both jobs; no custom token and no source archives.
- **No application code**: the pipeline does not modify the Agent, Central,
  Tool Registry, Workflow Engine, installers, database, or HTTP API.
- **Verification**: `GOOS=linux GOARCH=amd64|arm64 go build ./cmd/agent` and
  `./cmd/central` all pass locally, producing statically linked, stripped ELF
  binaries whose names match the installer asset expectations.

## 22. Embedded migration framework

- **Purpose**: a built-in migration runner for Central that executes the SQL
  files in `sql/migrations/` with no external migration tool
  (no golang-migrate, goose, or migrate CLI).
- **Migration source**: `sql/migrations/*.sql` remain the single source of
  truth and are reused exactly as-is. `sql/migrations/embed.go` embeds every
  `.sql` file into the binary via `//go:embed *.sql`
  (`migrations.FS`); migrations are never read from disk at runtime.
- **Tracking**: the runner creates a `schema_migrations` table automatically if
  it does not exist (`version TEXT PRIMARY KEY`,
  `applied_at TIMESTAMPTZ NOT NULL DEFAULT now()`).
- **Ordering**: migrations run in lexicographical file-name order
  (`0001_… < 0002_… < … < 0010_…`), so the numbering defines the sequence.
- **Execution**: each migration runs inside its own transaction
  (`internal/migration/storage.go` — `Storage.apply`). The version is recorded
  in `schema_migrations` in the same transaction, so a failure rolls back both
  the migration and its bookkeeping row, stops immediately, and returns an
  error. Multi-statement files rely on pgx's simple protocol, which is
  automatically selected when a query has no arguments.
- **Idempotency**: already-applied versions are skipped (`Storage.AppliedVersions`
  read at the start of `Run`), so re-running is safe.
- **Runner** (`internal/migration/runner.go`): `Runner.Run(ctx)` applies all
  pending migrations and returns the versions applied; `Runner.Status(ctx)`
  returns applied and pending lists; `Runner.Migrations()` loads and sorts the
  embedded files. `NewRunner(source fs.FS, storage *Storage)` takes any
  `fs.FS`, which is what the tests exploit.
- **Bootstrap**: `bootstrap.New` runs pending migrations automatically after
  verifying database connectivity and before the HTTP server starts
  (`internal/bootstrap/app.go`). If migrations fail, Central terminates with a
  `bootstrap: run migrations:` error.
- **CLI** (`cmd/migrate`, binary `opspilot-migrate`): `up` applies pending
  migrations and prints each applied version (or `no pending migrations`);
  `status` prints the applied and pending lists. There is no `down` command
  and no rollback support.
- **No external dependency**: the framework uses only `embed`, `io/fs`, and
  pgx, all already in the module.
- **Verification**: integration tests run against a dedicated
  `opspilot_migration_test` database (so they never collide with the postgres
  package tests on the shared `opspilot` DB) and cover empty database,
  partially migrated, already up-to-date, failed-migration rollback (the failed
  version is never recorded and its partial DDL is rolled back), `schema_migrations`
  creation/columns, lexicographical ordering (a migration that depends on the
  previous one succeeds only if order is respected), embedded loading (10 files,
  sorted, non-empty), and `status`. `gofmt`, `go build`, `go vet`, `go test ./...`,
  `GOOS=linux go build ./...`, and `GOOS=linux go vet ./...` all pass.

## 23. Config loading (Phase 2)

**Problem**: `pkg/config.Load()` previously read environment variables only, so
the `/etc/opspilot/central.yaml` written by `scripts/install-central.sh` (see
§20) was never used.

**Precedence** (highest wins) — `internal` to `pkg/config/config.go`:

1. Built-in defaults.
2. YAML file — `/etc/opspilot/central.yaml`, overridden by the
   `OPSPILOT_CONFIG` environment variable.
3. Environment variables — always override YAML, with the same
   set-but-empty-is-unset semantics as before.

A missing YAML file is **not** an error: startup continues with defaults plus
environment variables (fully backwards compatible with env-only deployments).
An unreadable or invalid YAML file **is** an error.

**YAML format** — exactly the shape produced by `install-central.sh`, plus the
full option set supported by the spec:

```yaml
server:
  host: 0.0.0.0
  port: 8080
database:
  host: localhost
  port: 5432
  database: opspilot
  username: opspilot
  password: opspilot
  sslmode: disable
logger:
  level: info
auth:
  server_secret: change-me
```

Field mapping: `database.database` → `Config.Database.Name`
(`applyFile`), `database.username` → `Config.Database.User`,
`server.host` → `Config.HTTP.Host`, etc. Empty YAML values (the installer
template ships an empty `database:` block) fall back to the built-in
defaults, so the installer-generated file loads unchanged.

**Implementation**: no new dependency — `gopkg.in/yaml.v3` was already a direct
dependency. `Load()` builds `defaults()`, runs `loadFile` (returns `nil`
on `os.IsNotExist`, otherwise parses into a struct-tagged `fileConfig` via
`yaml.Unmarshal`, error on parse failure), applies non-zero file fields via
`applyFile`, then `applyEnv` overlays the environment last. The shared
`pkg/config` package is used by both `cmd/central` (via `bootstrap`) and
`cmd/agent` (for `OPSPILOT_ENV` / `OPSPILOT_LOG_LEVEL`); the agent reads its
own per-agent YAML separately (§9), so agent behavior is unchanged.

**Tests** (`pkg/config/config_test.go`, isolated via `clearEnv` + `t.Setenv` +
`t.TempDir`): defaults only, YAML only, environment only, YAML+environment
override (env wins), missing config file (no error), invalid YAML (error),
partial YAML (provided values honored, rest default), installer-generated YAML
(empty `database` block falls back to defaults), and `OPSPILOT_CONFIG` override.
`gofmt`, `go build ./...`, `go vet ./...`, `go test ./...`, `GOOS=linux
go build ./...`, and `GOOS=linux go vet ./...` all pass. End-to-end verified:
Central started reading port, database, and server secret from a YAML file
(`OPSPILOT_CONFIG`), migrated the schema, and served `/healthz` on the YAML-
configured port.

## 24. Registration token CLI (Phase 2)

**Problem**: registration tokens are required for agent onboarding, but the
only supported path was direct database access — the API has no token-create
endpoint, so operators could not provision agents without manually inserting
rows.

**Command** — the CLI is the official operator interface. `cmd/central`
dispatches on the first argument: `opspilot-central` starts the HTTP server
exactly as before; `opspilot-central token ...` runs a subcommand and exits.
There are no HTTP endpoints and no web UI for tokens.

```
opspilot-central token create [--environment <env>] [--expires <lifetime>]
opspilot-central token list
opspilot-central token revoke <token-id>
```

- **create** — generates a token `ops_rt_<base64url(32 random bytes)>`
  (≥256 bits of entropy via `crypto/rand`), prints the plain token exactly
  once under a `Registration Token` heading, and persists **only** the
  HMAC-SHA256 hex hash (`security.HMACHasher`, the same hash the registration
  flow looks up). Options: `--environment` (default `production`) and
  `--expires`, an optional lifetime parsed as a Go duration (`24h`, `90s`) or
  a whole number of days (`7d`, `30d`); the default lifetime is 30 days.
  Invalid lifetimes are rejected before anything is written.
- **list** — prints a table of `ID`, `Environment`, `Created At`, `Expires
  At`, `Revoked`, `Consumed`, most recently created first. Token values are
  never shown. `Consumed` is always `no`: consumption deletes the row, so a
  surviving token is by definition unconsumed. Expired tokens remain listed.
- **revoke** — sets `revoked_at = now()` on the token id; the row is kept
  (consumed tokens are already deleted by registration, so they are
  unaffected). Idempotent: revoking twice succeeds.

**Implementation** — `cmd/central/token` is a dedicated CLI package. It reuses
the existing `registrationtoken` domain, `RegistrationTokenRepository` (a new
`List` method was added for the list command, backed by a new
`ListRegistrationTokens` sqlc query against the existing table), the existing
HMAC hasher, and the shared `pkg/config` + `postgres.New` initialization (same
as `cmd/migrate`); no duplicate hashing, repository, or connection-pool code.
The HTTP server startup path in `cmd/central/main.go` is untouched. A database
that cannot be reached produces a clear `connect database:` error and exit
code 1; config-load failures produce `load config:`.

**Testing** — `cmd/central/token/token_test.go` covers create output format
(exactly three lines: heading, blank line, token), token format (`ops_rt_`
prefix, ≥32 bytes decoded, uniqueness), that the stored value is the HMAC of
the plain token (and never the token itself), default environment, the
`--environment` and `--expires` options (including `7d`/`30d`/`24h` and the
30-day default), invalid lifetimes, list output (no token values, revoked yes/
no, expired tokens still shown), revoke, revoke twice, invalid/missing ids,
unknown/missing subcommands, and clear bootstrap (config) and database-failure
errors. `internal/infrastructure/postgres/registration_token_list_integration_test.go`
covers `List` ordering, nullable `environment`, `revoked_at`, and that revoke
does not delete rows. `gofmt`, `go build ./...`, `go vet ./...`,
`go test ./...`, `GOOS=linux go build ./...`, and `GOOS=linux go vet ./...`
all pass.
