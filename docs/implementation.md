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
| Capability registration                          | `POST /api/v1/capabilities`     | startup sync             | Implemented     |
| WebSocket / Telegram / AI                        | —                               | —                        | Not Implemented |
| Docker SDK, sandboxing                           | —                               | —                        | Not Implemented |
| Command results query API                        | —                               | —                        | Not Implemented |
| Token rotation, metrics                          | —                               | —                        | Not Implemented |

## 2. HTTP API

All JSON. Errors use `{"error":{"code","message"}}`.

| Method | Path                        | Auth            | Success                                  | Error codes                                                                  |
| ------ | --------------------------- | --------------- | ---------------------------------------- | ---------------------------------------------------------------------------- |
| GET    | `/healthz`                  | —               | `200 ok`                                 | —                                                                            |
| POST   | `/api/v1/agents/register`   | token           | `201 {agent_id,status}`                  | 400 `validation_error`, 401 `invalid_token`, 409 `token_already_used`, 500   |
| POST   | `/api/v1/agents/heartbeat`  | agent_id+secret | `200 {status,next_heartbeat}`            | 400, 401 `invalid_credentials`, 500                                          |
| POST   | `/api/v1/commands`          | —               | `201 {command_id,status}`                | 400, 500                                                                     |
| POST   | `/api/v1/commands/lease`    | —               | `200 {command_id,tool,payload}` or `204` | 400, 500                                                                     |
| POST   | `/api/v1/commands/start`    | —               | `200 {command_id,status}`                | 400, 403 `command_not_owned`, 404 `not_found`, 409 `invalid_transition`, 500 |
| POST   | `/api/v1/commands/complete` | —               | `200 {command_id,status}`                | same as `start`                                                              |
| POST   | `/api/v1/commands/fail`     | —               | `200 {command_id,status}`                | same as `start`                                                              |
| POST   | `/api/v1/commands/approve`  | —               | `200 {status}`                           | 400 `validation_error`, 404 `command_not_found`, 500                          |
| POST   | `/api/v1/capabilities`      | agent_id+secret | `200 {status,count}`                     | 400, 401 `invalid_credentials`, 500                                          |

Request bodies:

- `register`: `registration_token`, `secret`, `version`, `server{hostname,environment}`
- `heartbeat`: `agent_id`, `secret`
- `create`: `agent_id`, `tool`, `payload` (JSON object)
- `lease`: `agent_id`
- `start`/`complete`/`fail`: `agent_id`, `command_id` (+ `result` for complete, `error` for fail)
- `approve`: `command_id`
- `capabilities`: `agent_id`, `secret`, `capabilities[{tool_name,version,description,parameter_schema,confirmation_level,available,unavailable_reason}]`

## 3. Database schema

Migrations (`sql/migrations/0001..0009`):

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

Tables:

- **servers** — `id`, `name`, `hostname`, `environment`, `status`, `created_at`,
  `updated_at`; `UNIQUE (hostname, environment)`.
- **agents** — `id`, `server_id` FK, `secret` (Argon2id hash, see §8), `version`,
  `status`, `last_heartbeat`, `created_at`, `updated_at`.
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
- **Agent secrets**: stored as Argon2id hashes. Registration hashes the secret;
  heartbeat and capability sync verify the presented secret against the stored
  hash (`401 invalid_credentials` on mismatch).
- Command create/lease/transition endpoints are **not authenticated** (no
  secret check); capability sync and heartbeat are.

## 9. Configuration

Central — env vars (`pkg/config`), with defaults:

`OPSPILOT_ENV`, `OPSPILOT_HTTP_HOST`/`PORT`, `OPSPILOT_DB_HOST`/`PORT`/`USER`/
`PASSWORD`/`NAME`/`SSLMODE`, `OPSPILOT_LOG_LEVEL`, `OPSPILOT_AUTH_SERVER_SECRET`
(default `dev-only-secret-change-me`).

Agent — YAML (`configs/agent.example.yaml`):
`central_url`, `registration_token`, `secret`, `version`,
`server{hostname,environment}`, `agent_id`, `poll_interval`,
`execution_policy{enabled,timeout,allowed_commands,denied_commands,working_directory}`,
and the optional `projects` section (see §15).

## 10. Project structure

```
cmd/central/            central binary
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
sql/migrations/         0001..0009
sql/queries/            annotated SQL for sqlc
deployments/            docker-compose (PostgreSQL 16)
docs/                   architecture, implementation, roadmap, adr/
```

## 11. Technical debt

- `agents.secret` still names the Argon2id hash column `secret` (renaming to
  `secret_hash` was deferred in migration `0002`).
- Command endpoints are unauthenticated.
- `execution_policy.working_directory` is inert in the registry-driven flow.
- Leases are permanent: a leased command never returns to `pending`, even if
  its agent dies (no lease timeout/renewal).
- `assertTransition` reads the command, then updates with an atomic `WHERE`;
  state is re-checked in SQL but the read is an extra round-trip.
- Migrations are static files; there is no migration runner in the binary.
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
- Token rotation; metrics and observability beyond zap logs
- Migration tooling (`cmd/cli`, `migrate-*` Makefile targets)

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
