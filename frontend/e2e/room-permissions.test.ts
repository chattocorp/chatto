import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser, type TestUser } from './fixtures/testUser';
import * as routes from './routes';

interface TestSpace {
  id: string;
  name: string;
}

// ============================================================================
// GraphQL Helper Functions
// ============================================================================

async function createSpaceViaAPI(page: Page, name?: string): Promise<TestSpace> {
  const spaceName = name ?? `Room Perms Space ${Date.now()}`;
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: CreateSpaceInput!) { createSpace(input: $input) { id name } }`,
      variables: { input: { name: spaceName, description: 'Room permissions test space' } }
    }
  });
  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.createSpace).toBeTruthy();
  return { id: data.data.createSpace.id, name: data.data.createSpace.name };
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
  await page.request.post('/auth/logout');
}

async function joinSpaceViaAPI(page: Page, spaceId: string): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: JoinSpaceInput!) { joinSpace(input: $input) }`,
      variables: { input: { spaceId } }
    }
  });
  expect(resp.ok()).toBeTruthy();
}

async function createRoomViaAPI(page: Page, spaceId: string, name?: string): Promise<string> {
  const roomName = name ?? `room${Date.now()}`;
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: CreateRoomInput!) { createRoom(input: $input) { id name } }`,
      variables: { input: { spaceId, name: roomName } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  const data = await resp.json();
  if (data.errors || !data.data?.createRoom) {
    throw new Error(`createRoom failed: ${JSON.stringify(data)}`);
  }
  return data.data.createRoom.id;
}

async function getRoomByName(page: Page, spaceId: string, roomName: string): Promise<string> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `query($spaceId: ID!) { space(id: $spaceId) { rooms { id name } } }`,
      variables: { spaceId }
    }
  });
  expect(resp.ok()).toBeTruthy();
  const data = await resp.json();
  const rooms = data.data?.space?.rooms;
  if (!rooms) {
    throw new Error(`Failed to get rooms: ${JSON.stringify(data)}`);
  }
  const room = rooms.find((r: { name: string }) => r.name.toLowerCase() === roomName.toLowerCase());
  if (!room) {
    throw new Error(`Room '${roomName}' not found in space ${spaceId}`);
  }
  return room.id;
}

async function joinRoomViaAPI(page: Page, spaceId: string, roomId: string): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: JoinRoomInput!) { joinRoom(input: $input) }`,
      variables: { input: { spaceId, roomId } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.joinRoom).toBe(true);
}

async function _grantSpacePermission(
  page: Page,
  spaceId: string,
  role: string,
  permission: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: GrantSpacePermissionInput!) { grantSpacePermission(input: $input) }`,
      variables: { input: { spaceId, role, permission } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.grantSpacePermission).toBe(true);
}

async function denySpacePermission(
  page: Page,
  spaceId: string,
  role: string,
  permission: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: DenySpacePermissionInput!) { denySpacePermission(input: $input) }`,
      variables: { input: { spaceId, role, permission } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.denySpacePermission).toBe(true);
}

async function revokeSpacePermission(
  page: Page,
  spaceId: string,
  role: string,
  permission: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: RevokeSpacePermissionInput!) { revokeSpacePermission(input: $input) }`,
      variables: { input: { spaceId, role, permission } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.revokeSpacePermission).toBe(true);
}

async function grantRoomPermission(
  page: Page,
  spaceId: string,
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
      variables: { input: { spaceId, roomId, role, permission } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.grantRoomPermission).toBe(true);
}

async function denyRoomPermission(
  page: Page,
  spaceId: string,
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
      variables: { input: { spaceId, roomId, role, permission } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.denyRoomPermission).toBe(true);
}

async function postMessageViaAPI(
  page: Page,
  spaceId: string,
  roomId: string,
  body: string
): Promise<{ id: string } | null> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) {
				postMessage(input: $input) { id }
			}`,
      variables: { input: { spaceId, roomId, body } }
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
  spaceId: string,
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
      variables: { input: { spaceId, roomId, body, inThread } }
    }
  });
  const data = await resp.json();
  if (data.errors) {
    return null;
  }
  return data.data?.postMessage ?? null;
}

