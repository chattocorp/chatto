import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  logoutCurrentUser,
  loginAsAdminAndUsePrimaryServer,
  type TestUser
} from './fixtures/testUser';
import * as routes from './routes';

interface TestServer {
  id: string;
  name: string;
}

// ============================================================================
// GraphQL Helper Functions
// ============================================================================

async function usePrimaryServerViaAPI(page: Page, _name?: string): Promise<TestServer> {
  return loginAsAdminAndUsePrimaryServer(page);
}

async function createSecondTestUser(page: Page): Promise<TestUser> {
  const timestamp = Date.now();
  const testUser: TestUser = {
    login: `rpuser${timestamp}`,
    displayName: `RP User ${timestamp}`,
    password: 'testpassword123'
  };
  const createResp = await page.request.post('/auth/test/create-user', {
    headers: { 'Content-Type': 'application/json' },
    data: {
      login: testUser.login,
      displayName: testUser.displayName,
      password: testUser.password
    }
  });
  expect(createResp.ok()).toBeTruthy();
  const createData = await createResp.json();
  testUser.id = createData.id;

  // Verify email
  const verifyResp = await page.request.post('/auth/test/verify-email', {
    headers: { 'Content-Type': 'application/json' },
    data: { userId: testUser.id, email: `${testUser.login}@example.com` }
  });
  expect(verifyResp.ok()).toBeTruthy();
  return testUser;
}

async function loginUser(page: Page, login: string, password: string): Promise<void> {
  const resp = await page.request.post('/auth/login', { data: { login, password } });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).success).toBe(true);
}

async function logoutUser(page: Page): Promise<void> {
  await logoutCurrentUser(page);
}

async function createRoomViaAPI(page: Page, name?: string): Promise<string> {
  const roomName = name ?? `room${Date.now()}`;
  const groupResp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: { query: `query { server { roomGroups { id } } }` }
  });
  expect(groupResp.ok()).toBeTruthy();
  const groupData = await groupResp.json();
  const groupId = groupData.data?.server?.roomGroups?.[0]?.id;
  if (!groupId) {
    throw new Error(`No room group available for e2e room creation: ${JSON.stringify(groupData)}`);
  }

  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: CreateRoomInput!) { createRoom(input: $input) { id name } }`,
      variables: { input: { name: roomName, groupId } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  const data = await resp.json();
  if (data.errors || !data.data?.createRoom) {
    throw new Error(`createRoom failed: ${JSON.stringify(data)}`);
  }
  return data.data.createRoom.id;
}

async function getRoomByName(page: Page, roomName: string): Promise<string> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `query { server { rooms(type: CHANNEL) { id name } } }`
    }
  });
  expect(resp.ok()).toBeTruthy();
  const data = await resp.json();
  const rooms = data.data?.server?.rooms;
  if (!rooms) {
    throw new Error(`Failed to get rooms: ${JSON.stringify(data)}`);
  }
  const room = rooms.find((r: { name: string }) => r.name.toLowerCase() === roomName.toLowerCase());
  if (!room) {
    throw new Error(`Room '${roomName}' not found in instance`);
  }
  return room.id;
}

async function joinRoomViaAPI(page: Page, roomId: string): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: JoinRoomInput!) { joinRoom(input: $input) { id } }`,
      variables: { input: { roomId } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.joinRoom?.id).toBe(roomId);
}

async function denyPermission(page: Page, role: string, permission: string): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: DenyPermissionInput!) { denyPermission(input: $input) }`,
      variables: { input: { roleName: role, permission } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.denyPermission).toBe(true);
}

async function revokePermission(page: Page, role: string, permission: string): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: RevokePermissionInput!) { revokePermission(input: $input) }`,
      variables: { input: { roleName: role, permission } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.revokePermission).toBe(true);
}

async function grantRoomPermission(
  page: Page,
  roomId: string,
  role: string,
  permission: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: GrantRoomPermissionInput!) {
				grantRoomPermission(input: $input)
			}`,
      variables: { input: { roomId, roleName: role, permission } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.grantRoomPermission).toBe(true);
}

async function denyRoomPermission(
  page: Page,
  roomId: string,
  role: string,
  permission: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: DenyRoomPermissionInput!) {
				denyRoomPermission(input: $input)
			}`,
      variables: { input: { roomId, roleName: role, permission } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.denyRoomPermission).toBe(true);
}

