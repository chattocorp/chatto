import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  activatePrivilegedMode,
  clearUserPermissionOverride,
  createAndLoginTestUser,
  denyUserPermission,
  logoutCurrentUser,
  loginAsAdminAndUsePrimaryServer,
  type TestUser
} from './fixtures/testUser';
import { withLoggedInServerWindow } from './fixtures/serverUser';
import { browserAuthenticationHeaders } from './fixtures/csrf';
import {
  connectPost,
  connectPostResponse,
  createRoomViaConnect,
  expectPermissionDecisionUpdate,
  type E2EAdminRole,
  type E2EPermissionDecision,
  type E2EPermissionDecisionUpdateResponse,
  getDefaultRoomGroupIdViaConnect,
  getRoomIdByNameViaConnect,
  joinRoomViaConnect,
  unwrapAdminRole
} from './fixtures/connectHelpers';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

interface TestServer {
  id: string;
  name: string;
}

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
  const resp = await page.request.post('/auth/browser/login', {
    headers: await browserAuthenticationHeaders(page),
    data: { login, password }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).success).toBe(true);
}

async function logoutUser(page: Page): Promise<void> {
  await logoutCurrentUser(page);
}

async function createRoomViaAPI(page: Page, name?: string, description = ''): Promise<string> {
  const roomName = name ?? `room${Date.now()}`;
  const groupId = await getDefaultRoomGroupIdViaConnect(page);
  return createRoomViaConnect(page, roomName, groupId, description);
}

function shortSuffix(): string {
  return Date.now().toString(36).slice(-6);
}

async function getRoomByName(page: Page, roomName: string): Promise<string> {
  return getRoomIdByNameViaConnect(page, roomName);
}

async function joinRoomViaAPI(page: Page, roomId: string): Promise<void> {
  await joinRoomViaConnect(page, roomId);
}

async function revokePermission(page: Page, role: string, permission: string): Promise<void> {
  await setRolePermission(page, role, permission, 'PERMISSION_DECISION_NONE');
}

async function grantRoomPermission(
  page: Page,
  roomId: string,
  role: string,
  permission: string
): Promise<void> {
  await setRolePermission(page, role, permission, 'PERMISSION_DECISION_ALLOW', roomId);
}

/**
 * Denies a permission to one user in one room. Roles, `everyone` included,
 * only grant permissions, so a room-scope deny targets the user (ADR-116).
 */
async function denyUserRoomPermission(
  page: Page,
  roomId: string,
  userId: string,
  permission: string
): Promise<void> {
  await denyUserPermission(page, userId, permission, {
    kind: 'PERMISSION_SCOPE_KIND_ROOM',
    id: roomId
  });
}

/** Clears the room-scope setting of a permission on one user. */
async function clearUserRoomPermission(
  page: Page,
  roomId: string,
  userId: string,
  permission: string
): Promise<void> {
  await clearUserPermissionOverride(page, userId, permission, {
    kind: 'PERMISSION_SCOPE_KIND_ROOM',
    id: roomId
  });
}

async function setRolePermission(
  page: Page,
  roleName: string,
  permission: string,
  decision: E2EPermissionDecision,
  roomId?: string
): Promise<void> {
  const scope = roomId
    ? ({ kind: 'PERMISSION_SCOPE_KIND_ROOM', id: roomId } as const)
    : ({ kind: 'PERMISSION_SCOPE_KIND_SERVER' } as const);
  const data = await connectPost<E2EPermissionDecisionUpdateResponse>(
    page,
    'chatto.admin.v1.AdminPermissionService/SetRolePermission',
    {
      roleName,
      permission,
      decision,
      scope
    }
  );
  expectPermissionDecisionUpdate(data, { permission, decision, scope });
}

async function postMessageViaAPI(
  page: Page,
  roomId: string,
  body: string,
  options: { createThread?: boolean } = {}
): Promise<{ id: string } | null> {
  const resp = await connectPostResponse(page, 'chatto.api.v1.MessageService/CreateMessage', {
    roomId,
    body,
    createThread: options.createThread ?? false
  });
  if (!resp.ok()) {
    return null;
  }
  const data = (await resp.json()) as { message?: { id?: string } };
  return data.message?.id ? { id: data.message.id } : null;
}

