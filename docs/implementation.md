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
| Tool Registry | — | `Register`/`Find`/`List` | Implemented |
| `system.uptime` tool | — | `/usr/bin/uptime` | Implemented |
| `system.memory` tool | — | `/proc/meminfo` | Implemented |
| `system.cpu` tool | — | `/proc/stat` | Implemented |
| `system.disk` tool | — | `statfs(2)` on `/` | Implemented |
| `system.processes` tool | — | `/proc/<pid>/*` | Implemented |
| `pm2.list` tool | — | `pm2 jlist` | Implemented |
| `pm2.logs` tool | — | `pm2 logs --nostream --raw` | Implemented |
| `pm2.restart` tool | — | `pm2 restart` | Implemented |
| Capability registration                          | `POST /api/v1/capabilities`     | startup sync             | Implemented     |
| WebSocket / Telegram / AI                        | —                               | —                        | Not Implemented |
| Docker / systemctl tools, sandboxing             | —                               | —                        | Not Implemented |
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
| POST   | `/api/v1/capabilities`      | agent_id+secret | `200 {status,count}`                     | 400, 401 `invalid_credentials`, 500                                          |

Request bodies:

- `register`: `registration_token`, `secret`, `version`, `server{hostname,environment}`
- `heartbeat`: `agent_id`, `secret`
- `create`: `agent_id`, `tool`, `payload` (JSON object)
- `lease`: `agent_id`
- `start`/`complete`/`fail`: `agent_id`, `command_id` (+ `result` for complete, `error` for fail)
- `capabilities`: `agent_id`, `secret`, `capabilities[{tool_name,version,description,parameter_schema,confirmation_level}]`

## 3. Database schema

Migrations (`sql/migrations/0001..0007`):

- `0001_init.sql` — `servers`, `agents`, `commands`
- `0002_agent_auth.sql` — `registration_tokens`
- `0003_command_lease.sql` — adds `commands.leased_at`, `commands.lease_owner`
- `0004_command_execution.sql` — adds `commands.started_at`, `commands.completed_at`
- `0005_capabilities.sql` — `capabilities`
- `0006_capability_parameter_schema.sql` — adds `capabilities.parameter_schema`
- `0007_capability_confirmation.sql` — adds `capabilities.confirmation_level`

Tables:

- **servers** — `id`, `name`, `hostname`, `environment`, `status`, `created_at`,
  `updated_at`; `UNIQUE (hostname, environment)`.
- **agents** — `id`, `server_id` FK, `secret` (Argon2id hash, see §8), `version`,
  `status`, `last_heartbeat`, `created_at`, `updated_at`.
- **commands** — `id`, `agent_id` FK, `tool_name`, `payload` JSONB, `status`,
  `result` JSONB, `error`, `leased_at`, `lease_owner`, `started_at`,
  `completed_at`, `created_at`, `updated_at`. State machine:
  `pending → leased → running → completed | failed`, enforced by atomic
  `UPDATE ... WHERE status = '<expected>'`.
- **registration_tokens** — `id`, `token_hash` UNIQUE (HMAC-SHA256 hex),
  `environment`, `expires_at`, `revoked_at`, `created_at`. Tokens are deleted
  on consumption; no `used_at` column.
- **capabilities** — `id`, `agent_id` FK, `tool_name`, `version`,
  `description`, `parameter_schema` JSONB (JSON Schema document),
  `confirmation_level` TEXT (`none` | `required`), `created_at`,
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
  next tick.
- SIGINT/SIGTERM cancel the context; the loops exit cleanly.

## 5. Tool Registry

- `internal/agent/registry.go` — `Tool` interface (`Name`, `Version`,
  `Description`, `ParameterSchema`, `ConfirmationLevel`, `Execute(ctx, payload)`)
  and a concurrency-safe `Registry` with `Register`, `Find`, `List` (sorted
  names). `ParameterSchema` returns the tool's accepted payload as a JSON
  Schema document; tools that take no payload return
  `{"type":"object","properties":{}}`. `ConfirmationLevel` returns the tool's
  confirmation metadata: `agent.ConfirmationNone` (`"none"`) for read-only
  tools and `agent.ConfirmationRequired` (`"required"`) for write tools. There
  is no execution-behavior change — confirmation is metadata only.
- `internal/agent/registry_executor.go` — the agent's `Executor`. It never
  switches on tool names: `Find → policy gate → Execute`. An unregistered name
  returns `tool not implemented`.
- Tools are grouped into category packages under `internal/agent/tools/` and
  registered once in `cmd/agent/main.go`:
  - `internal/agent/tools/system/` — `system.*` tools: `uptime.go`, `memory.go`,
    `cpu.go`, `disk.go` (+ `diskstat_linux.go`/`diskstat_other.go` build-tagged
    `statfs(2)`), `processes.go`.
  - `internal/agent/tools/pm2/` — `pm2.*` tools: `list.go`, `logs.go`,
    `restart.go`.
- Current tools (read-only tools — `system.*`, `pm2.list`, `pm2.logs` —
  advertise `confirmation_level = none`; the write tool `pm2.restart`
  advertises `confirmation_level = required`):
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
- `internal/agent/tool.go` — shared helpers used by the tool packages:
  `CommandResult{Stdout,Stderr,ExitCode}`, `EmptyParameterSchema` (the
  `{"type":"object","properties":{}}` schema constant), and `RunCommand`
  (context-aware `exec.CommandContext` wrapper); context expiry surfaces as a
  `tool timed out` error.

## 6. Capability registration

- Agent startup calls `registry.List()` and sends each tool's `name`, `version`,
  `description`, `parameter_schema`, and `confirmation_level` to
  `POST /api/v1/capabilities` (one request, batch body).
- Central (`internal/application/capability`) authenticates the agent (by id +
  secret) then upserts each capability (`ON CONFLICT (agent_id, tool_name) DO
  UPDATE`), returning the number persisted. The parameter schema is stored in
  `capabilities.parameter_schema` (JSONB) and the confirmation level in
  `capabilities.confirmation_level` (TEXT, validated to be `none` or
  `required`).

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
`execution_policy{enabled,timeout,allowed_commands,denied_commands,working_directory}`.

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
sql/migrations/         0001..0006
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
- Audit, alert, and confirmation tables/domains
- Additional tools (Docker, systemctl); tool sandboxing; tool version matrix
- Token rotation; metrics and observability beyond zap logs
- Migration tooling (`cmd/cli`, `migrate-*` Makefile targets)
