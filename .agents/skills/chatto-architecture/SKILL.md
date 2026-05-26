---
name: "chatto-architecture"
description: "Update docs/ARCHITECTURE.md to reflect the current state of the codebase by examining code and documentation."
---

# Update Architecture Documentation

Update `docs/ARCHITECTURE.md` to reflect the current state of the codebase. Examine the code and update the documentation while preserving the established structure and format.

## Document Structure

ARCHITECTURE.md follows this exact structure. Preserve all sections and their ordering:

1. **Table of Contents** - Auto-generated list of all sections with links (see below)
2. **Overview** - Brief description, Core Concepts (Instance, Spaces, Rooms, Users)
3. **NATS Authentication** - Auth methods table, Embedded/External setup
4. **Architecture & APIs** - Layer descriptions (NATS, Core, NATS API, GraphQL, Web Client, Email)
5. **NATS API Layer** - Package Structure table, Subject Pattern, Services table, Request/Response Format, GraphQL Integration
6. **GraphQL API Overview** - High-level overview of the GraphQL API:
   - General description of the API's purpose and design
   - Key queries (spaces, rooms, messages, users, admin)
   - Key mutations (space/room/message CRUD, membership operations)
   - Key subscriptions (real-time events, presence updates)
   - No detailed field documentation - just the most important operations
7. **Architecture Pattern: CRUD + Event Publishing** - Write Path table, Consistency Model
8. **Roles and Permissions (RBAC)** - Comprehensive RBAC documentation:
   - **Instance-Level Roles**: Owner configuration via `owners.emails`, what instance owners can access
   - **Space-Level Roles**: Role definitions, how roles are assigned to space members
   - **Available Permissions**: Table of all permissions with descriptions
   - **Permission Checks**: How `Can*` functions work in the codebase
9. **NATS JetStream Inventory** - Complete inventory of all JetStream resources:
   - **Streams**: Table listing all streams with their purpose
     - For each stream: all subjects it captures and what events flow through them
   - **KV Buckets**: Table listing all KV buckets with their purpose
     - For each bucket: all key patterns used and what data they store
   - **Object Store Buckets**: Table listing all object stores with their purpose
   - Include subject/key pattern syntax (e.g., `space.{spaceId}.room.{roomId}.message.created`)

## Parallelization Strategy

**Use subagents extensively to speed up research.** Launch multiple Task tool calls in parallel to gather information from different parts of the codebase simultaneously.

### Recommended Parallel Research Tasks

Launch these subagents **in parallel** at the start:

1. **GraphQL Schema Agent** (Explore): "Find all GraphQL queries, mutations, and subscriptions in `cli/internal/graph/schema/*.graphqls`. List each operation with a one-line description."

2. **RBAC Agent** (Explore): "Find all permission constants in `cli/internal/core/permissions.go` and all `Can*` functions in `cli/internal/core/can.go`. Document each permission and what it controls."

3. **JetStream Streams Agent** (Explore): "Find all `CreateOrUpdateStream` calls in `cli/internal/core/`. For each stream, find all `Publish` calls that write to it. List streams with their subjects."

4. **JetStream KV Agent** (Explore): "Find all `CreateOrUpdateKeyValue` calls in `cli/internal/core/`. For each KV bucket, find all key patterns used with `Get`, `Put`, `Create`, `Update`. List buckets with their key patterns."

5. **Object Stores Agent** (Explore): "Find all `CreateOrUpdateObjectStore` calls in `cli/internal/core/`. Document each object store and its purpose."

6. **NATS API Services Agent** (Explore): "Find all NATS micro services in `cli/internal/core_api/`. List each service with its subject pattern and operations."

### After Parallel Research

Once all subagents return:
1. Read current `docs/ARCHITECTURE.md`
2. Merge subagent findings into the appropriate sections
3. Write the updated documentation

## Instructions

1. **Examine the codebase** to understand current state:

   - Read `proto/chatto/` for protobuf definitions (event types, messages)
   - Read `cli/internal/core/` for Core implementation (KV keys, stream subjects)
   - Read `cli/internal/core_api/` for NATS services
   - Read `cli/internal/graph/schema/` for GraphQL schema (queries, mutations, subscriptions)
   - Read `cli/internal/core/permissions.go` for permission definitions
   - Read `cli/internal/core/can.go` for permission check functions
   - Search for `CreateOrUpdateKeyValue`, `CreateOrUpdateStream`, `CreateOrUpdateObjectStore` calls
   - Search for `Publish` calls to find all NATS subjects used

2. **For GraphQL API Overview**:

   - Read all `.graphqls` files in `cli/internal/graph/schema/`
   - Identify the most important queries, mutations, and subscriptions
   - Focus on user-facing operations, not internal details
   - Group by domain (spaces, rooms, messages, users, admin)

3. **For RBAC Overview**:

   - Read `cli/internal/core/permissions.go` for all permission constants
   - Read `cli/internal/core/can.go` for all `Can*` functions
   - Read `cli/internal/graph/authz.go` for GraphQL authorization helpers
   - Check `owners.emails` configuration in server setup
   - Document both instance-level and space-level authorization

4. **For NATS JetStream Inventory**:

   - Find all `CreateOrUpdateStream` calls to list streams
   - Find all `CreateOrUpdateKeyValue` calls to list KV buckets
   - Find all `CreateOrUpdateObjectStore` calls to list object stores
   - For each stream: grep for `Publish` calls to find all subjects
   - For each KV bucket: grep for `Get`, `Put`, `Create`, `Update` calls to find all key patterns
   - Document the naming conventions and variable placeholders

5. **Compare with existing documentation**:

   - Read current `docs/ARCHITECTURE.md`
   - Identify discrepancies between code and documentation

6. **Update documentation**:

   - Add new entries to appropriate tables
   - Update existing entries if they've changed
   - Remove entries for deleted resources
   - Preserve markdown table formatting (aligned columns)
   - Keep notes/explanations accurate and concise
   - Add relative links to source files (see below)

7. **Validation checklist**:
   - All streams in code appear in Streams table
   - All KV buckets in code appear in KV Buckets table
   - All object stores in code appear in Object Store Buckets table
   - All NATS API services appear in Services table
   - All permissions appear in Available Permissions table
   - All `Can*` functions are documented
   - Key GraphQL operations are listed
   - Subject/key patterns match actual code usage

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
## NATS API Layer

Key files: [`cli/internal/core_api/`](cli/internal/core_api/), [`cli/internal/core_api_client/`](cli/internal/core_api_client/)

The NATS API layer provides...
```

**Example sections and their key files:**

| Section | Key Files |
| ------- | --------- |
| Core / Architecture | `cli/internal/core/core.go` |
| NATS API Layer | `cli/internal/core_api/`, `cli/internal/core_api_client/` |
| GraphQL API | `cli/internal/graph/schema/`, `cli/internal/graph/resolver.go` |
| Roles and Permissions | `cli/internal/core/permissions.go`, `cli/internal/core/can.go`, `cli/internal/rbac/` |
| KV Buckets / Streams | `cli/internal/core/core.go` (initialization), specific domain files |
| Messages | `cli/internal/core/rooms.go` |
| Encryption | `cli/internal/encryption/` |