async function replyToMessageViaAPI(
  page: Page,
  roomId: string,
  inThread: string,
  body: string
): Promise<{ id: string } | null> {
  const resp = await connectPostResponse(page, 'chatto.api.v1.MessageService/CreateMessage', {
    roomId,
    body,
    threadRootEventId: inThread
  });
  if (!resp.ok()) {
    return null;
  }
  const data = (await resp.json()) as { message?: { id?: string } };
  return data.message?.id ? { id: data.message.id } : null;
}

async function addReactionViaAPI(
  page: Page,
  roomId: string,
  messageEventId: string,
  emoji: string
): Promise<boolean> {
  const resp = await connectPostResponse(page, 'chatto.api.v1.MessageService/AddReaction', {
    roomId,
    messageEventId,
    emoji
  });
  return resp.ok();
}

// ============================================================================
// Test Scenarios
// ============================================================================

test.describe('Room-Level Permission Overrides', () => {
  test.describe('message.read — Message Content', () => {
    test('live read-mode revocation scrubs content while write-only posting remains available', async ({
      page,
      browser,
      serverURL
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);
      const visibleBody = `Visible before message.read denial ${Date.now()}`;
      expect(await postMessageViaAPI(page, roomId, visibleBody)).not.toBeNull();

      const member = await createSecondTestUser(page);
      const browserErrors: string[] = [];

      await withLoggedInServerWindow(browser, serverURL, member, async ({ page: memberPage }) => {
        let connections = 0;
        memberPage.on('websocket', () => connections++);
        memberPage.on('console', (message) => {
          // The denied request and the nonfatal realtime availability signal
          // are expected when read authority is removed. Keep all other
          // console errors actionable.
          const expectedErrors = new Set([
            'Failed to load resource: the server responded with a status of 403 (Forbidden)',
            '[eventBus:localhost] realtime error {code: room_unavailable, message: room timeline is unavailable, fatal: false}'
          ]);
          if (message.type() === 'error' && !expectedErrors.has(message.text())) {
            browserErrors.push(message.text());
          }
        });
        memberPage.on('pageerror', (error) => browserErrors.push(error.message));

        await joinRoomViaAPI(memberPage, roomId);
        await memberPage.goto(routes.room(roomId));
        await expect(memberPage.getByText(visibleBody)).toBeVisible();
        const originalComposer = await memberPage.getByTestId('message-input').elementHandle();
        const originalShell = await memberPage
          .getByRole('button', { name: 'Toggle sidebar', exact: true })
          .elementHandle();
        const initialConnections = connections;

        await denyUserRoomPermission(page, roomId, member.id!, 'message.read');
        await denyUserRoomPermission(page, roomId, member.id!, 'message.read-interactions');

        const denial = memberPage.getByText(
          'You do not have permission to read messages in this room.'
        );
        await expect(denial).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
        await expect(memberPage.getByText(visibleBody)).toHaveCount(0);
        await expect(memberPage.locator('[role="article"]')).toHaveCount(0);
        expect(await originalComposer!.evaluate((node) => node.isConnected)).toBe(true);
        expect(await originalShell!.evaluate((node) => node.isConnected)).toBe(true);
        expect(connections).toBe(initialConnections);
        await expect(memberPage.getByTestId('message-input')).toHaveAttribute(
          'contenteditable',
          'true'
        );

        const deniedTimeline = await connectPostResponse(
          memberPage,
          'chatto.api.v1.RoomService/GetRoomEvents',
          { roomId, limit: 10 }
        );
        expect(deniedTimeline.status()).toBe(403);
        await expect(deniedTimeline.json()).resolves.toEqual(
          expect.objectContaining({ code: 'permission_denied' })
        );

        const writeOnlyBody = `Posted without message.read ${Date.now()}`;
        const composer = memberPage.getByTestId('message-input');
        await composer.fill(writeOnlyBody);
        await composer.press('Control+Enter');
        await expect(composer).toBeEmpty();
        await expect(memberPage.getByText(writeOnlyBody)).toHaveCount(0);

        await page.goto(routes.room(roomId));
        await expect(page.getByText(writeOnlyBody)).toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });

        // Clearing the user's deny restores the server-scope allow of everyone.
        await clearUserRoomPermission(page, roomId, member.id!, 'message.read');

        await expect(denial).toHaveCount(0, { timeout: TIMEOUTS.REALTIME_EVENT });
        await expect(memberPage.getByText(visibleBody)).toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });
        await expect(memberPage.getByText(writeOnlyBody)).toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });
        expect(await originalComposer!.evaluate((node) => node.isConnected)).toBe(true);
        expect(connections).toBe(initialConnections);
      });

      expect(browserErrors, 'browser console and page errors').toEqual([]);
    });

    test('message.read-interactions reveals the complete thread after a direct mention', async ({
      page,
      browser,
      serverURL
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      const rootBody = `Interaction root ${Date.now()}`;
      const root = await postMessageViaAPI(page, roomId, rootBody, { createThread: true });
      expect(root).not.toBeNull();
      const earlierBody = `Earlier interaction reply ${Date.now()}`;
      const earlierReply = await replyToMessageViaAPI(page, roomId, root!.id, earlierBody);
      expect(earlierReply).not.toBeNull();
      const unrelatedBody = `Unrelated root ${Date.now()}`;
      expect(await postMessageViaAPI(page, roomId, unrelatedBody)).not.toBeNull();

      const member = await createSecondTestUser(page);
      await withLoggedInServerWindow(browser, serverURL, member, async ({ page: memberPage }) => {
        await joinRoomViaAPI(memberPage, roomId);
        await denyUserRoomPermission(page, roomId, member.id!, 'message.read');
        await grantRoomPermission(page, roomId, 'everyone', 'message.read-interactions');

        type TimelineResponse = { page?: { events?: Array<{ id?: string }> } };
        const beforeMention = await connectPost<TimelineResponse>(
          memberPage,
          'chatto.api.v1.RoomService/GetRoomEvents',
          { roomId, limit: 20 }
        );
        expect(beforeMention.page?.events ?? []).toEqual([]);

        await memberPage.goto(routes.room(roomId));
        await expect(memberPage.getByText('No conversations you can read yet.')).toBeVisible();
        await expect(
          memberPage.getByText('This is the beginning of this conversation.')
        ).toHaveCount(0);

        const mentionSuffix = `interaction access ${Date.now()}`;
        const mentionBody = `@${member.login} ${mentionSuffix}`;
        const mentionReply = await replyToMessageViaAPI(page, roomId, root!.id, mentionBody);
        expect(mentionReply).not.toBeNull();

        const roomTimeline = await connectPost<TimelineResponse>(
          memberPage,
          'chatto.api.v1.RoomService/GetRoomEvents',
          { roomId, limit: 20 }
        );
        expect((roomTimeline.page?.events ?? []).map((event) => event.id)).toEqual([root!.id]);

        const threadTimeline = await connectPost<TimelineResponse>(
          memberPage,
          'chatto.api.v1.ThreadService/GetThreadEvents',
          { roomId, threadRootEventId: root!.id, limit: 20 }
        );
        expect((threadTimeline.page?.events ?? []).map((event) => event.id)).toEqual([
          root!.id,
          earlierReply!.id,
          mentionReply!.id
        ]);

        await memberPage.goto(routes.thread(roomId, root!.id));
        await expect(memberPage.getByTestId('thread-pane').getByText(rootBody)).toBeVisible();
        await expect(memberPage.getByText(earlierBody)).toBeVisible();
        await expect(memberPage.getByText(`@${member.displayName} ${mentionSuffix}`)).toBeVisible();
        await expect(memberPage.getByText(unrelatedBody)).toHaveCount(0);

        await memberPage.goto(routes.room(roomId));
        await expect(memberPage.getByTestId('room-main-pane').getByText(rootBody)).toBeVisible();
        await expect(memberPage.getByText(unrelatedBody)).toHaveCount(0);
        await clearUserRoomPermission(page, roomId, member.id!, 'message.read');
        await expect(memberPage.getByText(unrelatedBody)).toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });
      });
    });
  });

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

      // Create second user and deny message.post to them at room level
      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, roomId, member.id!, 'message.post');

      // The member joins the room
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

    test('room grant gives access that the server setting does not give', async ({
      page,
      roomPage: _roomPage
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      // Clear message.post at server level for everyone. Roles only grant
      // (ADR-116).
      await revokePermission(page, 'everyone', 'message.post');

      // Grant at room level for everyone. The nearest decision for that same
      // subject decides.
      await grantRoomPermission(page, roomId, 'everyone', 'message.post');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));

      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'true');
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

      // Create second user and deny message.react to them at room level
      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, roomId, member.id!, 'message.react');

      // The member joins the room
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
  // See cli/AGENTS.md → "message moderation".

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
      await activatePrivilegedMode(page);
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

      // Create second user and deny message.post to them only in room A
      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, roomAId, member.id!, 'message.post');

      // The member joins both rooms
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

      // Create second user and deny message.post to them at room level
      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, roomId, member.id!, 'message.post');

      // The member joins the room
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Try to post directly via ConnectRPC (bypassing UI)
      const result = await postMessageViaAPI(page, roomId, 'Sneaky message');
      expect(result).toBeNull();
    });

    test('room grant gives access that the server setting does not give (backend enforcement)', async ({
      page
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      const adminMsg = await postMessageViaAPI(page, roomId, 'React to this');
      expect(adminMsg).not.toBeNull();

      // Clear message.react at server level for everyone. Roles only grant
      // (ADR-116).
      await revokePermission(page, 'everyone', 'message.react');

      // Grant message.react at room level. The room decision is nearest for
      // this same subject.
      await grantRoomPermission(page, roomId, 'everyone', 'message.react');

      // Create second user, join the room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      const success = await addReactionViaAPI(page, roomId, adminMsg!.id, 'thumbsup');
      expect(success).toBe(true);
    });
  });
});

