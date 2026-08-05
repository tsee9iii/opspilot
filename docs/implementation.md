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
| Agent installer (Phase 1)                        | —                               | `scripts/install.sh`     | Implemented     |
| Central installer (Phase 1)                      | `scripts/install-central.sh`    | —                        | Implemented     |
| Capability registration                          | `POST /api/v1/capabilities`     | startup sync             | Implemented     |
| WebSocket / Telegram / AI                        | —                               | —                        | Not Implemented |
| Docker SDK, sandboxing                           | —                               | —                        | Not Implemented |
| Command results query API                        | `GET /api/v1/commands/{id}`     | —                       | Implemented     |
| Token rotation, metrics                          | —                               | —                        | Not Implemented |

## 2. HTTP API

All JSON. Errors use `{"error":{"code","message"}}`.

| Method | Path                        | Auth            | Success                                  | Error codes                                                                  |
| ------ | --------------------------- | --------------- | ---------------------------------------- | ---------------------------------------------------------------------------- |
| GET    | `/healthz`                  | —               | `200 ok`                                 | —                                                                            |
| POST   | `/api/v1/agents/register`   | token           | `201 {agent_id,status}`                  | 400 `validation_error`, 401 `invalid_token`, 409 `token_already_used`, 500   |
| POST   | `/api/v1/agents/heartbeat`  | agent_id+secret | `200 {status,next_heartbeat}`            | 400, 401 `invalid_credentials`, 500                                          |
| POST   | `/api/v1/agents/unregister` | agent_id+secret | `200 {status:"unregistered"}`           | 400, 401 `invalid_credentials`, 404 `agent_not_found`, 500                   |
| POST   | `/api/v1/commands`          | —               | `201 {command_id,status}`                | 400, 500                                                                     |
| POST   | `/api/v1/commands/lease`    | —               | `200 {command_id,tool,payload}` or `204` | 400, 500                                                                     |
| POST   | `/api/v1/commands/start`    | —               | `200 {command_id,status}`                | 400, 403 `command_not_owned`, 404 `not_found`, 409 `invalid_transition`, 500 |
| POST   | `/api/v1/commands/complete` | —               | `200 {command_id,status}`                | same as `start`                                                              |
| POST   | `/api/v1/commands/fail`     | —               | `200 {command_id,status}`                | same as `start`                                                              |
| POST   | `/api/v1/commands/approve`  | —               | `200 {status}`                           | 400 `validation_error`, 404 `command_not_found`, 500                          |
| GET    | `/api/v1/commands/{id}`    | —               | `200 {command}`, see below               | 400 `invalid_command_id`, 404 `command_not_found`, 500                        |
| POST   | `/api/v1/capabilities`      | agent_id+secret | `200 {status,count}`                     | 400, 401 `invalid_credentials`, 500                                          |

Request bodies:

- `register`: `registration_token`, `secret`, `version`, `server{hostname,environment}`
- `heartbeat`: `agent_id`, `secret`
- `unregister`: `agent_id`, `secret`
- `create`: `agent_id`, `tool`, `payload` (JSON object)
- `lease`: `agent_id`
- `start`/`complete`/`fail`: `agent_id`, `command_id` (+ `result` for complete, `error` for fail)
- `approve`: `command_id`
- `capabilities`: `agent_id`, `secret`, `capabilities[{tool_name,version,description,parameter_schema,confirmation_level,available,unavailable_reason}]`

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

Migrations (`sql/migrations/0001..0010`):

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
- **Agent secrets**: stored as Argon2id hashes. Registration hashes the secret;
  heartbeat, capability sync, and unregister verify the presented secret
  against the stored hash (`401 invalid_credentials` on mismatch). Agents whose
  status is `unregistered` are rejected with `401 invalid_credentials` on
  heartbeat and capability sync (§18).
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
- The `deploy.project` (and `diagnose.project`/`restart.project`) tool that
  wires the Project Profiles and workflow engine (§15, §16) into the registry
  and command pipeline
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

## 17. Agent installer (Phase 1)

- **Purpose**: a production installer (`scripts/install.sh`) that downloads the
  latest released `opspilot-agent` binary from GitHub Releases and installs it
  as a systemd service on a Linux host.
- **Scope**: the installer does **not** register the agent, does **not** contact
  the Central server, and does **not** clone the repository. No Go code is
  involved; the config template is created for the operator to fill in, after
  which the service must be restarted.
- **Platform detection**: Linux only; `uname -m` maps `x86_64 → amd64` and
  `aarch64 → arm64`. Anything else fails with
  `unsupported OS` / `unsupported architecture`.
- **Release assets** (published on GitHub Releases per architecture):
  `opspilot-agent-linux-amd64`, `opspilot-agent-linux-arm64`. The asset is
  fetched from `https://github.com/opspilot/opspilot/releases/latest/download/<asset>`
  via `curl -fsSL --retry 3`. A downloaded file that is empty or not a Linux
  ELF binary (checked via the `\x7fELF` magic) aborts the install.
- **Binary install**: installed to `/usr/local/bin/opspilot-agent` with mode
  `0755` via `install -m 0755`.
- **Config**: `/etc/opspilot/` is created if missing; a minimal
  `/etc/opspilot/agent.yaml` template is written (mode `0600`) **only** when it
  does not already exist — an existing, operator-edited config is never
  overwritten.
- **System user**: the `opspilot` system user is created only when missing
  (`useradd --system --no-create-home --user-group --shell /bin/false
  opspilot`), guarded by `id opspilot`. The config directory and file are
  `chown`ed to `opspilot:opspilot`.
- **systemd service**: `/etc/systemd/system/opspilot-agent.service` with
  `After=network-online.target` + `Wants=network-online.target`,
  `ExecStart=/usr/local/bin/opspilot-agent`, `Restart=always`, `RestartSec=5`,
  `User=opspilot`, `Group=opspilot`, `WantedBy=multi-user.target`. The service
  is started via `systemctl daemon-reload; enable; start`.
- **Idempotency**: safe to re-run — re-running does not re-create the user and
  does not overwrite an existing config; the binary and unit file are simply
  reinstalled and the service re-enabled.
- **Completion output**: always prints the exact `Installation completed.` /
  `Next step:` block directing the operator to edit `/etc/opspilot/agent.yaml`
  and run `sudo systemctl restart opspilot-agent`.
- **Verification**: `shellcheck` and `bash -n` clean; the full install flow
  (platform detection, download + ELF check, binary/user/config/service install,
  enable + start) and the idempotency behavior were verified in an
  `ubuntu:24.04` container.

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
