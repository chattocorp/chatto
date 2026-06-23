---
name: "chatto-architecture"
description: "Update docs/ARCHITECTURE.md to reflect the current state of the codebase by examining code and documentation."
---

# Update Architecture Documentation

Update `docs/ARCHITECTURE.md` to reflect the current state of the codebase. Examine the code and update the documentation while preserving the established structure and format.

## Document Structure

ARCHITECTURE.md follows this exact structure. Preserve all sections and their ordering:

1. **Table of Contents** - Auto-generated list of all sections with links (see below)
2. **Overview** - Brief description, Core Concepts cross-reference to `docs/GLOSSARY.md`
3. **NATS Authentication** - Auth methods table, Embedded/External setup
4. **Architecture & APIs** - Layer descriptions (NATS, Core, GraphQL, Web Client, Email)
5. **Core Services** - Current runtime services and their responsibilities
6. **Projection Inventory** - Registered projections, subject filters, nested read models, and primary readers
7. **ConnectRPC API Overview** - Protobuf-first public API inventory:
   - General description of the `/api/connect` mount and generated service paths
   - Explicit endpoint inventory table for every public ConnectRPC RPC
   - Authentication/authorization behavior for each endpoint
   - Source-of-truth links to API protos, generated handlers, service implementations, and HTTP mounting
8. **GraphQL API Overview** - High-level overview of the GraphQL API:
   - General description of the API's purpose and design
   - Key queries (spaces, rooms, messages, users, admin)
   - Key mutations (space/room/message CRUD, membership operations)
   - Key subscriptions (real-time events, presence updates)
   - No detailed field documentation - just the most important operations
9. **Architecture Pattern: Event-Sourced Writes** - Write Path table, Consistency Model
10. **Roles, Permissions, and Direct Messages** - Pointers to the authoritative FDRs/rules/source files
11. **NATS Resource Inventory** - Current runtime resources only:
   - **Current Resources**: Stream, KV, object-store, live-subject roots
   - **Event Envelopes**: Durable `corev1.Event` vs transient `corev1.LiveEvent`
   - **EVT Subject Patterns**: Current `evt.{aggregateType}.{aggregateId}.{eventType}` families and wildcard filters
   - **Durable EVT Event Inventory**: Explicit map of current EVT subject patterns to concrete protobuf event message types
   - **Transient Live Subjects**: Current `live.sync.>` subjects
   - **KV Buckets**: Current bucket key patterns only
   - **Object Store Buckets**: Current object-store key patterns only

Do not add detailed inventories for removed or historical pre-0.1 storage. `docs/ARCHITECTURE.md` describes the current version of the runtime architecture, not migration archaeology.

## Parallelization Strategy

Use parallel research where the active agent environment allows it. If subagents are available and the user has authorized delegation, launch multiple focused explorers. Otherwise, run the same searches locally and parallelize read-only shell commands where possible.

### Recommended Parallel Research Tasks

Recommended independent research slices:

1. **GraphQL Schema**: "Find all GraphQL queries, mutations, and subscriptions in `cli/internal/graph/*.graphqls`. List each operation with a one-line description."

2. **ConnectRPC API**: "Find all services and RPCs in `proto/chatto/api/v1/*.proto`, all generated handlers registered in `cli/internal/connectapi/api.go`, and the HTTP mount in `cli/internal/http_server/connect.go`. List each `/api/connect/{fully.qualified.Service}/{Method}` endpoint with auth/authorization behavior and a one-line description."

3. **RBAC**: "Find all permission constants in `cli/internal/core/permissions.go` and all `Can*` functions in `cli/internal/core/can.go`. Document each permission and what it controls."

4. **Core Services**: "Read `cli/internal/core/core.go`, `cli/internal/core/*_service.go`, and `cli/internal/video/service.go`. List current services with their responsibilities and lifecycle/readiness role."

5. **Projections**: "Read `NewChattoCore`, all `New*Projection` constructors, `Subjects()` methods, and `projection_subjects_test.go`. List registered projections, nested read models, and subject filters."

6. **Current Storage**: "Find all current `CreateOrUpdateStream`, `CreateOrUpdateKeyValue`, and `CreateOrUpdateObjectStore` calls in `cli/internal/core/`. List current resources and key/object patterns used by runtime code."

7. **EVT Subjects**: "Read `cli/internal/events/subjects.go`, `proto/chatto/core/v1/event.proto`, and `cli/internal/core/subjects/subjects.go`. List durable EVT subject patterns mapped to concrete protobuf event message types, plus transient `live.sync.>` subjects."

