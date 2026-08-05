# ADR-0006: Tool Registry

## Status

Accepted

## Context

The agent's executor originally switched on tool names inside a single type.
Adding a tool meant editing that switch statement, and the set of supported
tools was invisible outside the agent. Two needs drove a redesign:

1. A generic extension point so new tools are additive rather than
   modifications to control flow.
2. Self-describing tool metadata so the agent can advertise its capabilities
   to central at startup.

## Decision

Introduce a generic Tool Registry on the agent.

- **`Tool` interface**: `Name()`, `Version()`, `Description()`, and
  `Execute(ctx, payload)`.
- **`Registry`**: concurrency-safe `Register(tool)`, `Find(name)`,
  `List()`.
- **`RegistryExecutor`**: the agent's executor never switches on names — it
  runs `Find → policy gate → Execute`. An unregistered name simply returns
  `tool not implemented`.
- **Policy**: the existing execution policy (enabled / allowed / denied /
  timeout) gates tools by name before execution.
- **Capabilities**: at startup the agent calls `Registry.List()` and sends
  each tool's name, version, and description to `POST /api/v1/capabilities`.
- The registry currently holds one tool: `system.uptime` (runs
  `/usr/bin/uptime`, returns `{"stdout","stderr","exit_code"}`).

## Consequences

- Adding a tool is a new `Tool` implementation registered in one place; no
  executor switch is edited.
- Authorization is uniform: a tool is either registered (executable) or not,
  and the policy can further restrict it by name.
- Tool metadata is a single source of truth used for both execution and
  capability advertisement.
- Executing an unknown tool name fails with a fixed `tool not implemented`
  error rather than a compile-time error, so typos surface at runtime.