// ============================================================================
// Permission resolution tests
// ============================================================================

async function assignServerRole(page: Page, userId: string, roleName: string): Promise<void> {
  const data = await connectPost<{ member?: { roles?: string[]; user?: { id?: string } } }>(
    page,
    'chatto.admin.v1.AdminUserService/AssignRole',
    { userId, roleName }
  );
  expect(data.member?.user?.id).toBe(userId);
  expect(data.member?.roles ?? []).toContain(roleName);
}

test.describe('Permission-only Resolution', () => {
  test.describe('#general room - default posting', () => {
    test('joining from an unjoined sidebar room shows inline join and enables posting', async ({
      page,
      roomPage
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Inline Join ${Date.now()}`);
      const description = `Launch coordination ${Date.now()}`;
      const roomId = await createRoomViaAPI(page, `inline-join-${Date.now()}`, description);
      await joinRoomViaAPI(page, roomId);
      const hiddenBody = `Hidden before joining ${Date.now()}`;
      await postMessageViaAPI(page, roomId, hiddenBody);

      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await page.goto(routes.chat);

      const roomLink = page.locator(`a[href="${routes.room(roomId)}"]`).first();
      await expect(roomLink).toHaveCount(0);
      await page.getByTestId('room-group-more').click();
      await expect(roomLink).toBeVisible();
      await roomLink.click();

      await expect(page).toHaveURL(new RegExp(`${routes.room(roomId)}$`));
      await expect(page.getByRole('button', { name: 'Join Room' })).toBeVisible();
      await expect(page.getByTestId('room-join-preview')).toContainText(description);
      await expect(page.getByTestId('room-join-preview')).toContainText('In Lobby');
      await expect(page.getByLabel('Room members')).toContainText('1 member');
      await expect(page.getByText(hiddenBody)).toHaveCount(0);
      await expect(page.locator('dialog[open]')).toHaveCount(0);

      await page.getByRole('button', { name: 'Join Room' }).click();
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'true');

      const body = `Posted after inline join ${Date.now()}`;
      await roomPage.sendMessage(body);
    });

    test('message deep links to unjoined rooms preserve the target after inline join', async ({
      page
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Inline Message Link ${Date.now()}`);
      const roomId = await createRoomViaAPI(page, `msg-link-${shortSuffix()}`);
      await joinRoomViaAPI(page, roomId);
      const targetBody = `Linked before join ${Date.now()}`;
      const target = await postMessageViaAPI(page, roomId, targetBody);
      expect(target).not.toBeNull();

      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);

      await page.goto(routes.messageLink(roomId, target!.id));
      await expect(page.getByRole('button', { name: 'Join Room' })).toBeVisible();
      await expect(page).toHaveURL(new RegExp(`${routes.messageLink(roomId, target!.id)}$`));

      await page.getByRole('button', { name: 'Join Room' }).click();
      await expect(page).toHaveURL(new RegExp(`${routes.room(roomId)}$`));
      await expect(page.getByText(targetBody)).toBeVisible();
    });

    test('a member denied message.post in #general cannot post there', async ({
      page,
      roomPage: _roomPage
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Muted Test ${Date.now()}`);
      const generalRoomId = await getRoomByName(page, 'general');

      // Roles only grant, so the deny goes on the member (still authed as
      // e2eadmin from usePrimaryServerViaAPI).
      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, generalRoomId, member.id!, 'message.post');

      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, generalRoomId);

      await page.goto(routes.room(generalRoomId));
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'false');
    });
  });

  // These tests keep the closed defaults of a new server (ADR-116): only the
  // seeded rooms are open to everyone, at room scope.
  test.describe('closed server defaults', () => {
    test.use({ openServerToEveryone: false });

    test('all server members can post to #general by default', async ({ page, roomPage }) => {
      // The seeded #general room is open to everyone at room scope.
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
    });

    test('a new room is hidden from members until everyone may list and join it', async ({
      page
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Closed Room ${Date.now()}`);
      const roomId = await createRoomViaAPI(page, `closed-${shortSuffix()}`);
      const member = await createSecondTestUser(page);

      const listMemberRoomIds = async () => {
        const data = await connectPost<{ rooms?: { room?: { id?: string } }[] }>(
          page,
          'chatto.api.v1.RoomDirectoryService/ListRooms',
          { scope: 'ROOM_DIRECTORY_SCOPE_CHANNELS' }
        );
        return data.rooms?.map((entry) => entry.room?.id) ?? [];
      };
      const tryJoin = () =>
        connectPostResponse(page, 'chatto.api.v1.RoomService/JoinRoom', { roomId });

      // The new room starts closed: the member can neither find nor join it.
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      expect(await listMemberRoomIds()).not.toContain(roomId);
      expect((await tryJoin()).ok()).toBe(false);

      // An admin allows room.list and room.join for everyone on this room.
      await logoutUser(page);
      await usePrimaryServerViaAPI(page);
      await grantRoomPermission(page, roomId, 'everyone', 'room.list');
      await grantRoomPermission(page, roomId, 'everyone', 'room.join');

      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      expect(await listMemberRoomIds()).toContain(roomId);
      await joinRoomViaAPI(page, roomId);
    });

    test('room settings warn about a closed room until it is opened', async ({ page }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Access Summary ${Date.now()}`);
      const roomId = await createRoomViaAPI(page, `summary-${shortSuffix()}`);
      await activatePrivilegedMode(page);

      const summary = page.getByTestId('access-summary');
      await page.goto(`${routes.serverAdminRooms}/${roomId}`);
      await expect(summary).toContainText('Nobody can find, join, or read this room yet');

      await grantRoomPermission(page, roomId, 'everyone', 'room.list');
      await grantRoomPermission(page, roomId, 'everyone', 'room.join');
      await page.reload();
      await expect(summary).toContainText('Everyone can join this room, but cannot read it.');

      await grantRoomPermission(page, roomId, 'everyone', 'message.read');
      await page.reload();
      await expect(summary).toContainText('Everyone can find and join this room.');
    });

    test('announcements room lets the owner post and members only read', async ({
      page,
      roomPage
    }) => {
      // Owner loads the primary server - this auto-creates #announcements
      const _owner = await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page, `Announcements Test ${Date.now()}`);
      const announcementsRoomId = await getRoomByName(page, 'announcements');

      // Owner should be able to post
      await page.goto(routes.room(announcementsRoomId));
      const ownerChatInput = page.getByTestId('message-input');
      await expect(ownerChatInput).toHaveAttribute('contenteditable', 'true');
      await roomPage.sendMessage('Important announcement from owner!');

      // Create regular member
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, announcementsRoomId);

      // Member can see the announcement but cannot post root messages
      await page.goto(routes.room(announcementsRoomId));
      await expect(page.getByText('Important announcement from owner!')).toBeVisible();
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'false');
    });

    test('admin can post root messages in announcements room', async ({ page }) => {
      // Owner loads the primary server - this auto-creates #announcements
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
      await expect(chatInput).toHaveAttribute('contenteditable', 'true');
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

      // Create second user and deny every thread-reply path to them at room level
      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, roomId, member.id!, 'message.post-in-thread');
      await denyUserRoomPermission(page, roomId, member.id!, 'message.post');
      await denyUserRoomPermission(page, roomId, member.id!, 'message.post-in-interactions');

      // The member joins the room
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

      // Create second user and deny every thread-reply path to them at room level
      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, roomId, member.id!, 'message.post-in-thread');
      await denyUserRoomPermission(page, roomId, member.id!, 'message.post');
      await denyUserRoomPermission(page, roomId, member.id!, 'message.post-in-interactions');

      // The member joins the room
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      // Posting in thread should be denied (no start_thread/post_in_thread split — all blocked)
      const replied = await replyToMessageViaAPI(page, roomId, rootMsg!.id, 'This should fail');
      expect(replied).toBeNull();
    });

    test('message.post includes explicit thread creation despite a narrower denial', async ({
      page
    }) => {
      // Admin creates server and room
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);

      // Create second user and deny message.post-in-thread to them at room level
      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, roomId, member.id!, 'message.post-in-thread');

      // The member joins the room
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinRoomViaAPI(page, roomId);

      await page.goto(routes.room(roomId));
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'true');
      await expect(page.getByRole('button', { name: 'Post as thread' })).toBeVisible();

      // Root posting should still work
      const posted = await postMessageViaAPI(page, roomId, 'Member can still post root');
      expect(posted).not.toBeNull();

      // Broad posting includes explicit thread creation.
      const thread = await postMessageViaAPI(page, roomId, 'Member can create a thread', {
        createThread: true
      });
      expect(thread).not.toBeNull();
    });
  });

  test.describe('message.post — Read-only rooms with open threads', () => {
    test('a room-scope deny of root posts can keep thread replies open', async ({ page }) => {
      // Admin creates server and room, posts a root message
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page);
      await joinRoomViaAPI(page, roomId);
      const rootMsg = await postMessageViaAPI(page, roomId, 'Root for post-denied test');
      expect(rootMsg).not.toBeNull();

      // Create second user and deny message.post to them at room level. It
      // includes thread replies, so allow those explicitly for everyone.
      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, roomId, member.id!, 'message.post');
      await grantRoomPermission(page, roomId, 'everyone', 'message.post-in-thread');

      // The member joins the room
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
      // Admin creates a room and denies `room.join` to a second user at room
      // scope. `room.list` stays at its default (allow), so the room is
      // still discoverable.
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page, `restricted-${Date.now()}`);
      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, roomId, member.id!, 'room.join');

      // The second user signs in. They haven't joined this room and never
      // will be able to via the directory, but they should be able to see
      // it.
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

    test('restricted room direct links render inline access denial instead of a modal', async ({
      page
    }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      const restrictedDescription = `Restricted preview ${Date.now()}`;
      const roomId = await createRoomViaAPI(
        page,
        `restrict-${shortSuffix()}`,
        restrictedDescription
      );
      await joinRoomViaAPI(page, roomId);

      const member = await createSecondTestUser(page);
      await denyUserRoomPermission(page, roomId, member.id!, 'room.join');
      await logoutUser(page);
      await loginUser(page, member.login, member.password);

      await page.goto(routes.room(roomId));
      await expect(page.getByText('You do not have permission to join this room.')).toBeVisible();
      await expect(page.getByRole('link', { name: 'Return to Server' })).toBeVisible();
      await expect(page.locator('dialog[open]')).toHaveCount(0);
      await expect(page.getByRole('button', { name: 'Join Room' })).toHaveCount(0);
      await expect(page.getByTestId('room-join-preview')).toHaveCount(0);
      await expect(page.getByText(restrictedDescription)).toHaveCount(0);
      await expect(page.getByLabel('Room members')).toHaveCount(0);
    });
  });
});
