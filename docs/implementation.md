# opspilot — Repository Reference

Reference of what is currently implemented. Unimplemented areas are stated
explicitly as **Not Implemented**.

## Executables

- `cmd/central` — the only entrypoint. Loads `.env`, boots config/logger/pool,
  serves HTTP, graceful shutdown on SIGINT/SIGTERM.
- `cmd/agent` — **Not Implemented.**

## Layer map

| Layer | Package | Contents |
|---|---|---|
| Transport | `internal/transport/http` | Router, `/healthz` + agent-register handlers, request/response DTOs, JSON error envelope |
| Application | `internal/application/agent` | `RegisterUseCase`, `Repository` interface, request/response types |
| Domain | `internal/domain/agent` | `Agent` |
| Domain | `internal/domain/server` | `Server` |
| Domain | `internal/domain/command` | `Command` |
| Domain | `internal/domain/registrationtoken` | `RegistrationToken` |
| Infrastructure | `internal/infrastructure/postgres` | Pool factory, `AgentRepository`, `RegistrationTokenRepository` |
| Bootstrap | `internal/bootstrap` | Composition root: repository → use case → handler |
| Shared | `pkg/config` | Env-based config loader |
| Shared | `pkg/logger` | Zap factory |

Domain entities use `uuid.UUID` identifiers, `[]byte` for payload/result,
and carry no JSON/DB tags, validation, or behavior.

## HTTP API

- `GET /healthz` — `200`.
- `POST /api/v1/agents/register` — validates body, delegates to
  `RegisterUseCase`, persists server + agent in one transaction, returns
  `201 {"agent_id","status"}`. Errors: `{"error":{"code","message"}}`
  (400/500). Body: `secret`, `version`, `server.hostname`,
  `server.environment`; `agent_id` is DB-generated.

## Persistence

Migrations (`sql/migrations`):

- `0001_init.sql` — `servers`, `agents`, `commands`; unique
  `servers(hostname, environment)`.
- `0002_agent_auth.sql` — `registration_tokens`
  (`token_hash` UNIQUE, `expires_at`, nullable `environment`/`revoked_at`).
  Consumed tokens are deleted, no `used_at` column.

sqlc (pgx/v5) generates `gen/postgresql`; queries live in `sql/queries`.

- `AgentRepository.RegisterAgent` — one transaction: upsert server by
  (hostname, environment), insert agent, commit, rollback on failure.
  Secret is stored as-is; hashing is **Not Implemented**.
- `RegistrationTokenRepository` — `Create`, `FindByHash`, `Consume` (atomic
  delete returning whether a row existed), `Revoke`. **Not wired** to any
  endpoint or use case.

## Configuration

Env vars with defaults: `OPSPILOT_ENV`, `OPSPILOT_HTTP_HOST/PORT`,
`OPSPILOT_DB_HOST/PORT/USER/PASSWORD/NAME/SSLMODE`, `OPSPILOT_LOG_LEVEL`.

`Config.Auth.ServerSecret` is declared but **not populated** by `Load()` and
is not consumed anywhere.

## Infrastructure

- `deployments/docker-compose.yml` — PostgreSQL 16, `opspilot` bridge
  network, port 5432, named volume, healthcheck.
- Postgres pool: `MaxConns 10`, `MinConns 2`; connectivity check is the
  caller's responsibility.
- `Makefile` — `all/tidy/fmt/vet/lint/build/build-central/build-agent/test/
  run-central/run-agent/dev-up/dev-down/sqlc-generate/clean/help`.

## Dependencies

`go 1.24` / `toolchain go1.25.0`. Direct: `google/uuid`, `jackc/pgx/v5`,
`joho/godotenv`, `go.uber.org/zap`.

## Not Implemented

- `cmd/agent`; WebSocket and the shared `pkg/protocol` package
- Telegram; Hermes runtime; DeepSeek client
- Authentication: no middleware, no registration-token enforcement on the
  register endpoint, no secret hashing (plaintext in DB), no HTTP endpoints
  for registration tokens (repository only), no rotation/heartbeat/revocation
- Audit, alert, confirmation tables and domains
- Migration tooling (`cmd/cli`, `migrate-*` Makefile targets)
