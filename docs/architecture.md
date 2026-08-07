# opspilot — Architecture

This document describes the **current** implementation only. Planned features are intentionally not covered.

## Overview

opspilot is a Go monorepo with three executables:

- **central** — the control plane. Exposes a JSON HTTP API, owns the PostgreSQL database, and holds the command queue.
- **agent** — runs on managed hosts. Registers itself, reports heartbeats and health, advertises its capabilities, and polls for, executes, and reports commands.
- **mcp** — an MCP stdio server exposing the control plane to the Hermes assistant. Its toolset is mode-gated and always fails closed on mutations.

```mermaid
flowchart LR
    subgraph Host
        A[agent]
    end
    subgraph Control Plane
        C[central HTTP API]
        P[(PostgreSQL)]
        M[mcp server]
    end
    A -- HTTPS/JSON --> C
    C -- SQL --> P
    M -- SQL --> P
    H[Hermes assistant] -- MCP stdio --> M
```

## Module layout

```
cmd/
  central/            central binary entrypoint
  agent/              agent binary entrypoint
  mcp/                MCP stdio server entrypoint
internal/
  bootstrap/          central composition root, lifecycle, graceful shutdown
  transport/http/     HTTP handlers, DTOs, routing (central)
  application/        use cases (agent, command, capability, health, alert)
  domain/             entities (agent, server, command, registrationtoken)
  infrastructure/
    postgres/         sqlc-backed repositories, pgx pool
    security/         HMAC (tokens) and Argon2id (secrets) hashers
    webhook/          outbound signed webhook delivery (alerts)
  agent/              agent runtime, tool registry, executor, policy, health collector
  mcp/tools/          MCP tool definitions and mode-gated toolset builder
pkg/
  config/             env-based configuration
  logger/             zap logger
gen/postgresql/       sqlc-generated query code (checked in)
sql/
  migrations/         0001..0014 schema migrations
  queries/            annotated SQL consumed by sqlc
```

## Central

### Layering

Central follows a clean/hexagonal style. Dependencies point inward: `transport → application → domain`, with `infrastructure` implementing the interfaces the application layer declares.

```mermaid
flowchart TB
    HTTP[HTTP router + handlers]
    UC[application use cases]
    DOM[domain entities]
    REPO[postgres repositories]
    SEC[security hashers]
    DB[(PostgreSQL)]

    HTTP --> UC
    UC --> DOM
    UC --> REPO
    UC --> SEC
    REPO --> DB
```

### Composition root

`internal/bootstrap` wires everything: loads config → builds logger → opens the pgx pool (`MaxConns=10`, `MinConns=2`) → constructs repositories, hashers, and use cases → starts the HTTP server and shuts down gracefully on SIGINT/SIGTERM.

### HTTP API

| Method | Path                    | Auth        | Success response |
| ------ | ----------------------- | ----------- | ---------------- |
| GET    | `/healthz`              | —           | `200 ok`         |
| POST   | `/api/v1/agents/register` | token      | `201 {agent_id, status}` |
| POST   | `/api/v1/agents/heartbeat` | agent_id+secret | `200 {status, next_heartbeat}` |
| POST   | `/api/v1/agents/health` | agent_id+secret | `200 {status}` |
| POST   | `/api/v1/commands`      | operator    | `201 {command_id, status}` |
| POST   | `/api/v1/commands/approve` | operator | `200 {command_id, status}` |
| POST   | `/api/v1/commands/lease` | —          | `200 {command_id, tool, payload}` or `204` |
| POST   | `/api/v1/commands/start` | —          | `200 {command_id, status}` |
| POST   | `/api/v1/commands/complete` | —        | `200 {command_id, status}` |
| POST   | `/api/v1/commands/fail` | —           | `200 {command_id, status}` |
| POST   | `/api/v1/capabilities`  | agent_id+secret | `200 {status, count}` |
| GET    | `/api/v1/alerts`        | operator    | `200 {alerts, total}` |
| POST   | `/api/v1/alerts/{id}/acknowledge` | operator | `200 {alert}` |

Errors use a consistent envelope: `{"error":{"code","message"}}`. Command transition endpoints enforce ownership and state: `404 not_found`, `403 command_not_owned`, `409 invalid_transition`. Creating a command requires the operator bearer token (`operatorAuth` middleware, constant-time compare; failures return `401` without leaking whether a token was valid). Every operator route also requires the `X-Operator-Actor` header (1–128 chars, control chars rejected) so each audit record names the human or system that acted. Capability failures surface as `400 capability_not_found` when a tool has no registered capability and `409 capability_unavailable` when a registered capability is disabled — the command is never created.

### Command lifecycle

Commands flow through a state machine persisted in PostgreSQL. Transitions are atomic `UPDATE ... WHERE status = 'expected'` statements; leasing uses `FOR UPDATE SKIP LOCKED` (FIFO by `created_at`) so concurrent agents never claim the same row.

