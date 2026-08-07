# opspilot — Roadmap

Implementation detail lives in `docs/implementation.md`; this file only tracks
what is done and what is next.

## Completed

- Monorepo bootstrap: `central`, `agent`, `mcp` and `migrate` binaries, config
  (YAML + env), zap logger, pgx pool, HTTP router, Makefile, docker-compose
- Agent registration (one-time HMAC token) and heartbeat
- HMAC request signing for every agent request (`internal/agentsign`), with
  timestamp window and nonce replay rejection
- Agent lifecycle: unregister endpoint + host uninstall script
- Command lifecycle: create (`pending`) → lease (`leased`) → start (`running`) →
  complete / fail; atomic, FIFO leasing (`FOR UPDATE SKIP LOCKED`)
- Confirmation policy and central-side enforcement (pending commands are never
  leased)
- Operator authentication (`OperatorAuth` bearer token) plus the
  `X-Operator-Actor` audit actor on every operator route
- Immutable command audit trail (`source`, `requested_by`, `requested_at`,
  `approved_by`, `approved_at`, `approval_note`)
- Command results query API (`GET /api/v1/commands/{id}`)
- Execution policy (enabled / timeout / allowed / denied / working directory)
- Tool Registry (`Register` / `Find` / `List` / `ListMetadata`) with JSON Schema
  payload validation, capability availability, and semantic catalog metadata
- Tools: `system.*`, `pm2.*`, `docker.*` (incl. `docker.inspect`),
  `systemctl.*`, `journal.logs`, `git.*`, `http.check` (SSRF-hardened),
  `file.read`, `filesystem.list`, `workflow.diagnose`, `workflow.deploy`,
  `deploy.project`
- Project Profiles and the workflow engine, plus the docker-compose / pm2 /
  script deploy strategies
- Capability registration (agent startup → central)
- Agent health reporting + `GET /api/v1/health`
- Alert evaluation, acknowledge API, and signed outbound webhooks
- SSE command wake-ups (`GET /api/v1/agents/events`) with polling fallback
- MCP stdio server with the `inventory` / `investigate` / `operate` mode tiers
- Embedded migration framework (`0001`–`0014`), sqlc-generated queries
- Registration token CLI (`opspilot-central token create/list/revoke`)
- Installers (`install.sh`, `install-central.sh`), CI, and the GitHub release
  pipeline (six binaries per tag)
- Clean architecture layering (transport / application / domain / infrastructure)

## In Progress

- None — features are approved one at a time

## Planned

- Fix `scripts/uninstall-agent.sh` to HMAC-sign its unregister request (it is
  currently rejected with `401`; see implementation §11)
- `restart.project` tool
- MCP `workflow_rollback` / `workflow_backup` / `workflow_upgrade` and the
  workflows behind them
- Multi-instance central wake-ups (PostgreSQL `LISTEN`/`NOTIFY` or a broker)
- Command history / capability listing over HTTP
- WebSocket transport
- Telegram integration
- AI / DeepSeek command generation
- Tool sandboxing and a tool version matrix
- Token rotation
- Metrics and observability beyond zap logs
- Migration `down` / rollback support; a central uninstall script
