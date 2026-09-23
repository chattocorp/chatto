# FDR-025: User Search & Member Directory

**Status:** Active
**Last reviewed:** 2026-09-23

## Overview

Any authenticated user can browse the server's member directory — a paginated list of all active human and bot accounts on the server, with optional substring search. The directory powers general member-picking surfaces such as user comboboxes, quick switching, and @mention autocomplete. Admin member-management screens use the separate `AdminUserService` because they expose administrative fields and permissions.

## Behavior

- The directory query accepts an optional search string, an offset, and a limit. Returns the matching members and a total count for paginating.
- The canonical public surface is ConnectRPC `UserService.ListUsers(search, page)`.
- Search matches a substring of either `login` or `displayName`, case-insensitive. Empty search returns all members.
- Pagination is offset-based: caller specifies `offset` and `limit`; the response also includes `totalCount` so the caller can compute whether there are more pages.
- Default page size is 20; the maximum is 500. Requests larger than 500 are silently clamped down.
- Results are sorted by `createdAt` ascending (oldest member first). Users created before the timestamp field existed sort to the end, alphabetically by login.
- Direct user lookups by stable user ID or login return the same public directory row shape as the directory and require authentication. Batch user hydration by stable user ID supports cache-miss loading without N+1 reads.
- Directory and lookup rows expose the canonical `User.bot` metadata. Clients render an accessible bot indicator so people can distinguish automation from human accounts.
- Room membership lists return user IDs in ascending ID order. Clients resolve
  missing profiles through bounded batch reads. Ordinary room listing does not
  decrypt user names; name search still reads the profiles it searches.
- The bundled client retains loaded room members for the authenticated server
  session. Moving between rooms reuses these lists. Realtime joins, leaves, and
  profile changes update retained rooms, including rooms that are not open.
- A room join also loads the joining user's profile when the client has not
  opened that room. The profile becomes available to other client views.
- Mention completion can search for names before the room list finishes
  loading. It does not wait for the background member scan.
- Connected members load independently of the full list. This includes available,
  away, and do-not-disturb users. The full list continues loading names for
  mention completion. A failed preview does not stop that full load.
- Recovery resets and access loss clear retained membership. Changes to
  universal-room eligibility require an authoritative membership read.

## Design Decisions

### 1. Substring search on login and displayName

**Decision:** The search matches case-insensitive substrings against both `login` and `displayName`. "ali" finds users with `login: alice`, `login: regalia`, and `displayName: "Ali Smith"` alike.
**Why:** Both `login` and `displayName` are meaningful identifiers depending on context — some users go by their handle, others by their real name. Substring (not prefix-only) accommodates "I remember the middle part" cases. Case-insensitivity is what users expect.
**Tradeoff:** Substring is more permissive than prefix and produces more false-positive matches. For a chat-app member directory, that's fine — there are no autocompletes here that rank results aggressively. The mention autocomplete has its own ranking on top (see FDR-006).

### 2. Offset-based pagination, not cursor

**Decision:** Pagination uses `offset` + `limit`, not a cursor.
**Why:** Cursor pagination is what you want when results can shift between calls (an infinite scroll over a live stream). The member directory is mostly stable — new signups happen sometimes, but the page-flipping use case is rare. Offset-based is simpler to consume from the frontend and lets the UI show "Page 3 of 12".
**Tradeoff:** If the directory changes mid-scroll, the user might see duplicates or skipped entries across pages. Acceptable given the volume and update rate.

### 3. Hard limit of 500, silent clamp

**Decision:** Requests with `limit > 500` are clamped to 500 without an error.
**Why:** An error would break clients that send larger numbers naively. Clamping serves the request with a sensible cap. Normal clients request smaller pages, so the clamp mainly affects malformed requests.
**Tradeoff:** A client expecting all users in one response may get a truncated page and may not realise. The `totalCount` field in the response surfaces the discrepancy.

### 4. Sort server users by createdAt, with a stable fallback

**Decision:** The server user directory sorts by `createdAt` ascending. Users with null `createdAt` sort to the end alphabetically by login. Room membership pages sort by stable user ID so the server can list IDs without reading profiles.
**Why:** "Oldest first" is a stable order that matches the admin mental model ("show me long-term members first; new signups at the end"). The alphabetical fallback for null timestamps keeps the order deterministic for legacy users without inventing a fake timestamp.
**Tradeoff:** Sorting by recency (newest first) is occasionally what an admin wants when investigating a signup wave. Not exposed today; could be added as a sort parameter if needed.

### 5. All authenticated users can browse member profiles

**Decision:** No special permission required; any authenticated user can list members or look up a member by stable user ID.
**Why:** Chatto's privacy model treats user identity (login, display name, avatar) as public to other members. Hiding members from members would be incongruent — they'd see each other in messages anyway. Operators who want a fully private member list would need a different feature.
**Tradeoff:** Bot accounts intentionally surface in normal listings. Integrations that need only human accounts must filter the presence of canonical `User.bot` metadata. The admin UI may still require admin permissions to reach its member-management page, but the underlying directory query remains available to authenticated users.

### 6. Implicit membership, no explicit member records

**Decision:** After the #330 consolidation, every authenticated user is implicitly a member of the server. There's no `ServerMembership` record; the user list *is* the member list.
**Why:** Explicit memberships would require a join-leave workflow that didn't exist (Chatto's earlier design assumed everyone-is-a-member). Removing them reduced storage and code paths without losing functionality. See ADR-027.
**Tradeoff:** No way to mark someone as "a user on this server but not currently a member". For operators who need that, the suspension flow (FDR-001's user-level deny pattern) handles it.

### 7. Separate admin list selection from private rows

**Decision:** Admin member lists return IDs first. The client loads missing
admin rows in batches and reuses them across searches and pages within the
same session. Private rows and role labels share a cache lifetime. Logout,
permission loss, and account deletion clear private data and fence older reads.

**Why:** Listing accounts must not wait for every row's email, avatar, roles,
account permissions, and presence. Keeping this cache separate from ordinary
profiles prevents private admin data from entering member-facing caches.

**Tradeoff:** A cold page needs a second request. Both reads check access, and
pagination counts IDs even when an account disappears before its row is loaded.

### 8. Separate room membership from user profiles

**Decision:** Room membership reads return IDs. The client shares cached user
profiles across rooms and resolves missing profiles in batches. The room store
continues background loading so local mention matching has names available.
Reads caused by a room join wait for that join before they load profiles.
**Why:** A room switch must not repeat profile reads for users the client already
knows. Each membership page must not decrypt every profile in the room.
**Tradeoff:** A cold load requires a second request for names. Name search remains
available while that load is pending. A membership event during offset pagination
restarts the scan at the event boundary to prevent skipped entries.

### 9. Load connected members independently

**Decision:** Read connected presence groups alongside the full directory. Share
cached profiles between these reads. Realtime presence takes precedence over a
preview that was already in flight. The full directory determines final membership.
**Why:** A small online group must not wait behind many offline profiles. Separate
status reads also supply current presence when a cached name is reused.
**Tradeoff:** Opening an uncached room starts three additional small list reads.
Presence can change between pages, so the preview is not a fixed snapshot.

## Permissions

Server user-directory reads require authentication only. Room membership lists
require room membership or the applicable room-management or discovery access.
DM member lists require participation in that DM.

## Related

- **ADRs:** ADR-027 (instance/space consolidation), ADR-101 (shared client user profiles)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-006 (@Mentions), FDR-021 (Admin Dashboard & System Monitoring), FDR-022 (User Profile), FDR-038 (Bot Accounts)