async function postMessageViaAPI(
  page: Page,
  roomId: string,
  body: string
): Promise<{ id: string } | null> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) {
				postMessage(input: $input) { id }
			}`,
      variables: { input: { roomId, body } }
    }
  });
  const data = await resp.json();
  if (data.errors) {
    return null;
  }
  return data.data?.postMessage ?? null;
}

async function replyToMessageViaAPI(
  page: Page,
  roomId: string,
  inThread: string,
  body: string
): Promise<{ id: string } | null> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) {
				postMessage(input: $input) { id }
			}`,
      variables: { input: { roomId, body, threadRootEventId: inThread } }
    }
  });
  const data = await resp.json();
  if (data.errors) {
    return null;
  }
  return data.data?.postMessage ?? null;
}

async function addReactionViaAPI(
  page: Page,
  roomId: string,
  messageEventId: string,
  emoji: string
): Promise<boolean> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: AddReactionInput!) {
				addReaction(input: $input)
			}`,
      variables: { input: { roomId, messageEventId, emoji } }
    }
  });
  const data = await resp.json();
  return !data.errors;
}

// ============================================================================
// Test Scenarios
// ============================================================================

test.describe('Room-Level Permission Overrides', () => {
  test.describe('message.post — Chat Input', () => {
    test('room denial disables chat input even when server allows', async ({
      page,
      roomPage: _roomPage
    }) => {
      // Admin creates server and room
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      // Deny message.post at room level for everyone
      await denyRoomPermission(page, roomId, 'everyone', 'message.post');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));

      // Chat input should be disabled
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'false');
    });

    test('room grant enables chat input when server has no grant', async ({
      page,
      roomPage: _roomPage
    }) => {
      // Admin creates server and room
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      // Revoke message.post from everyone at server level (neutral, not deny)
      await revokePermission(page, 'everyone', 'message.post');

      // Grant message.post at room level for everyone
      await grantRoomPermission(page, roomId, 'everyone', 'message.post');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));

      // Chat input should be enabled
      const chatInput = page.getByTestId('message-input');
      await expect(chatInput).toHaveAttribute('contenteditable', 'true');
    });

    test('server denial beats room grant for the same role', async ({
      page,
      roomPage: _roomPage
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      // Deny message.post at server level for everyone
      await denyPermission(page, 'everyone', 'message.post');

      // Grant at room level for everyone. Deny-wins means the server deny
      // still blocks the permission for non-owner users.
      await grantRoomPermission(page, roomId, 'everyone', 'message.post');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));

      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'false');
    });
  });

  test.describe('message.react — Reaction Buttons', () => {
    test('room denial hides reaction buttons', async ({ page, roomPage }) => {
      // Admin creates server, room, joins, sends a message
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      await page.goto(routes.room(roomId));
      await roomPage.sendMessage('Test message for reactions');

      // Deny message.react at room level for everyone
      await denyRoomPermission(page, roomId, 'everyone', 'message.react');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      await page.goto(routes.room(roomId));
      await expect(page.getByText('Test message for reactions')).toBeVisible();

      // Open context menu via toolbar — reaction buttons should not be present
      const message = roomPage.getMessage('Test message for reactions');
      await message.expectContextMenuNoReaction();
    });

    test('room grant shows reaction buttons when server has no grant', async ({
      page,
      roomPage
    }) => {
      // Admin creates server, room, joins, sends a message
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      await page.goto(routes.room(roomId));
      await roomPage.sendMessage('Test message for reactions grant');

      // Revoke message.react from everyone at server level (neutral, NOT deny)
      await revokePermission(page, 'everyone', 'message.react');

      // Grant message.react at room level for everyone
      await grantRoomPermission(page, roomId, 'everyone', 'message.react');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      await page.goto(routes.room(roomId));
      await expect(page.getByText('Test message for reactions grant')).toBeVisible();

      // Open context menu via toolbar — reaction buttons should be visible
      const message = roomPage.getMessage('Test message for reactions grant');
      await message.expectContextMenuHasReaction();
    });
  });

  // The `message.edit-own` permission was retired — authors can always edit
  // their own messages (subject only to the edit window). The describe
  // block that used to deny it via a room-scope override and assert the
  // Edit button disappeared no longer maps to a real backend code path.
  // See .claude/rules/authorization.md → "message moderation".

  test.describe('message.manage — Delete Button', () => {
    test('room grant enables Delete on other users messages', async ({ page, roomPage }) => {
      // Admin creates server, room, joins, sends a message
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      await page.goto(routes.room(roomId));
      await roomPage.sendMessage('Admin only message');

      // Grant message.manage at room level for everyone. (Replaces the
      // retired message.delete-any / message.edit-any duo — see ADR + Phase
      // 5 task in CLAUDE.md.)
      await grantRoomPermission(page, roomId, 'everyone', 'message.manage');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      await page.goto(routes.room(roomId));
      await expect(page.getByText('Admin only message')).toBeVisible();

      // Open context menu via toolbar — delete button should be visible
      // (room-level message.manage grant covers the previous message.delete-any).
      const message = roomPage.getMessage('Admin only message');
      await message.expectContextMenuHasDelete();
    });
  });

  test.describe('Per-Room Isolation', () => {
    test('override in one room does not affect another room', async ({
      page,
      roomPage: _roomPage
    }) => {
      // Admin loads the primary server and two rooms
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomAId = await createRoomViaAPI(page, `rooma${Date.now()}`);
      const roomBId = await createRoomViaAPI(page, `roomb${Date.now()}`);
      await joinRoomViaAPI(page, roomAId);
      await joinRoomViaAPI(page, roomBId);

      // Deny message.post only in room A
      await denyRoomPermission(page, roomAId, 'everyone', 'message.post');

      // Create second user, join both rooms
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomAId);
      await joinRoomViaAPI(page, roomBId);

      // Room A: chat input should be disabled
      await page.goto(routes.room(roomAId));
      const chatInputA = page.getByTestId('message-input');
      await expect(chatInputA).toHaveAttribute('contenteditable', 'false');

      // Room B: chat input should be enabled
      await page.goto(routes.room(roomBId));
      const chatInputB = page.getByTestId('message-input');
      await expect(chatInputB).toHaveAttribute('contenteditable', 'true');
    });
  });

  test.describe('Backend Enforcement', () => {
    test('room denial enforced by backend, not just UI', async ({ page }) => {
      // Admin creates server and room
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      // Deny message.post at room level for everyone
      await denyRoomPermission(page, roomId, 'everyone', 'message.post');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Try to post directly via GraphQL API (bypassing UI)
      const result = await postMessageViaAPI(page, roomId, 'Sneaky message');
      expect(result).toBeNull();
    });

    test('server denial beats room grant for the same role (backend enforcement)', async ({
      page
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      const adminMsg = await postMessageViaAPI(page, roomId, 'React to this');
      expect(adminMsg).not.toBeNull();

      // Deny message.react at server level for everyone
      await denyPermission(page, 'everyone', 'message.react');

      // Grant message.react at room level. Deny-wins means the server deny
      // still blocks the permission for non-owner users.
      await grantRoomPermission(page, roomId, 'everyone', 'message.react');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      const success = await addReactionViaAPI(page, roomId, adminMsg!.id, 'thumbsup');
      expect(success).toBe(false);
    });
  });
});

// ============================================================================
// Permission resolution tests
// ============================================================================

async function createServerRole(
  page: Page,
  name: string,
  displayName: string,
  description: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: CreateRoleInput!) {
				createRole(input: $input) { name }
			}`,
      variables: { input: { name, displayName, description } }
    }
  });
  if (!resp.ok()) {
    throw new Error(`createRole HTTP failed: ${resp.status()} - ${await resp.text()}`);
  }
  const data = await resp.json();
  if (data.errors || !data.data?.createRole) {
    throw new Error(`createRole failed: ${JSON.stringify(data)}`);
  }
}

