# FDR-025: User Search & Member Directory

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

Any authenticated user can browse the server's member directory — a paginated list of all users on the server, with optional substring search. The directory powers the admin Users page, the @mention autocomplete (in part), and any feature that needs "find a user by name".

## Behavior

- The directory query accepts an optional search string, an offset, and a limit. Returns the matching members and a total count for paginating.
- Search matches a substring of either `login` or `displayName`, case-insensitive. Empty search returns all members.
- Pagination is offset-based: caller specifies `offset` and `limit`; the response also includes `totalCount` so the caller can compute whether there are more pages.
- Default page size is 20; the maximum is 100. Requests larger than 100 are silently clamped down.
- Results are sorted by `createdAt` ascending (oldest member first). Users created before the timestamp field existed sort to the end, alphabetically by login.

## Design Decisions

### 1. Substring search on login and displayName

**Decision:** The search matches case-insensitive substrings against both `login` and `displayName`. "ali" finds users with `login: alice`, `login: regalia`, and `displayName: "Ali Smith"` alike.
**Why:** Both `login` and `displayName` are meaningful identifiers depending on context — some users go by their handle, others by their real name. Substring (not prefix-only) accommodates "I remember the middle part" cases. Case-insensitivity is what users expect.
**Tradeoff:** Substring is more permissive than prefix and produces more false-positive matches. For a chat-app member directory, that's fine — there are no autocompletes here that rank results aggressively. The mention autocomplete has its own ranking on top (see FDR-006).

### 2. Offset-based pagination, not cursor

**Decision:** Pagination uses `offset` + `limit`, not a cursor.
**Why:** Cursor pagination is what you want when results can shift between calls (an infinite scroll over a live stream). The member directory is mostly stable — new signups happen sometimes, but the page-flipping use case is rare. Offset-based is simpler to consume from the frontend and lets the UI show "Page 3 of 12".
**Tradeoff:** If the directory changes mid-scroll, the user might see duplicates or skipped entries across pages. Acceptable given the volume and update rate.

### 3. Hard limit of 100, silent clamp

**Decision:** Requests with `limit > 100` are clamped to 100 without an error.
**Why:** An error would break clients that send larger numbers naively. Clamping serves the request with a sensible cap. The frontend doesn't currently issue limits above 100, so the clamp only affects malformed requests.
**Tradeoff:** A client expecting all 500 users in one response gets 100 and may not realise. The `totalCount` field in the response surfaces the discrepancy.

### 4. Sort by createdAt, with a stable fallback

**Decision:** Primary sort by `createdAt` ascending; users with null `createdAt` (predates the field) sort to the end alphabetically by login.
**Why:** "Oldest first" is a stable order that matches the admin mental model ("show me long-term members first; new signups at the end"). The alphabetical fallback for null timestamps keeps the order deterministic for legacy users without inventing a fake timestamp.
**Tradeoff:** Sorting by recency (newest first) is occasionally what an admin wants when investigating a signup wave. Not exposed today; could be added as a sort parameter if needed.

### 5. All authenticated users can browse the directory

**Decision:** No special permission required; any authenticated user can list members.
**Why:** Chatto's privacy model treats user identity (login, display name, avatar) as public to other members. Hiding members from members would be incongruent — they'd see each other in messages anyway. Operators who want a fully private member list would need a different feature.
**Tradeoff:** Bot accounts or system users (if introduced) would surface in normal listings. Not a current concern.

### 6. Implicit membership, no explicit member records

**Decision:** After the #330 consolidation, every authenticated user is implicitly a member of the server. There's no `ServerMembership` record; the user list *is* the member list.
**Why:** Explicit memberships would require a join-leave workflow that didn't exist (Chatto's earlier design assumed everyone-is-a-member). Removing them reduced storage and code paths without losing functionality. See ADR-027.
**Tradeoff:** No way to mark someone as "a user on this server but not currently a member". For operators who need that, the suspension flow (FDR-001's user-level deny pattern) handles it.

## Permissions

No explicit permission — authentication only.

## Related

- **ADRs:** ADR-027 (instance/space consolidation)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-006 (@Mentions), FDR-021 (Admin Dashboard & System Monitoring), FDR-022 (User Profile)
