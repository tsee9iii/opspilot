# ADR-0002: Clean Architecture

## Status

Accepted

## Context

The platform's business rules (registration, heartbeat, command state
machine, capability sync) must not depend on the HTTP framework, the database
driver, or any other infrastructure detail. We need to unit-test business
logic without a live database and to be able to swap infrastructure without
touching application code.

## Decision

Organize `central` in a clean/hexagonal style with dependencies pointing
inward:

- `internal/transport/http` — HTTP handlers and DTOs only; no business logic
- `internal/application/` — use cases plus the repository interfaces they
  declare (e.g. `command.Repository`, `capability.CapabilityRepository`)
- `internal/domain/` — plain entities (agent, server, command,
  registrationtoken) with no infrastructure dependencies
- `internal/infrastructure/postgres` — sqlc-backed repositories that
  implement the application-layer interfaces
- `internal/infrastructure/security` — HMAC and Argon2id hashers
- `internal/bootstrap` — the composition root that constructs and wires every
  dependency

## Consequences

- Use cases are testable with fakes (e.g. the capability `SyncUseCase` is
  unit-tested against fake repositories and a fake hasher).
- Infrastructure is swappable; nothing above the repository layer knows
  PostgreSQL specifics.
- Dependencies are explicit and injected, which keeps `transport` free of
  business rules.
- The layering adds mapping boilerplate between transport DTOs, application
  request/response types, and domain entities.