async function postReplyViaAPI(
  page: Page,
  spaceId: string,
  roomId: string,
  inReplyTo: string,
  body: string,
  inThread?: string
): Promise<{ id: string } | null> {
  const input: Record<string, string> = { spaceId, roomId, body, inReplyTo };
  if (inThread) input.inThread = inThread;
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: PostMessageInput!) {
				postMessage(input: $input) { id }
			}`,
      variables: { input }
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
  spaceId: string,
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
      variables: { input: { spaceId, roomId, messageEventId, emoji } }
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
    test('room denial disables chat input even when space allows', async ({
      page,
      roomPage: _roomPage
    }) => {
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Deny message.post at room level for everyone
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.post');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));

      // Chat input should be disabled
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'false');
    });

    test('room grant enables chat input when space has no grant', async ({
      page,
      roomPage: _roomPage
    }) => {
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Revoke message.post from everyone at space level (neutral, not deny)
      await revokeSpacePermission(page, space.id, 'everyone', 'message.post');

      // Grant message.post at room level for everyone
      await grantRoomPermission(page, space.id, roomId, 'everyone', 'message.post');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));

      // Chat input should be enabled
      const chatInput = page.getByTestId('message-input');
      await expect(chatInput).toHaveAttribute('contenteditable', 'true');
    });

    test('space denial overrides room grant (deny-always-wins)', async ({
      page,
      roomPage: _roomPage
    }) => {
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Deny message.post at space level
      await denySpacePermission(page, space.id, 'everyone', 'message.post');

      // Also grant at room level (should NOT override the space denial)
      await grantRoomPermission(page, space.id, roomId, 'everyone', 'message.post');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));

      // Chat input should still be disabled (deny always wins)
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'false');
    });
  });

  test.describe('message.react — Reaction Buttons', () => {
    test('room denial hides reaction buttons', async ({ page, roomPage }) => {
      // Admin creates space, room, joins, sends a message
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      await page.goto(routes.room(roomId));
      await roomPage.sendMessage('Test message for reactions');

      // Deny message.react at room level for everyone
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.react');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      await page.goto(routes.room(roomId));
      await expect(page.getByText('Test message for reactions')).toBeVisible();

      // Open context menu via toolbar — reaction buttons should not be present
      const message = roomPage.getMessage('Test message for reactions');
      await message.expectContextMenuNoReaction();
    });

    test('room grant shows reaction buttons when space has no grant', async ({
      page,
      roomPage
    }) => {
      // Admin creates space, room, joins, sends a message
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      await page.goto(routes.room(roomId));
      await roomPage.sendMessage('Test message for reactions grant');

      // Revoke message.react from everyone at space level (neutral, NOT deny)
      await revokeSpacePermission(page, space.id, 'everyone', 'message.react');

      // Grant message.react at room level for everyone
      await grantRoomPermission(page, space.id, roomId, 'everyone', 'message.react');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      await page.goto(routes.room(roomId));
      await expect(page.getByText('Test message for reactions grant')).toBeVisible();

      // Open context menu via toolbar — reaction buttons should be visible
      const message = roomPage.getMessage('Test message for reactions grant');
      await message.expectContextMenuHasReaction();
    });
  });

  test.describe('message.edit-own — Edit Button', () => {
    test('room denial hides edit button for own messages', async ({ page, roomPage }) => {
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Deny message.edit-own at room level for everyone
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.edit-own');

      // Create second user, join space and room, send a message
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      await page.goto(routes.room(roomId));
      await roomPage.sendMessage('My editable message');

      // Open context menu via toolbar — edit button should not be present
      const message = roomPage.getMessage('My editable message');
      await message.expectContextMenuNoEdit();
    });
  });

  test.describe('message.delete-any — Delete Button', () => {
    test('room grant enables delete-any button on other users messages', async ({
      page,
      roomPage
    }) => {
      // Admin creates space, room, joins, sends a message
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      await page.goto(routes.room(roomId));
      await roomPage.sendMessage('Admin only message');

      // Grant message.delete-any at room level for everyone
      await grantRoomPermission(page, space.id, roomId, 'everyone', 'message.delete-any');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      await page.goto(routes.room(roomId));
      await expect(page.getByText('Admin only message')).toBeVisible();

      // Open context menu via toolbar — delete button should be visible (has message.delete-any via room grant)
      const message = roomPage.getMessage('Admin only message');
      await message.expectContextMenuHasDelete();
    });
  });

  test.describe('Per-Room Isolation', () => {
    test('override in one room does not affect another room', async ({
      page,
      roomPage: _roomPage
    }) => {
      // Admin creates space and two rooms
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomAId = await createRoomViaAPI(page, space.id, `rooma${Date.now()}`);
      const roomBId = await createRoomViaAPI(page, space.id, `roomb${Date.now()}`);
      await joinRoomViaAPI(page, space.id, roomAId);
      await joinRoomViaAPI(page, space.id, roomBId);

      // Deny message.post only in room A
      await denyRoomPermission(page, space.id, roomAId, 'everyone', 'message.post');

      // Create second user, join space and both rooms
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomAId);
      await joinRoomViaAPI(page, space.id, roomBId);

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
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Deny message.post at room level for everyone
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.post');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Try to post directly via GraphQL API (bypassing UI)
      const result = await postMessageViaAPI(page, space.id, roomId, 'Sneaky message');
      expect(result).toBeNull();
    });

    test('space denial cannot be overridden by room grant (backend enforcement)', async ({
      page
    }) => {
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Admin sends a message for the reaction target
      const adminMsg = await postMessageViaAPI(page, space.id, roomId, 'React to this');
      expect(adminMsg).not.toBeNull();

      // Deny message.react at space level
      await denySpacePermission(page, space.id, 'everyone', 'message.react');

      // Grant message.react at room level (should NOT override space deny)
      await grantRoomPermission(page, space.id, roomId, 'everyone', 'message.react');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Try to add reaction via API (bypassing UI)
      const success = await addReactionViaAPI(page, space.id, roomId, adminMsg!.id, 'thumbsup');
      expect(success).toBe(false);
    });
  });
});

// ============================================================================
// Role Hierarchy Tests
// ============================================================================

async function createSpaceRole(
  page: Page,
  spaceId: string,
  name: string,
  displayName: string,
  description: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: CreateSpaceRoleInput!) {
				createSpaceRole(input: $input) { name }
			}`,
      variables: { input: { spaceId, name, displayName, description } }
    }
  });
  if (!resp.ok()) {
    throw new Error(`createSpaceRole HTTP failed: ${resp.status()} - ${await resp.text()}`);
  }
  const data = await resp.json();
  if (data.errors || !data.data?.createSpaceRole) {
    throw new Error(`createSpaceRole failed: ${JSON.stringify(data)}`);
  }
}

