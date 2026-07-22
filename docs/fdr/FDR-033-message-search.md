# FDR-033: Message Search

**Status:** Experimental
**Last reviewed:** 2026-07-21

## Overview

Authenticated users can search message history on one Chatto server and open a
matching message in its original room or thread context. Search is an optional
server feature: operators decide whether to expose it and which trusted search
provider supplies results.

## Behavior

- Search covers the current bodies of messages in rooms the viewer may
  currently read, including direct messages, threads, and accessible archived
  rooms.
- A search applies to one server. It does not combine results from other
  servers registered in the client.
- Plain words are combined as required terms. Quoted text searches for an exact
  phrase, and an explicit `AND` is accepted between terms.
- Relevance favours literal word matches while adding lower-ranked recall from
  the operator-selected Bleve language analyzers, CJK token matching when its
  analyzer is selected, and conservative one-character spelling mistakes in
  longer words. The bundled provider enables all analyzers by default.
- Structured filters support a room (`in:`), author (`from:`), messages before
  or after a date, and messages with attachments.
- Search is a server-level page reached from the server sidebar between
  Overview and My Threads. It starts without a room filter.
- Each registered server retains its own transient query, ordering, and result
  state. Switching servers never carries a query into another server; returning
  to a server can restore its previous search.
- Results show the current message, author, room, and timestamp. The bundled
  client requests 50 at a time, loads more automatically, and can order them by
  relevance or newest first. Pagination reads the live provider index rather
  than a pinned snapshot, so results may shift while indexing continues.
- Selecting a result opens the message in its historical room or thread context
  using the normal jumped-mode navigation.
- Edits replace searchable content. Retracted, deleted, unavailable, or
  crypto-shredded bodies do not appear in results.
- Search is absent when the server feature is disabled. A configured provider
  that is still indexing or temporarily unavailable produces an explicit
  status without making the rest of the server unusable.
- Public readiness is deliberately coarse. Exact event-log indexing counts and
  rates remain operator telemetry rather than being exposed to every member.

## Design Decisions

### 1. Search is separate from navigation switching

**Decision:** Message search has its own server-level surface instead of being
merged into the quick switcher.
**Why:** Destination switching and reading historical content are different
tasks with different result density, filters, and navigation behavior.
**Tradeoff:** The client has two search-like entry points to learn.

### 2. Current visibility is authoritative

**Decision:** Results are limited to rooms the viewer may currently read, and
each result is checked again against current message state before delivery.
**Why:** A derived search index must never preserve access after membership or
content visibility changes. Search cannot become an alternative path around
the room privacy boundary.
**Tradeoff:** Authorization and hydration add work after text matching, and
stale provider hits may be discarded before a page is returned.

### 3. Only current message bodies are searchable

**Decision:** Editing a message replaces its indexed text instead of preserving
searchable edit history.
**Why:** Normal message reads expose the current body, so returning historical
text would be surprising and could reveal content the author removed. See
FDR-004.
**Tradeoff:** Search is not an edit-history or moderation-audit tool.

### 4. Search availability is negotiated independently

**Decision:** Public protocol support, operator feature enablement, provider
startup topology, and temporary provider readiness are separate states.
**Why:** A bundled or external provider may run independently from the main
app. Mixed-version clients need a stable support signal, while temporary
provider failure should degrade only Search. See ADR-041, ADR-045, and ADR-053.
**Tradeoff:** The API and client handle more states than a permanently embedded
search implementation would require.

### 5. Full-text indexing is a privileged optional cache

**Decision:** A provider may decrypt message bodies into a local derived index
that is excluded from normal backups and can be rebuilt from retained `EVT`
history.
**Why:** Useful server-side full-text search requires a plaintext-derived
representation even though durable message bodies remain encrypted. Bleve
logically removes retracted and crypto-shredded documents immediately and
reclaims their immutable segments through normal background merging. Operators
who require stronger physical-erasure guarantees must protect or explicitly
rebuild the index volume. See ADR-007, ADR-033, ADR-054, and ADR-055.
**Tradeoff:** Enabling Search expands the trusted server-side data surface and
requires operators to protect the provider volume.

### 6. One canonical query language fronts every provider

**Decision:** Chatto defines and parses the user-facing query syntax before
issuing normalized provider requests.
**Why:** Query syntax, required-term semantics, and filters should remain stable
when an operator replaces Bleve with another provider, and third-party clients
should not need to emit a backend-specific query language.
**Tradeoff:** Recall and ranking may still vary by provider because analysis
features such as stemming and typo tolerance are implementation details.

### 7. Search has a dedicated server page

**Decision:** Search lives in the server sidebar and opens as a full page rather
than a modal or part of the quick switcher.
**Why:** Searching message history is an extended reading task whose query,
results, filters, and future conversation context need durable screen space.
**Tradeoff:** Opening a result leaves the Search page; each server's transient
search is retained in memory so browser Back can restore it.

## Related

- **ADRs:** ADR-007 (per-user encryption with crypto-shredding), ADR-033
  (event-sourced state with projections), ADR-041 (runtime units), ADR-045
  (public API stability tiers), ADR-053 (versioned NATS service namespaces),
  ADR-054 (optional projection persistence), ADR-055 (pluggable message search
  over NATS)
- **FDRs:** FDR-004 (Message Editing & Deletion), FDR-014 (Jump to Present),
  FDR-015 (Quick Switcher), FDR-019 (Room Lifecycle), FDR-032 (Message
  Formatting)
