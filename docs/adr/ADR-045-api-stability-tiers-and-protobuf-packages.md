# ADR-045: API Stability Tiers and Protobuf Packages

**Date:** 2026-06-28

## Context

ADR-042 established protobuf-first APIs for ConnectRPC request/response calls
and the realtime websocket protocol. The first migration completed by moving
the bundled frontend from GraphQL to protobuf APIs, but the resulting
`chatto.api.v1` package mixed several contracts:

- a broad first-party app API optimized for the bundled web client;
- websocket realtime frames;
- the future long-lived integration API intended for bots, SDKs, tooling, and
  alternate clients.

Those contracts need different stability promises. The bundled frontend can use
screen-shaped payloads and capability-rich response models, but it still needs
mixed-version tolerance for self-hosted deployments. External integrations need
a smaller, cleaner API with stricter compatibility rules and less repeated
frontend DTO shape.

## Decision

Chatto will split protobuf API packages by compatibility tier.

`chatto.app.v1` contains the current first-party app ConnectRPC surface. These
services are mounted under `/api/connect` and are used by the bundled frontend.
The app API remains compatibility-sensitive because hosted clients and
self-hosted servers can drift, but it may keep UI-oriented payloads, viewer
capability bundles, admin screen DTOs, and other frontend workflow shapes.

`chatto.realtime.v1` contains binary websocket frames for `/api/realtime`. The
protocol is documented separately from ConnectRPC services. Realtime frames are
public protocol messages, not raw persisted `corev1` facts. They may import
stable app-level enums where doing so avoids duplicating wire meanings during
the transition.

`chatto.integration.v1` is reserved for the long-lived external integration
API. It will be introduced deliberately with stricter conventions for canonical
shared types, lookup semantics, pagination, mutation responses, and error-code
stability. Existing app RPCs are not automatically part of the integration API.

Generated documentation follows the same split:

- integration API overview/reference;
- app ConnectRPC reference;
- realtime protocol reference.

## Consequences

The current app can keep moving without accidentally making every frontend DTO a
long-lived integration promise.

External API design becomes a separate review track. Before an RPC enters
`chatto.integration.v1`, reviewers can ask whether its types, pagination,
absence semantics, mutation responses, and errors match the stricter
integration conventions.

Package names appear in ConnectRPC service paths. Moving the migrated app
surface from `chatto.api.v1` to `chatto.app.v1` is a wire-visible path change,
acceptable before the new integration API is promised stable. The bundled
frontend and generated clients move with the server in the same release.

Realtime gets a separate namespace and documentation page. This reduces the
size and ambiguity of the generated ConnectRPC reference, but the realtime
package can still depend on app-level enum definitions until Phase B creates
canonical integration/shared type guidance.
