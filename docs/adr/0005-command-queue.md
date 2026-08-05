# ADR-0005: Command Queue

## Status

Accepted

## Context

Central dispatches commands to agents that are not always connected. The
queue must be durable (survive restarts of central and agents), claim work
exactly once when multiple agents poll concurrently, and keep an auditable
record of a command's lifecycle. Command execution is asynchronous — agents
pull work rather than central pushing it.

## Decision

Model the queue as rows in PostgreSQL with an explicit state machine and
enforce correctness in SQL.

- **States**: `pending → leased → running → completed | failed`, persisted on
  the `commands` row.
- **Creation**: `POST /api/v1/commands` inserts a `pending` row.
- **Leasing**: an atomic
  `UPDATE ... WHERE id = (SELECT ... ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED)`
  claims the oldest pending command for an agent and records `leased_at` and
  `lease_owner`. Concurrent agents can never claim the same row.
- **Transitions**: `start` / `complete` / `fail` run
  `UPDATE ... WHERE status = '<expected>'`, so a stale transition is a no-op
  that surfaces as `409 invalid_transition`. Ownership is enforced by an
  `agent_id` predicate (`403 command_not_owned`).
- Timestamps (`leased_at`, `started_at`, `completed_at`) and outputs
  (`result`, `error`) are stored on the command.

## Consequences

- The queue is durable and FIFO per agent; incomplete work stays `pending`
  and is picked up on the next poll, including after restarts.
- Concurrency is safe without application-level locking or distributed
  coordination.
- State integrity is enforced by SQL, so application bugs cannot silently
  corrupt transitions.
- Leases are currently permanent (no timeout/renewal), so a command never
  returns to `pending` after being leased even if its agent dies.
- Each poll requires a database round-trip; this is acceptable at current
  scale but does not scale to very high dispatch rates.
