// Expected outcome for each (operation, persona) pair.
//
// Outcomes:
//   "allow"    — call returns success, data is non-null where applicable
//   "deny"     — call returns a permission/authorization error (or null result)
//   "auth"     — call returns "not authenticated" (no valid session)
//   "notfound" — call returns null without leaking existence (IDOR-safe)
//
// "allow" must be the EXPECTED behaviour, not the OBSERVED one. Every cell is
// a security claim. If a cell is wrong-on-purpose to capture current behaviour
// while a fix is in flight, mark it `// TODO(SEC-NN)` so it's grep-able.

import type { PersonaId } from "./personas.ts";

export type Outcome = "allow" | "deny" | "auth" | "notfound";
export type Matrix = Record<string, Partial<Record<PersonaId, Outcome>>>;

// Default for any persona not listed for an op = "deny". Be explicit when you
// mean "allow" — silence is denial.
export const MATRIX: Matrix = {
  // me() returns null for anon; not an error
  "Query.me": {
    anon: "allow", // null is the expected result, not an error
    randomUser: "allow",
    spaceMember: "allow",
    roomMember: "allow",
    spaceAdmin: "allow",
    otherSpaceOwner: "allow",
    instanceAdmin: "allow",
  },
  "Query.users": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "deny",
    spaceAdmin: "deny",
    otherSpaceOwner: "deny",
    instanceAdmin: "allow",
  },
  "Query.user(otherUserId)": {
    // Profiles are public to authenticated callers
    anon: "auth",
    randomUser: "allow",
    spaceMember: "allow",
    roomMember: "allow",
    spaceAdmin: "allow",
    otherSpaceOwner: "allow",
    instanceAdmin: "allow",
  },
  "Query.userByLogin(otherLogin)": {
    anon: "auth",
    randomUser: "allow",
    spaceMember: "allow",
    roomMember: "allow",
    spaceAdmin: "allow",
    otherSpaceOwner: "allow",
    instanceAdmin: "allow",
  },
  "Query.spaces": {
    // Discovery — public per docs
    anon: "allow",
    randomUser: "allow",
    spaceMember: "allow",
    roomMember: "allow",
    spaceAdmin: "allow",
    otherSpaceOwner: "allow",
    instanceAdmin: "allow",
  },
  "Query.space(publicSpaceId)": {
    anon: "allow",
    randomUser: "allow",
    spaceMember: "allow",
    roomMember: "allow",
    spaceAdmin: "allow",
    otherSpaceOwner: "allow",
    instanceAdmin: "allow",
  },
  "Query.room(publicRoom)": {
    // Room access requires room membership
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "allow",
    spaceAdmin: "allow", // admin can access rooms in their space
    otherSpaceOwner: "deny",
    instanceAdmin: "deny", // privacy boundary: instance admin must NOT read content
  },
  "Query.roomEvents(publicRoom)": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "allow",
    spaceAdmin: "allow",
    otherSpaceOwner: "deny",
    instanceAdmin: "deny",
  },
  "Query.roomEventByEventId(publicRoom, seededMsg)": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "allow",
    spaceAdmin: "allow",
    otherSpaceOwner: "deny",
    instanceAdmin: "deny",
  },

  "Query.admin.users": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "deny",
    spaceAdmin: "deny",
    otherSpaceOwner: "deny",
    instanceAdmin: "allow",
  },
  "Query.admin.systemInfo": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "deny",
    spaceAdmin: "deny",
    otherSpaceOwner: "deny",
    instanceAdmin: "allow",
  },

  "Mutation.updateSpace(otherSpace)": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "deny",
    spaceAdmin: "deny", // admin of a DIFFERENT space
    otherSpaceOwner: "allow",
    instanceAdmin: "deny", // unless instance admin is also a space admin — keep strict
  },
  "Mutation.createRoom(otherSpace)": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "deny",
    spaceAdmin: "deny",
    otherSpaceOwner: "allow",
    instanceAdmin: "deny",
  },
  "Mutation.postMessage(publicRoom)": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny", // not in room
    roomMember: "allow",
    spaceAdmin: "allow",
    otherSpaceOwner: "deny",
    instanceAdmin: "deny",
  },
  "Mutation.deleteMessage(otherUsersMsg)": {
    // Message owned by a seed user; nobody else should delete it (except mods).
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "deny", // not the author
    spaceAdmin: "allow", // admin/mod usually has delete-any; verify with code
    otherSpaceOwner: "deny",
    instanceAdmin: "deny",
  },
  "Mutation.editMessage(otherUsersMsg)": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "deny",
    spaceAdmin: "deny", // edit is author-only per schema docs
    otherSpaceOwner: "deny",
    instanceAdmin: "deny",
  },
  "Mutation.archiveRoom(publicRoom)": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "deny",
    spaceAdmin: "allow",
    otherSpaceOwner: "deny",
    instanceAdmin: "deny",
  },
  "Mutation.uploadSpaceLogo(otherSpace)": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "deny",
    spaceAdmin: "deny",
    otherSpaceOwner: "allow",
    instanceAdmin: "deny",
  },
  "Mutation.markRoomAsRead(publicRoom)": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "allow",
    spaceAdmin: "allow",
    otherSpaceOwner: "deny",
    instanceAdmin: "deny",
  },
  // Target user is `spaceAdminUserId` (set in operations.ts). Self (spaceAdmin)
  // and the rank-based override (instanceAdmin) succeed; everyone else lacks
  // the rank to manage that user and is denied.
  "Mutation.updateProfile": {
    anon: "auth",
    randomUser: "deny",
    spaceMember: "deny",
    roomMember: "deny",
    spaceAdmin: "allow",
    otherSpaceOwner: "deny",
    instanceAdmin: "allow",
  },
};
