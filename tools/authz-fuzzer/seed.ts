// Bootstraps the fixture world used by the fuzzer.
//
// World shape:
//   - publicSpace + publicRoom: created by spaceAdmin, joined by spaceMember
//     (space-only) and roomMember (room). spaceMember is intentionally NOT in
//     the room so we can test the room-membership boundary.
//   - otherSpace: created by otherSpaceOwner; nobody else is in it. Used for
//     cross-tenant probes (a spaceAdmin of publicSpace must NOT be able to
//     touch otherSpace).
//   - one seeded message in publicRoom posted by roomMember, used to test
//     edit/delete authorization (other users should not be able to alter it).

import { Client } from "./client.ts";
import { PERSONAS, type PersonaId } from "./personas.ts";

export type SeedWorld = {
  publicSpaceId: string;
  publicRoomId: string;
  otherSpaceId: string;
  seededMessageEventId: string | null;
  users: {
    randomUserId: string;
    spaceMemberUserId: string;
    roomMemberUserId: string;
    spaceAdminUserId: string;
    otherSpaceOwnerUserId: string;
    instanceAdminUserId: string;
  };
};

export type ClientPool = Record<PersonaId, Client>;

async function ensureUser(c: Client, login: string, email: string, password: string): Promise<string> {
  // Use the build-tagged /auth/test/create-user endpoint. The production
  // GraphQL `createUser` mutation was removed for security (#175) — it was
  // unauthenticated and let any caller win the race to instance owner.
  // The test endpoint requires `-tags test_endpoints` on the server build,
  // which `mise dev` and the e2e CI both use.
  const create = await c.http("/auth/test/create-user", {
    method: "POST",
    body: JSON.stringify({ login, displayName: login, password }),
  });
  if (create.ok) {
    const data = (await create.json()) as { id: string };
    if (email) await maybeVerifyEmail(c, data.id, email);
    return data.id;
  }

  // 409/500 on duplicate login — fall through to looking the user up so the
  // fuzzer is rerunnable against a polluted dev instance.
  const me = await c.query<{ userByLogin: { id: string } | null }>(
    "query($l: String!) { userByLogin(login: $l) { id } }",
    { l: login },
  );
  if (!me.data?.userByLogin) {
    throw new Error(`/auth/test/create-user failed (${create.status}) and no user with login ${login} exists`);
  }
  if (email) await maybeVerifyEmail(c, me.data.userByLogin.id, email);
  return me.data.userByLogin.id;
}

// Verify a user's email via the test endpoint so the instance-admin persona
// matches `owners.emails` in `chatto.toml`. Errors are tolerated because the
// endpoint is idempotent and most personas don't need a verified email.
async function maybeVerifyEmail(c: Client, userId: string, email: string): Promise<void> {
  await c.http("/auth/test/verify-email", {
    method: "POST",
    body: JSON.stringify({ userId, email }),
  });
}

export async function buildClients(endpoint: string): Promise<ClientPool> {
  const pool = {} as ClientPool;
  for (const p of PERSONAS) {
    pool[p.id] = new Client(endpoint, p.id);
  }
  return pool;
}

export async function seed(endpoint: string, pool: ClientPool): Promise<SeedWorld> {
  // Throwaway client for the user-provisioning calls. /auth/test/create-user
  // and /auth/test/verify-email don't create sessions, so cookies don't
  // contaminate later persona-specific Clients.
  const bootstrap = new Client(endpoint, "bootstrap");

  const ids: SeedWorld["users"] = {
    randomUserId: "",
    spaceMemberUserId: "",
    roomMemberUserId: "",
    spaceAdminUserId: "",
    otherSpaceOwnerUserId: "",
    instanceAdminUserId: "",
  };

  for (const p of PERSONAS) {
    if (p.id === "anon") continue;
    const userId = await ensureUser(bootstrap, p.login, p.email, p.password);
    (ids as Record<string, string>)[`${p.id}UserId`] = userId;
    await pool[p.id].login(p.login, p.password);
  }

  // The seed now verifies each persona's email via /auth/test/verify-email,
  // which is enough to make `instanceAdmin` work on any instance whose
  // `owners.emails` contains `instanceAdmin.email` from personas.ts. The
  // default value (`fuzz_iadmin@example.test`) is unlikely to match an
  // operator's config, so add it to chatto.toml before relying on the
  // admin-only cells:
  //
  //   [admin]
  //   emails = ["fuzz_iadmin@example.test"]
  //
  // Until that's done on the target instance, treat `instanceAdmin → allow`
  // cells as low-confidence (they'll fail-closed because the persona's
  // verified email doesn't match any admin entry).

  // spaceAdmin creates publicSpace.
  const sp = await pool.spaceAdmin.query<{ createSpace: { id: string } }>(
    "mutation($i: CreateSpaceInput!) { createSpace(input: $i) { id } }",
    { i: { name: "fuzz-public-space", description: "fuzzer fixture" } },
  );
  if (!sp.data?.createSpace) throw new Error(`createSpace failed: ${JSON.stringify(sp.errors)}`);
  const publicSpaceId = sp.data.createSpace.id;

  // spaceAdmin creates publicRoom.
  const rm = await pool.spaceAdmin.query<{ createRoom: { id: string } }>(
    "mutation($i: CreateRoomInput!) { createRoom(input: $i) { id } }",
    { i: { spaceId: publicSpaceId, name: "fuzz-public-room" } },
  );
  if (!rm.data?.createRoom) throw new Error(`createRoom failed: ${JSON.stringify(rm.errors)}`);
  const publicRoomId = rm.data.createRoom.id;

  // spaceMember + roomMember join the space; only roomMember joins the room.
  for (const id of ["spaceMember", "roomMember"] as const) {
    await pool[id].query("mutation($i: JoinSpaceInput!) { joinSpace(input: $i) }", {
      i: { spaceId: publicSpaceId },
    });
  }
  await pool.roomMember.query("mutation($i: JoinRoomInput!) { joinRoom(input: $i) }", {
    i: { spaceId: publicSpaceId, roomId: publicRoomId },
  });

  // roomMember posts a seed message used for edit/delete tests.
  const post = await pool.roomMember.query<{
    postMessage: { __typename: string; id?: string; eventId?: string };
  }>(
    "mutation($i: PostMessageInput!) { postMessage(input: $i) { __typename ... on Message { eventId } } }",
    { i: { spaceId: publicSpaceId, roomId: publicRoomId, body: "fuzz-seed-msg" } },
  );
  const seededMessageEventId = post.data?.postMessage?.eventId ?? null;

  // otherSpaceOwner creates an isolated space.
  const sp2 = await pool.otherSpaceOwner.query<{ createSpace: { id: string } }>(
    "mutation($i: CreateSpaceInput!) { createSpace(input: $i) { id } }",
    { i: { name: "fuzz-other-space" } },
  );
  if (!sp2.data?.createSpace) throw new Error(`createSpace(other) failed: ${JSON.stringify(sp2.errors)}`);
  const otherSpaceId = sp2.data.createSpace.id;

  return {
    publicSpaceId,
    publicRoomId,
    otherSpaceId,
    seededMessageEventId,
    users: ids,
  };
}
