# Authling Runtime Architecture Inventory

This directory records Authling's current runtime components and operational
contracts. Keep planned architecture in ADRs until it is implemented.

## Current Runtime

The [`authling` command](../../cmd/authling/main.go) currently exposes help and
version output only. It does not start HTTP services, implement OpenID Connect,
connect to NATS, or create JetStream resources.
