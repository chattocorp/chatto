// Operations under test. Each entry knows how to render its variables given
// the seed world. Add new operations here; pair every addition with a row in
// matrix.ts.
//
// Coverage strategy: start with operations that have non-obvious authz
// semantics (cross-tenant access, admin-only, ownership-gated). Trivial CRUD
// is lower-priority — we want the cells likeliest to surface real bugs.

import type { SeedWorld } from "./seed.ts";

export type Op = {
  name: string;
  category: "query" | "mutation";
  query: string;
  // Build variables from the seed world. Return null to skip the op when the
  // seed didn't produce the prerequisite (e.g., no DM thread exists).
  vars: (s: SeedWorld) => Record<string, unknown> | null;
};

export const OPERATIONS: Op[] = [
  // ---------- Queries: identity & discovery ----------
  {
    name: "Query.viewer.user",
    category: "query",
    query: "query { viewer { user { id login } } }",
    vars: () => ({}),
  },
  {
    name: "Query.users",
    category: "query",
    query: "query { users { id login } }",
    vars: () => ({}),
  },
  {
    name: "Query.user(otherUserId)",
    category: "query",
    query: "query($id: ID!) { user(id: $id) { id login } }",
    vars: (s) => ({ id: s.users.spaceAdminUserId }),
  },
  {
    name: "Query.userByLogin(otherLogin)",
    category: "query",
    query: "query($login: String!) { userByLogin(login: $login) { id } }",
    vars: (s) => ({ login: "fuzz_sadmin" }),
  },
  {
    name: "Query.spaces",
    category: "query",
    query: "query { spaces { id name } }",
    vars: () => ({}),
  },
  {
    name: "Query.space(publicSpaceId)",
    category: "query",
    query: "query($id: ID!) { space(id: $id) { id name memberCount } }",
    vars: (s) => ({ id: s.publicSpaceId }),
  },

  // ---------- Queries: room/message access (membership-gated) ----------
  {
    name: "Query.room(publicRoom)",
    category: "query",
    query: "query($s: ID!, $r: ID!) { room(spaceId: $s, roomId: $r) { id name } }",
    vars: (s) => ({ s: s.publicSpaceId, r: s.publicRoomId }),
  },
  {
    name: "Query.roomEvents(publicRoom)",
    category: "query",
    query:
      "query($s: ID!, $r: ID!) { roomEvents(spaceId: $s, roomId: $r, limit: 5) { events { __typename } } }",
    vars: (s) => ({ s: s.publicSpaceId, r: s.publicRoomId }),
  },
  {
    name: "Query.roomEventByEventId(publicRoom, seededMsg)",
    category: "query",
    query:
      "query($s: ID!, $r: ID!, $e: ID!) { roomEventByEventId(spaceId: $s, roomId: $r, eventId: $e) { __typename } }",
    vars: (s) =>
      s.seededMessageEventId
        ? { s: s.publicSpaceId, r: s.publicRoomId, e: s.seededMessageEventId }
        : null,
  },

  // ---------- Queries: admin namespace (privacy boundary) ----------
  {
    name: "Query.admin.users",
    category: "query",
    query: "query { admin { users { id login email } } }",
    vars: () => ({}),
  },
  {
    name: "Query.admin.systemInfo",
    category: "query",
    query: "query { admin { systemInfo { version uptime } } }",
    vars: () => ({}),
  },

  // ---------- Mutations: cross-tenant probes ----------
  {
    name: "Mutation.updateSpace(otherSpace)",
    category: "mutation",
    query:
      "mutation($i: UpdateSpaceInput!) { updateSpace(input: $i) { id } }",
    vars: (s) => ({
      i: { id: s.otherSpaceId, name: "fuzzed-rename", description: "x" },
    }),
  },
  {
    name: "Mutation.createRoom(otherSpace)",
    category: "mutation",
    query:
      "mutation($i: CreateRoomInput!) { createRoom(input: $i) { id } }",
    vars: (s) => ({
      i: { spaceId: s.otherSpaceId, name: "should-not-exist" },
    }),
  },
  {
    name: "Mutation.postMessage(publicRoom)",
    category: "mutation",
    query:
      "mutation($i: PostMessageInput!) { postMessage(input: $i) { __typename } }",
    vars: (s) => ({
      i: { spaceId: s.publicSpaceId, roomId: s.publicRoomId, body: "fuzz-msg" },
    }),
  },
  {
    name: "Mutation.deleteMessage(otherUsersMsg)",
    category: "mutation",
    query:
      "mutation($i: DeleteMessageInput!) { deleteMessage(input: $i) }",
    vars: (s) =>
      s.seededMessageEventId
        ? {
            i: {
              spaceId: s.publicSpaceId,
              roomId: s.publicRoomId,
              eventId: s.seededMessageEventId,
            },
          }
        : null,
  },
  {
    name: "Mutation.updateMessage(otherUsersMsg)",
    category: "mutation",
    query:
      "mutation($i: UpdateMessageInput!) { updateMessage(input: $i) }",
    vars: (s) =>
      s.seededMessageEventId
        ? {
            i: {
              spaceId: s.publicSpaceId,
              roomId: s.publicRoomId,
              eventId: s.seededMessageEventId,
              body: "edited-by-fuzzer",
            },
          }
        : null,
  },
  {
    name: "Mutation.archiveRoom(publicRoom)",
    category: "mutation",
    query:
      "mutation($i: ArchiveRoomInput!) { archiveRoom(input: $i) { id } }",
    vars: (s) => ({
      i: { spaceId: s.publicSpaceId, roomId: s.publicRoomId },
    }),
  },
  {
    name: "Mutation.uploadSpaceLogo(otherSpace)",
    category: "mutation",
    query:
      "mutation($i: UploadSpaceLogoInput!) { uploadSpaceLogo(input: $i) { id } }",
    vars: (s) => ({
      // Upload scalar isn't easily POSTable from this minimal client; the
      // operation tests authz on the resolver path. Expect deny long before
      // the upload is processed.
      i: { spaceId: s.otherSpaceId, file: null },
    }),
  },
  {
    name: "Mutation.markRoomAsRead(publicRoom)",
    category: "mutation",
    query:
      "mutation($i: MarkRoomAsReadInput!) { markRoomAsRead(input: $i) { lastReadAt } }",
    vars: (s) => ({
      i: { spaceId: s.publicSpaceId, roomId: s.publicRoomId },
    }),
  },
  {
    // Target is the spaceAdmin user. Only spaceAdmin (self) and instanceAdmin
    // (rank-based override) should succeed; everyone else gets denied.
    name: "Mutation.updateProfile",
    category: "mutation",
    query:
      "mutation($i: UpdateProfileInput!) { updateProfile(input: $i) { id displayName } }",
    vars: (s) => ({
      i: { userId: s.users.spaceAdminUserId, displayName: "fuzz-display" },
    }),
  },
  {
    name: "Mutation.deleteAvatar",
    category: "mutation",
    query: "mutation($id: ID!) { deleteAvatar(userId: $id) { id } }",
    vars: (s) => ({ id: s.users.spaceAdminUserId }),
  },
  {
    name: "Mutation.updateSettings",
    category: "mutation",
    query:
      "mutation($i: UpdateSettingsInput!) { updateSettings(input: $i) { timezone } }",
    vars: (s) => ({
      i: { userId: s.users.spaceAdminUserId, timezone: "Europe/Berlin" },
    }),
  },
];