Every command carries immutable audit fields recorded at creation: `source` (`api` / `mcp` / `system`), `requested_by`, `requested_at`. Confirmation-required commands are released only by an operator: `POST /commands/approve` writes `approved_by`, `approved_at` and an optional `approval_note` exactly once at the pending→approved transition.

```mermaid
stateDiagram-v2
    [*] --> pending: create (source, requested_by)
    pending --> leased: lease
    pending --> approved: operator approves
    leased --> running: start
    running --> completed: complete (result)
    running --> failed: fail (error)
    approved --> leased: lease
    completed --> [*]
    failed --> [*]
```

Commands created through the MCP path are always pending and are never self-approved: they wait for an independent operator to approve them, even for tools whose capability metadata would otherwise auto-approve.

### Database schema

```mermaid
erDiagram
    servers ||--o{ agents : has
    agents ||--o{ commands : receives
    agents ||--o{ capabilities : exposes
    agents ||--o{ registration_tokens : "consumed by"
    agents ||--o{ agent_health : "latest report"
    agents ||--o{ alerts : fires

    servers {
        uuid id PK
        text name
        text hostname
        text environment
        text status
    }
    agents {
        uuid id PK
        uuid server_id FK
        text secret "Argon2id hash"
        text version
        text status
        timestamptz last_heartbeat
    }
    commands {
        uuid id PK
        uuid agent_id FK
        text tool_name
        jsonb payload
        text status
        jsonb result
        text error
        timestamptz started_at
        timestamptz completed_at
        timestamptz leased_at
        text lease_owner
        text source "api|mcp|system"
        text requested_by
        timestamptz requested_at
        text approved_by
        timestamptz approved_at
        text approval_note
    }
    capabilities {
        uuid id PK
        uuid agent_id FK
        text tool_name
        text version
        text description
    }
    registration_tokens {
        uuid id PK
        text token_hash "HMAC-SHA256 hex"
        text environment
        timestamptz expires_at
        timestamptz revoked_at
    }
    agent_health {
        uuid agent_id PK
        uuid server_id FK
        timestamptz reported_at
        text status
        text agent_version
        text hostname
        text environment
        numeric cpu_user_percent
        numeric cpu_system_percent
        numeric cpu_idle_percent
        numeric memory_used_percent
        numeric disk_used_percent
        jsonb snapshot "raw report, verbatim"
    }
    alerts {
        uuid id PK
        uuid agent_id FK
        uuid server_id FK
        text rule_type
        text severity
        text status "open|acknowledged|resolved"
        text message
        timestamptz first_seen_at
        timestamptz last_seen_at
        timestamptz resolved_at
        timestamptz acknowledged_at
        text acknowledged_by
    }
```

### Security

- **Registration tokens**: a one-time token is stored as `HMAC-SHA256(OPSPILOT_AUTH_SERVER_SECRET, token)` (hex). Registration consumes it atomically; replay returns `409 token_already_used`.
- **Agent secrets**: plaintext secrets are never stored. Registration persists an Argon2id hash; heartbeat and capability sync verify against it.
- **Fail-closed configuration**: central refuses to start with development defaults (`OPSPILOT_AUTH_SERVER_SECRET` / `OPSPILOT_OPERATOR_TOKEN` / `OPSPILOT_DB_PASSWORD` unset), with `sslmode=disable`, or binding `0.0.0.0` in production. Validation errors name the offending variable but never its value.
- **Operator-authenticated command creation**: `POST /api/v1/commands` requires the operator bearer token so only authenticated operators can enqueue commands.
- **Fail-closed capability resolution**: a command for a tool with no registered capability is rejected (`capability_not_found`); a command for a registered-but-disabled capability is rejected (`capability_unavailable`). Capabilities are never implicitly approved.
- **MCP modes**: the MCP toolset is built from a mode that defaults to `inventory` and only ever grows. `inventory` exposes pure reads over PostgreSQL (ping, inventory, health, alerts) and never contacts agents. `investigate` adds read-only agent tools (`file_read`, `filesystem_list`, `docker_inspect`, `workflow_diagnose`) that are still policy-enforced. `operate` adds `workflow_deploy`; any command created through MCP is always recorded as `source=mcp` and stays pending until an operator approves it — it is never self-approved. The deprecated `OPSPILOT_MCP_READ_ONLY` flag maps `true→inventory` / `false→operate`; an explicit `mode` always wins. The MCP service should run as a least-privilege database role.
- **Operator audit actor**: every operator-authenticated route (command create/approve, alert list/acknowledge) requires the `X-Operator-Actor` header, and the actor is persisted on the affected record so approval chains are attributable.
- **Webhook delivery**: outbound alert webhooks are disabled by default, require HTTPS in production, sign each raw payload with HMAC-SHA256 (`X-Opspilot-Signature`), carry an event id (`X-Opspilot-Event-Id`) for idempotency, and retry at most three times with backoff. Response bodies are never logged.