### After Parallel Research

Once research completes:
1. Read current `docs/ARCHITECTURE.md`
2. Merge subagent findings into the appropriate sections
3. Write the updated documentation

## Instructions

1. **Examine the codebase** to understand current state:

   - Read `proto/chatto/` for protobuf definitions (event types, messages)
   - Read `cli/internal/core/core.go` for service/projection/storage wiring
   - Read `cli/internal/core/*_service.go` for current core services
   - Read `cli/internal/events/subjects.go` for durable EVT subject and event-token definitions
   - Read `cli/internal/core/subjects/subjects.go` for transient live subject helpers
   - Read `cli/internal/core/projection_subjects_test.go` for projection subject policy
   - Read `cli/internal/graph/*.graphqls` for GraphQL schema (queries, mutations, subscriptions)
   - Read `docs/GLOSSARY.md` when changing or relying on canonical vocabulary in the Overview/Core Concepts section
   - Read `cli/internal/core/permissions.go` for permission definitions
   - Read `cli/internal/core/can.go` for permission check functions
   - Search for `CreateOrUpdateKeyValue`, `CreateOrUpdateStream`, `CreateOrUpdateObjectStore` calls
   - Search for `EventPublisher`, `publishLiveEvent`, and `LiveSync*` helpers to find current publish paths

2. **For GraphQL API Overview**:

   - Read all `.graphqls` files in `cli/internal/graph/`
   - Identify the most important queries, mutations, and subscriptions
   - Focus on user-facing operations, not internal details
   - Group by domain (spaces, rooms, messages, users, admin)

3. **For ConnectRPC API Overview**:

   - Read all public API `.proto` files in `proto/chatto/api/v1/`
   - Read `cli/internal/connectapi/api.go` to verify which generated service handlers are registered
   - Read `cli/internal/http_server/connect.go` to verify the `/api/connect` mount behavior
   - Read each implementation file in `cli/internal/connectapi/` to identify auth/authorization behavior and core service delegation
   - Maintain an explicit `Endpoint Inventory` table in `docs/ARCHITECTURE.md` with one row per mounted RPC
   - Derive endpoint paths as `/api/connect/{proto package}.{Service}/{Method}` (for example `/api/connect/chatto.api.v1.ServerService/GetServer`)
   - Include service name, RPC name, auth/authorization notes, and a concise behavior description
   - Remove rows for unmounted or deleted RPCs, and add rows whenever a new public ConnectRPC method is registered

4. **For RBAC Pointers**:

   - Read `cli/internal/core/permissions.go` for all permission constants
   - Read `cli/internal/core/can.go` for all `Can*` functions
   - Read `cli/internal/graph/authz.go` for GraphQL authorization helpers
   - Check `owners.emails` configuration in server setup
   - Keep `docs/ARCHITECTURE.md` as an inventory: link to the authoritative FDRs/rules/source files instead of duplicating the full RBAC model there

5. **For Overview and Glossary Cross-Reference**:

   - Keep `Core Concepts` as a pointer to `docs/GLOSSARY.md`; do not duplicate term definitions there
   - If architecture changes affect canonical terms such as Server, Room, Event, Projection, Subject, or Live Event, update `docs/GLOSSARY.md` in the same change
   - Keep glossary entries concise; link back to `docs/ARCHITECTURE.md` for detailed architecture

6. **For Core Services and Projections**:

   - Use `NewChattoCore` as the authoritative wiring point for current services and registered projections
   - Use `projection_subjects_test.go` to verify subject filters
   - Document nested read models under their registered parent projection

7. **For NATS Resource Inventory**:

   - Find all `CreateOrUpdateStream` calls to list streams
   - Find all `CreateOrUpdateKeyValue` calls to list KV buckets
   - Find all `CreateOrUpdateObjectStore` calls to list object stores
   - For `EVT`: use `cli/internal/events/subjects.go` and `proto/chatto/core/v1/event.proto` as the source of truth for subject patterns and concrete protobuf event message types
   - For transient live subjects: use `cli/internal/core/subjects/subjects.go` and `publishLiveEvent` callers
   - For each KV bucket: grep for `Get`, `Put`, `Create`, `Update` calls to find all key patterns
   - Document the naming conventions and variable placeholders
   - Remove detailed entries for resources that are not part of the current runtime architecture

8. **Compare with existing documentation**:

   - Read current `docs/ARCHITECTURE.md`
   - Identify discrepancies between code and documentation