async function assignServerRole(page: Page, userId: string, roleName: string): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: AssignRoleInput!) {
				assignRole(input: $input)
			}`,
      variables: { input: { userId, roleName } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.assignRole).toBe(true);
}

async function reorderServerRoles(page: Page, roleNames: string[]): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: ReorderRolesInput!) {
				reorderRoles(input: $input) { name position }
			}`,
      variables: { input: { roleNames } }
    }
  });
  expect(resp.ok()).toBeTruthy();
}

test.describe('Permission-only Resolution', () => {
  test.describe('#general room - default posting', () => {
    test('all server members can post to #general by default', async ({ page, roomPage }) => {
      // Owner loads the primary server (auto-creates #general and #announcements rooms)
      const _owner = await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Hierarchy Test ${Date.now()}`);
      const generalRoomId = await getRoomByName(page, 'general');

      // Create regular member
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, generalRoomId);

      // Member should be able to post
      await page.goto(routes.room(generalRoomId));
      const chatInput = page.getByTestId('message-input');
      await expect(chatInput).toHaveAttribute('contenteditable', 'true');

      // Actually post a message
      await roomPage.sendMessage('Hello from a regular member!');
      await expect(page.getByText('Hello from a regular member!')).toBeVisible();
    });

    test('muted members cannot post to #general (role denial wins)', async ({
      page,
      roomPage: _roomPage
    }) => {
      // Issue #330: usePrimaryServerViaAPI re-logs in as e2eadmin; subsequent admin
      // operations stay on that session instead of bouncing back through a
      // fresh "owner" account that the bootstrap server wouldn't recognise.
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Muted Test ${Date.now()}`);
      const generalRoomId = await getRoomByName(page, 'general');

      // Create "muted" role
      await createServerRole(page, 'muted', 'Muted', 'Cannot post messages');

      // Reorder remains display metadata; the permission denial itself is what
      // blocks posting under the deny-wins resolver.
      await reorderServerRoles(page, ['muted']);

      // Deny message.post for the muted role at room level
      await denyRoomPermission(page, generalRoomId, 'muted', 'message.post');

      // Create member and assign muted role (still authed as e2eadmin from usePrimaryServerViaAPI).
      const member = await createSecondTestUser(page);
      await assignServerRole(page, member.id!, 'muted');

      // Login as member
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, generalRoomId);

      // Member should NOT be able to post (muted role denial takes precedence)
      await page.goto(routes.room(generalRoomId));
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'false');
    });
  });

  test.describe('#announcements room - restricted posting', () => {
    test('announcements room auto-configures permissions (owner can post, member cannot)', async ({
      page,
      roomPage
    }) => {
      // Owner loads the primary server - this auto-creates #announcements with restricted permissions
      const _owner = await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Announcements Test ${Date.now()}`);
      const announcementsRoomId = await getRoomByName(page, 'announcements');

      // Owner should be able to post
      await page.goto(routes.room(announcementsRoomId));
      const ownerChatInput = page.getByTestId('message-input');
      await expect(ownerChatInput).toHaveAttribute('contenteditable', 'true');
      await roomPage.sendMessage('Important announcement from owner!');
      await expect(page.getByText('Important announcement from owner!')).toBeVisible();

      // Create regular member
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, announcementsRoomId);

      // Member should NOT be able to post
      await page.goto(routes.room(announcementsRoomId));
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'false');

      // But member can still see the announcement
      await expect(page.getByText('Important announcement from owner!')).toBeVisible();
    });

    test('admin cannot post root messages in announcements room', async ({ page }) => {
      // Owner loads the primary server - this auto-creates #announcements with restricted permissions
      const _owner = await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Admin Ann Test ${Date.now()}`);
      const announcementsRoomId = await getRoomByName(page, 'announcements');

      // Create member and assign admin role
      const admin = await createSecondTestUser(page);
      await assignServerRole(page, admin.id!, 'admin');

      // Login as admin
      await logoutUser(page);
      await loginUser(page, admin.login, admin.password);
      await joinRoomViaAPI(page, announcementsRoomId);

      await page.goto(routes.room(announcementsRoomId));
      const chatInput = page.getByTestId('message-input');
      await expect(chatInput).toHaveAttribute('contenteditable', 'false');
    });

    test('moderator cannot post root messages in announcements room', async ({ page }) => {
      // Owner loads the primary server - this auto-creates #announcements with restricted permissions
      const _owner = await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Mod Ann Test ${Date.now()}`);
      const announcementsRoomId = await getRoomByName(page, 'announcements');

      // Create member and assign moderator role
      const mod = await createSecondTestUser(page);
      await assignServerRole(page, mod.id!, 'moderator');

      // Login as moderator
      await logoutUser(page);
      await loginUser(page, mod.login, mod.password);
      await joinRoomViaAPI(page, announcementsRoomId);

      await page.goto(routes.room(announcementsRoomId));
      const chatInput = page.getByTestId('message-input');
      await expect(chatInput).toHaveAttribute('contenteditable', 'false');
    });
  });

  test.describe('message.post-in-thread — Posting in Threads', () => {
    test('message.post-in-thread denied disables thread composer', async ({ page, roomPage }) => {
      // Admin creates server and room, posts a root message
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);
      const rootMsg = await postMessageViaAPI(page, roomId, 'Root for post-in-thread test');
      expect(rootMsg).not.toBeNull();

      // Deny message.post-in-thread at room level for everyone
      await denyRoomPermission(page, roomId, 'everyone', 'message.post-in-thread');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Navigate to room, open thread via direct URL
      await page.goto(routes.thread(roomId, rootMsg!.id));
      await roomPage.expectThreadPaneVisible();

      // Thread reply input should be disabled
      await expect(page.getByTestId('thread-reply-input')).toHaveAttribute(
        'contenteditable',
        'false'
      );
    });

    test('message.post-in-thread denied blocks all thread replies via API', async ({ page }) => {
      // Admin creates server and room
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);
      const rootMsg = await postMessageViaAPI(page, roomId, 'Root for post-in-thread API test');
      expect(rootMsg).not.toBeNull();

      // Deny message.post-in-thread at room level for everyone
      await denyRoomPermission(page, roomId, 'everyone', 'message.post-in-thread');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Posting in thread should be denied (no start_thread/post_in_thread split — all blocked)
      const replied = await replyToMessageViaAPI(page, roomId, rootMsg!.id, 'This should fail');
      expect(replied).toBeNull();
    });

    test('message.post-in-thread denied does not affect root posting', async ({ page }) => {
      // Admin creates server and room
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      // Deny message.post-in-thread at room level for everyone
      await denyRoomPermission(page, roomId, 'everyone', 'message.post-in-thread');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Root posting should still work
      const posted = await postMessageViaAPI(page, roomId, 'Member can still post root');
      expect(posted).not.toBeNull();
    });
  });

  test.describe('message.post — Independence from Thread Permissions', () => {
    test('message.post denied does not affect thread operations', async ({ page }) => {
      // Admin creates server and room, posts a root message
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);
      const rootMsg = await postMessageViaAPI(page, roomId, 'Root for post-denied test');
      expect(rootMsg).not.toBeNull();

      // Deny message.post at room level for everyone (but keep thread perms)
      await denyRoomPermission(page, roomId, 'everyone', 'message.post');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Root posting should be denied
      const posted = await postMessageViaAPI(page, roomId, 'This should fail');
      expect(posted).toBeNull();

      // Starting a new thread should still work
      const replied = await replyToMessageViaAPI(
        page,
        roomId,
        rootMsg!.id,
        'Member can start thread'
      );
      expect(replied).not.toBeNull();

      // Posting in existing thread should still work
      const replied2 = await replyToMessageViaAPI(
        page,
        roomId,
        rootMsg!.id,
        'Member can post in thread'
      );
      expect(replied2).not.toBeNull();
    });
  });

  // ==========================================================================
  // room.list (Discoverability) vs room.join (Joinability)
  // ==========================================================================
  //
  // A room can be listable (visible in the Overview / room directory) but
  // not directly joinable — the state a future request-to-join flow keys
  // off. The directory must surface the room and render the "Restricted"
  // indicator instead of a Join button.
  test.describe('room.list vs room.join — listable but not joinable', () => {
    test('room with room.join denied at room scope still appears in the directory, with no Join button', async ({
      page
    }) => {
      // Admin creates a room and denies `room.join` for everyone at room
      // scope. `room.list` stays at its default (allow), so the room is
      // still discoverable.
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page, `restricted-${Date.now()}`);
      await denyRoomPermission(page, roomId, 'everyone', 'room.join');

      // A second user signs in. They haven't joined this room and never
      // will be able to via the directory, but they should be able to see
      // it.
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to the Overview / room directory. /chat/- IS the
      // Overview, so a goto is enough — clicking the sidebar nav's
      // "Overview" link is redundant AND collides with the empty-state's
      // "Overview" link rendered when the viewer has no joined rooms.
      await page.goto(routes.chat);

      // The restricted room is listed.
      const row = page.locator('li', { hasText: /restricted-/ }).first();
      await expect(row).toBeVisible();

      // It carries the "Restricted" affordance instead of a Join button.
      // Match the exact text so we don't collide with the room name itself
      // (the `restricted-{timestamp}` test fixture starts with the same
      // substring).
      await expect(row.getByText('Restricted', { exact: true })).toBeVisible();
      await expect(row.getByRole('button', { name: 'Join' })).toHaveCount(0);
    });
  });
});