### Agent health reporting

Each agent runs a health collector on `health_report_interval` (default 60s), independent of the heartbeat and command-poll loops. It gathers CPU, memory and disk metrics from the local tools and optionally probes configured project health endpoints (via the SSRF-hardened `http.check` tool), then POSTs the report to `/api/v1/agents/health`. Central stores at most one report per agent (each report overwrites the previous) so it always holds the latest snapshot for alert evaluation. The raw report body is stored verbatim as `snapshot`.

`GET /api/v1/alerts` and the MCP inventory tools read central state only — they never contact agents, so a fleet-wide health or alert query cannot trigger traffic to managed hosts.

### Alert lifecycle

A background evaluator in central periodically walks all agents with their latest health report and applies configured rules. Rules are inert unless enabled with a non-zero threshold:

- `agent_offline` — last heartbeat older than `max_offline` (severity critical by default).
- `disk_usage` — latest report `disk_used_percent` above `threshold_percent`.
- `health_report_stale` — last health report older than `max_report_age` even though the agent is online.
- `project_unhealthy` — parses the `project_health` section of the raw snapshot and fires when any configured probe reports unhealthy.

```mermaid
stateDiagram-v2
    [*] --> open: rule fires
    open --> acknowledged: operator acknowledges
    open --> resolved: report recovers / rule clears
    acknowledged --> resolved: report recovers / rule clears
    resolved --> [*]
```

Alerts are idempotent per `(agent_id, rule_type)` while open: a repeated unhealthy report advances `last_seen_at` instead of opening a duplicate. Recovery resolves both open and acknowledged alerts. Acknowledging is an authenticated operator API action only — the MCP server has no acknowledge tool. Only state transitions emit webhook events (open, resolve, acknowledge).

## Agent

### Startup

```mermaid
sequenceDiagram
    participant A as agent
    participant C as central

    alt no persisted agent_id
        A->>C: POST /api/v1/agents/register
        C-->>A: 201 {agent_id}
    end
    A->>A: registry.List()
    loop for each tool
        A->>C: POST /api/v1/capabilities
        C-->>A: 200 {status, count}
    end
    A->>A: start heartbeat loop (goroutine)
    A->>A: start health report loop (goroutine, interval)
    A->>A: start command poll loop (main)
```

### Tool execution

The agent never switches on tool names. Execution is `Registry → Find → Execute`:

```mermaid
flowchart LR
    LOOP[command poll loop]
    REG[Tool Registry]
    POL[Execution Policy]
    TOOL[system.uptime]
    RES[result JSON]

    LOOP -->|lease, start| LOOP
    LOOP --> REG
    REG -->|Find name| REG
    REG -->|Execute| TOOL
    REG -->|Allow name + timeout| POL
    TOOL --> RES
```

The execution policy gates every tool by name (`enabled` / `allowed_commands` / `denied_commands`) and bounds execution with `timeout`. Any unregistered tool name returns `tool not implemented`.

### Read-only tool hardening

The file and HTTP tools are fail-closed:

- **`file.read` / `filesystem.list`** resolve paths relative to a configured project root; `..` traversal and symlink escapes are rejected. Absolute paths are denied by default and only honoured when the operator sets `filesystem.allow_absolute_paths: true`.
- **`http.check`** is SSRF-hardened. Restricted ranges (loopback, link-local incl. cloud metadata `169.254.169.254`, RFC1918 private, CGNAT, multicast, reserved) are denied by default. Every resolved IP is validated before connecting, the connection is pinned to the validated IP (defeating DNS rebinding), redirects are never followed, and response bodies/headers are never returned. Configuring `http_check.allow_endpoints` / `allow_hosts` / `allow_cidrs` restricts the tool to exactly those destinations; `http_check.allow_private` opts back into private ranges.

### Transport security

In a production environment (`server.environment: production`) the agent refuses to start unless `central_url` uses `https://`; `allow_insecure_central: true` is the development-only escape hatch for a TLS-terminating proxy.

### Command loop and SSE wake-ups

The agent leases commands from central on an interval and on SSE wake-ups. Each cycle: lease one command → `start` it → execute the tool → `complete` with the result or `fail` with the error. An empty queue (`204`) just sleeps until the next wake-up or poll tick.

**SSE wake-up channel.** By default (`sse_enabled: true`) the agent also keeps a signed SSE stream open to `GET /api/v1/agents/events` on central. Central pushes a heartbeat every 15s and an `event: wakeup` whenever a leasable command appears for that agent; the agent responds by leasing immediately. This drops interactive Milo/Hermes command latency from the poll interval to roughly one authenticated round trip.

