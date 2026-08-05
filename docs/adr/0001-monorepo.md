# ADR-0001: Monorepo

## Status

Accepted

## Context

opspilot ships two executables: `central`, the control plane that owns the
database and command queue, and `agent`, which runs on managed hosts and
executes commands. Both need shared configuration loading, structured logging,
database access, and domain types, and the two must evolve in lockstep (an
agent feature almost always implies a central API change).

A multi-repository setup would force versioned cross-repo dependencies and
coordinated releases for every feature.

## Decision

Use a single Go module monorepo.

- Module: `github.com/opspilot/opspilot`
- Entrypoints: `cmd/central` and `cmd/agent`
- Shared code lives under `internal/` and is used by both binaries
- One build, one test run, one version history

## Consequences

- A feature is implemented and verified across both binaries in a single
  change; there is no release-coordination overhead.
- `go build ./...` and `go test ./...` cover the whole platform.
- Shared internal packages are not importable by external consumers, which is
  acceptable for a closed platform.
- The dependency graph and build surface grow together, so a dependency
  pulled in by one side affects the other.