async function assignSpaceRole(
  page: Page,
  spaceId: string,
  userId: string,
  roleName: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: AssignSpaceRoleInput!) {
				assignSpaceRole(input: $input)
			}`,
      variables: { input: { spaceId, userId, roleName } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.assignSpaceRole).toBe(true);
}

async function reorderSpaceRoles(page: Page, spaceId: string, roleNames: string[]): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: ReorderSpaceRolesInput!) {
				reorderSpaceRoles(input: $input) { name position }
			}`,
      variables: { input: { spaceId, roleNames } }
    }
  });
  expect(resp.ok()).toBeTruthy();
}

test.describe('Role Hierarchy Permission Resolution', () => {
  test.describe('#general room - default posting', () => {
    test('all space members can post to #general by default', async ({ page, roomPage }) => {
      // Owner creates space (auto-creates #general and #announcements rooms)
      const _owner = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page, `Hierarchy Test ${Date.now()}`);
      const generalRoomId = await getRoomByName(page, space.id, 'general');

      // Create regular member
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, generalRoomId);

      // Member should be able to post
      await page.goto(routes.room(generalRoomId));
      const chatInput = page.getByTestId('message-input');
      await expect(chatInput).toHaveAttribute('contenteditable', 'true');

      // Actually post a message
      await roomPage.sendMessage('Hello from a regular member!');
      await expect(page.getByText('Hello from a regular member!')).toBeVisible();
    });

    test('muted members cannot post to #general (higher-ranked role denial wins)', async ({
      page,
      roomPage: _roomPage
    }) => {
      // Owner creates space (auto-creates #general and #announcements rooms)
      const owner = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page, `Muted Test ${Date.now()}`);
      const generalRoomId = await getRoomByName(page, space.id, 'general');

      // Create "muted" role
      await createSpaceRole(page, space.id, 'muted', 'Muted', 'Cannot post messages');

      // Reorder to put "muted" first (position 1), giving it higher rank than "everyone"
      // This is important: role hierarchy means lower position = higher rank = checked first
      await reorderSpaceRoles(page, space.id, ['muted']);

      // Deny message.post for the muted role at room level
      await denyRoomPermission(page, space.id, generalRoomId, 'muted', 'message.post');

      // Create member and assign muted role
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, owner.login, owner.password);
      await assignSpaceRole(page, space.id, member.id!, 'muted');

      // Login as member
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, generalRoomId);

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
      // Owner creates space - this auto-creates #announcements with restricted permissions
      const _owner = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page, `Announcements Test ${Date.now()}`);
      const announcementsRoomId = await getRoomByName(page, space.id, 'announcements');

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
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, announcementsRoomId);

      // Member should NOT be able to post
      await page.goto(routes.room(announcementsRoomId));
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'false');

      // But member can still see the announcement
      await expect(page.getByText('Important announcement from owner!')).toBeVisible();
    });

    test('admin can post in announcements room', async ({ page, roomPage }) => {
      // Owner creates space - this auto-creates #announcements with restricted permissions
      const _owner = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page, `Admin Ann Test ${Date.now()}`);
      const announcementsRoomId = await getRoomByName(page, space.id, 'announcements');

      // Create member and assign admin role
      const admin = await createSecondTestUser(page);
      await assignSpaceRole(page, space.id, admin.id!, 'admin');

      // Login as admin
      await logoutUser(page);
      await loginUser(page, admin.login, admin.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, announcementsRoomId);

      // Admin should be able to post
      await page.goto(routes.room(announcementsRoomId));
      const chatInput = page.getByTestId('message-input');
      await expect(chatInput).toHaveAttribute('contenteditable', 'true');
      await roomPage.sendMessage('Announcement from admin!');
      await expect(page.getByText('Announcement from admin!')).toBeVisible();
    });

    test('moderator can post in announcements room', async ({ page, roomPage }) => {
      // Owner creates space - this auto-creates #announcements with restricted permissions
      const _owner = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page, `Mod Ann Test ${Date.now()}`);
      const announcementsRoomId = await getRoomByName(page, space.id, 'announcements');

      // Create member and assign moderator role
      const mod = await createSecondTestUser(page);
      await assignSpaceRole(page, space.id, mod.id!, 'moderator');

      // Login as moderator
      await logoutUser(page);
      await loginUser(page, mod.login, mod.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, announcementsRoomId);

      // Moderator should be able to post
      await page.goto(routes.room(announcementsRoomId));
      const chatInput = page.getByTestId('message-input');
      await expect(chatInput).toHaveAttribute('contenteditable', 'true');
      await roomPage.sendMessage('Announcement from moderator!');
      await expect(page.getByText('Announcement from moderator!')).toBeVisible();
    });
  });

  test.describe('message.post-in-thread — Posting in Threads', () => {
    test('message.post-in-thread denied disables thread composer', async ({ page, roomPage }) => {
      // Admin creates space and room, posts a root message
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);
      const rootMsg = await postMessageViaAPI(
        page,
        space.id,
        roomId,
        'Root for post-in-thread test'
      );
      expect(rootMsg).not.toBeNull();

      // Deny message.post-in-thread at room level for everyone
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.post-in-thread');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

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
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);
      const rootMsg = await postMessageViaAPI(
        page,
        space.id,
        roomId,
        'Root for post-in-thread API test'
      );
      expect(rootMsg).not.toBeNull();

      // Deny message.post-in-thread at room level for everyone
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.post-in-thread');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Posting in thread should be denied (no start_thread/post_in_thread split — all blocked)
      const replied = await replyToMessageViaAPI(
        page,
        space.id,
        roomId,
        rootMsg!.id,
        'This should fail'
      );
      expect(replied).toBeNull();
    });

    test('message.post-in-thread denied does not affect root posting', async ({ page }) => {
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Deny message.post-in-thread at room level for everyone
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.post-in-thread');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Root posting should still work
      const posted = await postMessageViaAPI(page, space.id, roomId, 'Member can still post root');
      expect(posted).not.toBeNull();
    });
  });

  test.describe('message.post — Independence from Thread Permissions', () => {
    test('message.post denied does not affect thread operations', async ({ page }) => {
      // Admin creates space and room, posts a root message
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);
      const rootMsg = await postMessageViaAPI(page, space.id, roomId, 'Root for post-denied test');
      expect(rootMsg).not.toBeNull();

      // Deny message.post at room level for everyone (but keep thread perms)
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.post');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Root posting should be denied
      const posted = await postMessageViaAPI(page, space.id, roomId, 'This should fail');
      expect(posted).toBeNull();

      // Starting a new thread should still work
      const replied = await replyToMessageViaAPI(
        page,
        space.id,
        roomId,
        rootMsg!.id,
        'Member can start thread'
      );
      expect(replied).not.toBeNull();

      // Posting in existing thread should still work
      const replied2 = await replyToMessageViaAPI(
        page,
        space.id,
        roomId,
        rootMsg!.id,
        'Member can post in thread'
      );
      expect(replied2).not.toBeNull();
    });
  });

  test.describe('message.reply — Reply Attribution in Room', () => {
    test('message.reply denied hides Reply button in room context menu', async ({
      page,
      roomPage
    }) => {
      // Admin creates space and room, posts a message
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);
      await postMessageViaAPI(page, space.id, roomId, 'Message for reply test');

      // Deny message.reply at room level for everyone
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.reply');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Navigate to room
      await page.goto(routes.room(roomId));
      await expect(page.getByText('Message for reply test')).toBeVisible();

      // Reply button should be hidden in context menu
      const message = roomPage.getMessage('Message for reply test');
      await message.expectContextMenuNoReply();
    });

    test('message.reply denied blocks reply attribution via API', async ({ page }) => {
      // Admin creates space and room, posts a message
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);
      const rootMsg = await postMessageViaAPI(page, space.id, roomId, 'Message for reply API test');
      expect(rootMsg).not.toBeNull();

      // Deny message.reply at room level for everyone
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.reply');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Posting with inReplyTo should be denied
      const replied = await postReplyViaAPI(
        page,
        space.id,
        roomId,
        rootMsg!.id,
        'Reply should fail'
      );
      expect(replied).toBeNull();

      // But posting without inReplyTo should still work
      const posted = await postMessageViaAPI(page, space.id, roomId, 'Plain post should work');
      expect(posted).not.toBeNull();
    });
  });

  test.describe('message.reply-in-thread — Reply Attribution in Thread', () => {
    test('message.reply-in-thread denied blocks reply attribution in threads via API', async ({
      page
    }) => {
      // Admin creates space and room, posts a message with a thread
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);
      const rootMsg = await postMessageViaAPI(page, space.id, roomId, 'Root for thread reply test');
      expect(rootMsg).not.toBeNull();
      const threadReply = await replyToMessageViaAPI(
        page,
        space.id,
        roomId,
        rootMsg!.id,
        'First thread reply'
      );
      expect(threadReply).not.toBeNull();

      // Deny message.reply-in-thread at room level for everyone
      await denyRoomPermission(page, space.id, roomId, 'everyone', 'message.reply-in-thread');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Posting in thread with inReplyTo should be denied
      const replied = await postReplyViaAPI(
        page,
        space.id,
        roomId,
        threadReply!.id,
        'Thread reply should fail',
        rootMsg!.id
      );
      expect(replied).toBeNull();

      // But posting in thread without inReplyTo should still work
      const posted = await replyToMessageViaAPI(
        page,
        space.id,
        roomId,
        rootMsg!.id,
        'Plain thread post should work'
      );
      expect(posted).not.toBeNull();
    });
  });
});
