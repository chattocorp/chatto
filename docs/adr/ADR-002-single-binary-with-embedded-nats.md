# ADR-002: Single Binary with Embedded NATS Server

**Date:** 2026-03-01

## Context

Chatto targets self-hosters and small teams who want a chat system they can run with minimal infrastructure. Typical chat applications require multiple services: a web server, a database, a message broker, a file store, and possibly a cache. Each adds deployment complexity, monitoring burden, and failure modes.

NATS server is written in Go and can be embedded as a library in the same process that runs application code.

## Decision

Embed the NATS server directly in the Chatto Go binary. A single executable starts the NATS server in-process, the HTTP API/realtime server, and the SvelteKit frontend (served as embedded static files). The only requirement is a port and a data directory.

For advanced deployments, an external NATS cluster can be used instead — the application connects as a regular NATS client regardless of whether the server is in-process or remote.

## Consequences

- **Single-command deployment**: `chatto serve` starts everything. No Docker Compose, no service orchestration required for basic use.
- **Reduced failure modes**: No network calls between the app and its data store in single-node mode. Eliminates connection pool management, reconnection logic, and network partitions for the common case.
- **Horizontal scaling is still possible**: Multiple Chatto instances can connect to an external NATS cluster. The embedded server is a convenience, not a constraint.
- **Memory footprint includes NATS**: The process uses more memory than a thin web server, since it also runs the JetStream storage engine.
- **Upgrades are atomic**: One binary to update, one process to restart. No version compatibility matrix between app and database.
- **Advanced features can be extracted**: The KMS service, for example, runs in-process by default but can be deployed standalone for high-security environments. The embedded pattern is the starting point, not the ceiling.
