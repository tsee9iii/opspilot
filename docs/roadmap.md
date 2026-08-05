# opspilot — Roadmap

## Completed

- Monorepo bootstrap: `central` + `agent` binaries, env config, zap logger, pgx pool, HTTP router, Makefile, docker-compose
- Agent registration (one-time HMAC token, Argon2id secret) and heartbeat
- Command lifecycle: create (`pending`) → lease (`leased`) → start (`running`) → complete / fail
- Atomic, FIFO command leasing (`FOR UPDATE SKIP LOCKED`)
- Agent command poll loop
- Shell executor → replaced by Tool Registry
- Execution policy (enabled / timeout / allowed / denied / working directory)
- Tool Registry (`Register` / `Find` / `List`) + `system.uptime`
- Capability registration (agent startup → central)
- Schema migrations `0001`–`0005`, sqlc-generated queries
- Clean architecture layering (transport / application / domain / infrastructure)

## In Progress

- None — features are approved one at a time

## Planned

- WebSocket transport
- Telegram integration
- AI / DeepSeek command generation
- Additional tools (Docker, systemctl) and sandboxing
- Command results query API
- Token rotation
- Metrics and observability
