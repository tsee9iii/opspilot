# opspilot — Architecture

This document describes the **current** implementation only. Planned features are intentionally not covered.

## Overview

opspilot is a Go monorepo with two executables:

- **central** — the control plane. Exposes a JSON HTTP API, owns the PostgreSQL database, and holds the command queue.
- **agent** — runs on managed hosts. Registers itself, reports heartbeats, advertises its capabilities, and polls for, executes, and reports commands.

```mermaid
flowchart LR
    subgraph Host
        A[agent]
    end
    subgraph Control Plane
        C[central HTTP API]
        P[(PostgreSQL)]
    end
    A -- HTTPS/JSON --> C
    C -- SQL --> P
```

## Module layout

```
cmd/
  central/            central binary entrypoint
  agent/              agent binary entrypoint
internal/
  bootstrap/          central composition root, lifecycle, graceful shutdown
  transport/http/     HTTP handlers, DTOs, routing (central)
  application/        use cases (agent, command, capability)
  domain/             entities (agent, server, command, registrationtoken)
  infrastructure/
    postgres/         sqlc-backed repositories, pgx pool
    security/         HMAC (tokens) and Argon2id (secrets) hashers
  agent/              agent runtime, tool registry, executor, policy
pkg/
  config/             env-based configuration
  logger/             zap logger
gen/postgresql/       sqlc-generated query code (checked in)
sql/
  migrations/         0001..0011 schema migrations
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
| POST   | `/api/v1/commands`      | operator    | `201 {command_id, status}` |
| POST   | `/api/v1/commands/lease` | —          | `200 {command_id, tool, payload}` or `204` |
| POST   | `/api/v1/commands/start` | —          | `200 {command_id, status}` |
| POST   | `/api/v1/commands/complete` | —        | `200 {command_id, status}` |
| POST   | `/api/v1/commands/fail` | —           | `200 {command_id, status}` |
| POST   | `/api/v1/capabilities`  | agent_id+secret | `200 {status, count}` |

Errors use a consistent envelope: `{"error":{"code","message"}}`. Command transition endpoints enforce ownership and state: `404 not_found`, `403 command_not_owned`, `409 invalid_transition`. Creating a command requires the operator bearer token (`operatorAuth` middleware, constant-time compare; failures return `401` without leaking whether a token was valid). Capability failures surface as `400 capability_not_found` when a tool has no registered capability and `409 capability_unavailable` when a registered capability is disabled — the command is never created.

### Command lifecycle

Commands flow through a state machine persisted in PostgreSQL. Transitions are atomic `UPDATE ... WHERE status = 'expected'` statements; leasing uses `FOR UPDATE SKIP LOCKED` (FIFO by `created_at`) so concurrent agents never claim the same row.

```mermaid
stateDiagram-v2
    [*] --> pending: create
    pending --> leased: lease
    leased --> running: start
    running --> completed: complete (result)
    running --> failed: fail (error)
    completed --> [*]
    failed --> [*]
```

### Database schema

```mermaid
erDiagram
    servers ||--o{ agents : has
    agents ||--o{ commands : receives
    agents ||--o{ capabilities : exposes
    agents ||--o{ registration_tokens : "consumed by"

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
```

### Security

- **Registration tokens**: a one-time token is stored as `HMAC-SHA256(OPSPILOT_AUTH_SERVER_SECRET, token)` (hex). Registration consumes it atomically; replay returns `409 token_already_used`.
- **Agent secrets**: plaintext secrets are never stored. Registration persists an Argon2id hash; heartbeat and capability sync verify against it.
- **Fail-closed configuration**: central refuses to start with development defaults (`OPSPILOT_AUTH_SERVER_SECRET` / `OPSPILOT_OPERATOR_TOKEN` / `OPSPILOT_DB_PASSWORD` unset), with `sslmode=disable`, or binding `0.0.0.0` in production. Validation errors name the offending variable but never its value.
- **Operator-authenticated command creation**: `POST /api/v1/commands` requires the operator bearer token so only authenticated operators can enqueue commands.
- **Fail-closed capability resolution**: a command for a tool with no registered capability is rejected (`capability_not_found`); a command for a registered-but-disabled capability is rejected (`capability_unavailable`). Capabilities are never implicitly approved.
- **MCP read-only by default**: the MCP server exposes only read-only tools (ping, inventory, `file_read`, `filesystem_list`, `docker_inspect`). Execution tools (`workflow_diagnose`, `workflow_deploy`) are registered only when `OPSPILOT_MCP_READ_ONLY=false`. The MCP service should run as a least-privilege database role.

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

### Command loop

The agent polls on a configurable interval. Each iteration: lease one command → `start` it → execute the tool → `complete` with the result or `fail` with the error. An empty queue (`204`) just sleeps until the next tick.

## Configuration

| Binary   | Source                      | Keys (examples) |
| -------- | --------------------------- | --------------- |
| central  | env (`OPSPILOT_*`)          | `OPSPILOT_HTTP_PORT`, `OPSPILOT_DB_HOST`, `OPSPILOT_AUTH_SERVER_SECRET`, `OPSPILOT_OPERATOR_TOKEN` |
| agent    | YAML (`configs/agent.example.yaml`) | `central_url`, `registration_token`, `secret`, `poll_interval`, `execution_policy`, `allow_insecure_central`, `filesystem.allow_absolute_paths`, `http_check.*` |
| mcp      | env (`OPSPILOT_*`)          | `OPSPILOT_DB_HOST`, `OPSPILOT_MCP_READ_ONLY` (default `true`) |

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
