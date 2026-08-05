# ADR-0004: Agent Authentication

## Status

Accepted

## Context

Agents register once and then authenticate subsequent calls (heartbeat,
capability sync). Two problems must be solved:

1. **Replay resistance for registration** — a registration token seen in
   transit or logs must not be usable twice.
2. **Secrets at rest** — plaintext agent secrets must never be stored in
   PostgreSQL, and a database dump must not expose credentials that work
   against the API.

## Decision

- **Registration tokens** are stored only as
  `HMAC-SHA256(OPSPILOT_AUTH_SERVER_SECRET, token)` hex, never as plaintext.
  The server secret defaults to `dev-only-secret-change-me` and is read from
  `OPSPILOT_AUTH_SERVER_SECRET`.
- **Consumption** is atomic: registration `DELETE`s the token row in the same
  unit of work; a replay returns `409 token_already_used`.
- **Agent secrets** are stored as Argon2id hashes. Registration hashes the
  supplied secret before persisting; heartbeat and capability sync verify the
  presented secret against the stored hash.
- Failed identity/secret checks return `401 invalid_credentials`.

## Consequences

- Registration tokens cannot be replayed even if captured.
- No plaintext credentials are persisted; a database leak does not yield
  working agent secrets.
- Argon2id is deliberately expensive, so each authenticated call (heartbeat,
  capability sync) pays a verification cost — acceptable at current scale.
- The token is single-use, so a misconfigured agent that loses its `agent_id`
  must be re-provisioned with a fresh token.