- **Wake-up only, never command data.** The SSE payload is just `{"agent_id": ..., "reason": "command_available"}`. Command payloads, secrets, approval decisions, and results are never sent over SSE — the agent always fetches the next command from the signed `POST /api/v1/commands/lease` endpoint, which remains the source of truth. A lost, duplicated, or reordered wake-up is therefore harmless; the fallback poll and the next wake-up reconcile anyway.
- **Single active stream per agent.** A reconnecting agent replaces its previous stream (idle streams are canceled), so duplicate connections can never double-deliver work.
- **Polling stays as fallback.** `poll_interval` (default 30s) is a recovery mechanism for startup, SSE disconnects, or a central without the SSE handler. On any SSE disconnect the agent wakes immediately so a command is not delayed. If SSE is disabled, lower `poll_interval` (e.g. 2–5s) since polling becomes the only delivery path.
- **Reconnect policy.** On disconnect the agent reconnects with exponential backoff (1s initial, 30s max, ±30% jitter), resetting to 1s after 60s of stable connection.
- **Timeout handling.** Central's global HTTP server write timeout (~30s) is cleared for the SSE handler only via `http.NewResponseController` and re-armed as a 30s per-write deadline; non-SSE endpoints keep their existing timeout.
- **Notifier wiring.** Central notifies through an in-memory `AgentNotifier` from the command application layer, so both the HTTP API and the in-process MCP dispatch path (which funnels through the same create/approve use cases) trigger wake-ups. Commands created by MCP operators are always `pending` and never wake an agent at creation; approval of a pending command wakes the target agent.
- **Single-process scope.** The notifier is in-memory, so wake-ups are only delivered while the agent's stream is connected to the same central process. Multi-instance central would need PostgreSQL `LISTEN`/`NOTIFY` or a broker; polling remains correct regardless.

## Configuration

| Binary   | Source                      | Keys (examples) |
| -------- | --------------------------- | --------------- |
| central  | env (`OPSPILOT_*`)          | `OPSPILOT_HTTP_PORT`, `OPSPILOT_DB_HOST`, `OPSPILOT_AUTH_SERVER_SECRET`, `OPSPILOT_OPERATOR_TOKEN`, `OPSPILOT_ALERTS_ENABLED`, `OPSPILOT_ALERTS_INTERVAL_SECONDS`, `OPSPILOT_ALERTS_DISK_USAGE_THRESHOLD_PERCENT` (and sibling per-rule vars), `OPSPILOT_WEBHOOK_URL` / `OPSPILOT_WEBHOOK_SECRET` |
| agent    | YAML (`configs/agent.example.yaml`) | `central_url`, `registration_token`, `secret`, `sse_enabled`, `poll_interval`, `health_report_interval`, `execution_policy`, `allow_insecure_central`, `filesystem.allow_absolute_paths`, `http_check.*` |
| mcp      | env (`OPSPILOT_*`)          | `OPSPILOT_DB_HOST`, `OPSPILOT_MCP_MODE` (`inventory` default, `investigate`, `operate`); deprecated `OPSPILOT_MCP_READ_ONLY` |

## Installer hardening

The installers (`scripts/install.sh`, `scripts/install-central.sh`) create dedicated `opspilot` system users and systemd units. Central runs with `NoNewPrivileges`, `PrivateTmp`, `ProtectHome`, and `ProtectSystem=strict` (only `/etc/opspilot` writable). The agent additionally needs to write project repositories and connect to Docker/PM2, so it runs with the safe subset (`NoNewPrivileges`, `PrivateTmp`) to avoid breaking deployments.

## Development tooling

- **sqlc v1.31.1** generates `gen/postgresql` from `sql/queries` + `sql/migrations` (`make sqlc-generate`).
- **Makefile** targets: `build`, `test`, `vet`, `run-central`, `run-agent`, `dev-up` (PostgreSQL via `deployments/docker-compose.yml`).
- **Logging**: zap — console encoding in development, JSON in production.

### Verification and CI

- The authoritative full verification command is `go test ./...` on an unrestricted CI/Linux runner. A clean result there is the release gate; nothing is deployed without it.
- HTTP integration tests use `httptest.NewServer`, which binds a loopback port. In a restricted sandbox that blocks loopback networking these tests fail for **environment reasons, not code reasons** — rerun them on an unrestricted runner before treating a failure as a regression.
- PostgreSQL-backed integration tests read `OPSPILOT_TEST_DATABASE_URL` (default `postgres://opspilot:opspilot@localhost:5432/opspilot?sslmode=disable`) and skip themselves when the database is unreachable. CI provisions a PostgreSQL service, so these tests run for real there.
- CI runs on Ubuntu and executes `gofmt` validation, `go vet ./...`, and `go test ./...` (see `.github/workflows/ci.yml`). CI must pass before production deployment.
