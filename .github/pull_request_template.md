## Why

<!-- What user, operator, integrator, or maintainer problem does this solve? -->

## What changed

<!-- Summarise observable behaviour and important implementation decisions. -->

<!-- For Chatto MCP catalog changes, apply FDR-043's tool-admission checklist.
Include the concrete workflow and catalog gap, cost/output limits, human
effects, canonical operations, authorization/privacy, retry/outcomes, primitive
choice, compatibility review, and repeatable real-host tester evidence.
Update FDR-043 and the complete catalog review in docs/MCP-INTEROPERABILITY.md.
ConnectRPC API completeness and CRUD symmetry are not MCP goals. Existing
tester exceptions do not admit another tool or tool class. -->

## API compatibility

<!-- Delete options that do not apply. Cover auth, discovery, API, admin, realtime, and public HTTP semantics. -->

- No public API or protocol behaviour changed.
- Additive and compatible.
- Behavioural change; temporal client/server impact described below.
- Deprecated without removal.
- Breaking experimental API change; compatibility plan and migration guidance included.

Older client → newer server:

Newer client → older server:

Capability discovery or minimum client/server version:

## Test plan

<!-- List exact checks and results, plus any verification still outstanding. -->