9. **Update documentation**:

   - Add new entries to appropriate tables
   - Update existing entries if they've changed
   - Remove entries for deleted resources
   - Preserve markdown table formatting (aligned columns)
   - Keep notes/explanations accurate and concise
   - Add relative links to source files (see below)

10. **Validation checklist**:
   - All streams in code appear in Streams table
   - All KV buckets in code appear in KV Buckets table
   - All object stores in code appear in Object Store Buckets table
   - All core services in `NewChattoCore` appear in Core Services
   - All registered projections appear in Projection Inventory
   - Core Concepts points to `docs/GLOSSARY.md`
   - `docs/GLOSSARY.md` is updated if architecture vocabulary changed
   - Durable EVT event tokens in `events/subjects.go` appear in Durable EVT Event Inventory with their concrete protobuf event message type
   - Current `live.sync.>` subjects appear in Transient Live Subjects
   - RBAC section points to the authoritative FDRs/rules/source files
   - ConnectRPC API Overview has an `Endpoint Inventory` table
   - Every service/RPC in `proto/chatto/api/v1/*.proto` that is registered by `cli/internal/connectapi/api.go` appears in the ConnectRPC endpoint inventory with its full `/api/connect/...` path
   - ConnectRPC endpoint auth/authorization notes match the implementation in `cli/internal/connectapi/`
   - Key GraphQL operations are listed
   - Subject/key patterns match actual code usage
   - Detailed legacy/pre-0.1 storage inventories are not reintroduced

## Table of Contents

**Generate a Table of Contents at the beginning of the document** (after the title, before the first section). The ToC should:

- List all `##` (h2) sections as top-level items
- List all `###` (h3) subsections as nested items under their parent
- Use markdown links to the section anchors
- Keep it concise - don't include h4 or deeper headings

**Format:**

```markdown
## Table of Contents

- [Overview](#overview)
  - [Core Concepts](#core-concepts)
- [NATS Authentication](#nats-authentication)
  - [Embedded NATS](#embedded-nats)
  - [External NATS](#external-nats)
- [Architecture & APIs](#architecture--apis)
...
```

**Anchor rules:**

- Lowercase the heading text
- Replace spaces with hyphens
- Remove special characters except hyphens
- For headings with `&`, replace with nothing (e.g., "Architecture & APIs" → `#architecture--apis`)

## Table Formatting

**Always use aligned markdown tables** so the source markdown is pleasant to read. Align columns by padding cells with spaces:

```markdown
| Column1 | Column2 | Column3                      |
| ------- | ------- | ---------------------------- |
| value1  | value2  | Longer description text here |
| short   | x       | Another row                  |
```

Not like this (hard to read in source):

```markdown
| Column1 | Column2 | Column3 |
| --- | --- | --- |
| value1 | value2 | Longer description text here |
| short | x | Another row |
```

Notes appear after tables as plain text starting with "Notes:" when additional context is needed.

## Source File Links

Each major section should include relative links to the most important related source files. This helps readers (human or agent) quickly navigate to the implementation when learning about the architecture.

**Where to add links:**

- At the start of a section, list 2-5 key files as a "Key files:" line
- Use relative paths from the repository root
- Link to the most authoritative/central files, not every file that touches the topic

**Format:**

```markdown
## Core Services

Key files: [`cli/internal/core/core.go`](cli/internal/core/core.go), [`cli/internal/core/*_service.go`](cli/internal/core/), [`cli/internal/video/service.go`](cli/internal/video/service.go)

The Core Services section provides...
```

**Example sections and their key files:**

| Section | Key Files |
| ------- | --------- |
| Core / Architecture | `cli/internal/core/core.go` |
| Core Services | `cli/internal/core/core.go`, `cli/internal/core/*_service.go`, `cli/internal/video/service.go` |
| Projection Inventory | `cli/internal/core/core.go`, `cli/internal/core/projection_subjects_test.go`, `cli/internal/events/projector.go` |
| GraphQL API | `cli/internal/graph/*.graphqls`, `cli/internal/graph/resolver.go` |
| Roles and Permissions | `cli/internal/core/permissions.go`, `cli/internal/core/can.go`, `cli/internal/rbac/` |
| KV Buckets / Streams | `cli/internal/core/core.go`, `cli/internal/events/subjects.go`, `proto/chatto/core/v1/event.proto` |
| Messages | `cli/internal/core/rooms.go` |
| Encryption | `cli/internal/encryption/` |
