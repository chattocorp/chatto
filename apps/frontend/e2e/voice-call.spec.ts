/**
 * E2E tests for voice/video call integration.
 *
 * These tests configure the test server with LiveKit credentials via the
 * serverOptions fixture. Token generation is pure JWT signing (no real
 * LiveKit server needed). Actual WebRTC connections cannot be tested in
 * CI — these tests verify the signaling layer, UI visibility, and
 * ConnectRPC calls.
 *
 * Camera/video tests require an actual LiveKit connection (participant mode)
 * which is not available in CI. Camera toggle, video thumbnails, and device
 * menu camera section can only be tested manually with `mise dev`.
 */

import type { Page } from '@playwright/test';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import {
  connectPost,
  connectPostResponse,
  getIdsFromUrlViaConnect,
  getRoomIdByNameViaConnect,
  joinRoomViaConnect
} from './fixtures/connectHelpers';
import { DMPage } from './pages/DMPage';
import { TIMEOUTS } from './constants';

// Configure the test server with LiveKit credentials via the server fixture.
// This scopes the env vars to the spawned server process without polluting
// the worker's process.env (which would leak to other test files).
test.use({
  serverOptions: {
    env: {
      CHATTO_LIVEKIT_ENABLED: 'true',
      CHATTO_LIVEKIT_URL: 'ws://localhost:7880',
      CHATTO_LIVEKIT_API_KEY: 'devkey',
      CHATTO_LIVEKIT_API_SECRET: 'secret'
    }
  }
});

interface JoinCallResponse {
  joined?: boolean;
}

interface GetCallTokenResponse {
  token?: string;
  e2eeKey?: string;
  callId?: string;
}

interface ListActiveCallRoomsResponse {
  roomIds?: string[];
}

interface ListCallParticipantsResponse {
  participants?: Array<{
    user?: { user?: { id?: string; displayName?: string; login?: string } };
  }>;
}

interface ServerStateResponse {
  livekitUrl?: string;
}

async function joinCallViaConnect(page: Page, roomId: string): Promise<boolean> {
  const data = await connectPost<JoinCallResponse>(
    page,
    'chatto.api.v1.VoiceCallService/JoinCall',
    { roomId }
  );
  return data.joined ?? false;
}

async function getCallTokenViaConnect(page: Page, roomId: string): Promise<GetCallTokenResponse> {
  return connectPost<GetCallTokenResponse>(page, 'chatto.api.v1.VoiceCallService/GetCallToken', {
    roomId
  });
}

async function listActiveCallRoomIdsViaConnect(page: Page): Promise<string[]> {
  const data = await connectPost<ListActiveCallRoomsResponse>(
    page,
    'chatto.api.v1.VoiceCallService/ListActiveCallRooms'
  );
  return data.roomIds ?? [];
}

async function listCallParticipantsViaConnect(
  page: Page,
  roomId: string
): Promise<ListCallParticipantsResponse['participants']> {
  const data = await connectPost<ListCallParticipantsResponse>(
    page,
    'chatto.api.v1.VoiceCallService/ListCallParticipants',
    { roomId }
  );
  return data.participants ?? [];
}

async function openCallTab(page: Page) {
  await page.locator('[data-testid="room-sidebar-toggle"]:visible').getByLabel('Show call').click();
}

test.describe('Voice calls', () => {
  test('call tab appears in room sidebar toggle', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const callTab = page
      .locator('[data-testid="room-sidebar-toggle"]:visible')
      .getByLabel('Show call');
    await expect(callTab).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('call tab appears in DM rooms', async ({ page, browser, serverURL }) => {
    // Create two users
    await createAndLoginTestUser(page);

    await withServerUser(browser!, serverURL, async ({ user: userB }) => {
      // User A starts a DM with User B
      const dmPage = new DMPage(page);
      await dmPage.startConversation(userB.login);

      // Call tab should be visible in the DM room sidebar toggle
      const callTab = page
        .locator('[data-testid="room-sidebar-toggle"]:visible')
        .getByLabel('Show call');
      await expect(callTab).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    });
  });

  test('call token RPC returns a valid JWT', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');

    await expect(joinCallViaConnect(page, roomId)).resolves.toBe(true);

    const token = await getCallTokenViaConnect(page, roomId);
    expect(token.token).toBeTruthy();
    expect(token.e2eeKey).toBeTruthy();
    expect(token.callId).toBeTruthy();
    // JWT tokens have 3 base64url parts separated by dots
    expect(token.token!.split('.')).toHaveLength(3);
  });

  test('call token RPC requires room membership', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A loads the server and room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createRoom(`voice-private-${Date.now()}`);
    const { roomId } = await getIdsFromUrlViaConnect(page);
    await expect(joinCallViaConnect(page, roomId)).resolves.toBe(true);

    // User B is on the server but not a member of the newly created room.
    await withServerUser(browser!, serverURL, async ({ page: page2 }) => {
      const response = await connectPostResponse(
        page2,
        'chatto.api.v1.VoiceCallService/GetCallToken',
        { roomId }
      );
      expect(response.ok()).toBe(false);
      expect(response.status()).toBe(403);
    });
  });

  test('active call room IDs return empty when no calls active', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await expect(listActiveCallRoomIdsViaConnect(page)).resolves.toEqual([]);
  });

  test('call icon appears and disappears via real-time events', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A loads the server and enters a room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceId = await chatPage.getServerScopeId();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');

    // The call icon should NOT be visible initially
    const callIcon = chatPage.roomList.getByTestId('room-call-icon');
    await expect(callIcon).not.toBeVisible();

    // User B joins the same server and room
    await withServerUser(browser!, serverURL, async ({ page: page2, user: userB }) => {
      await joinRoomViaConnect(page2, roomId);

      // Simulate User B joining a voice call via test webhook endpoint
      // (bypasses LiveKit HMAC — calls core.HandleCallParticipantJoined directly)
      await page.request.post('/webhooks/test/call-join', {
        data: {
          spaceId,
          roomId,
          userId: userB.id,
          displayName: userB.displayName,
          login: userB.login
        }
      });

      // User A should see the active-call icon appear
      await expect(callIcon).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Simulate User B leaving the voice call
      await page.request.post('/webhooks/test/call-leave', {
        data: {
          spaceId,
          roomId,
          userId: userB.id
        }
      });

      // User A should see the active-call icon disappear
      // (handleLeave re-queries active call rooms, which returns [] since KV is now empty)
      await expect(callIcon).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    });
  });

  test('call participants RPC returns participants after webhook join', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceId = await chatPage.getServerScopeId();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');

    // Initially empty
    await expect(listCallParticipantsViaConnect(page, roomId)).resolves.toEqual([]);

    // Create User B and have them join
    await withServerUser(browser!, serverURL, async ({ page: page2, user: userB }) => {
      await joinRoomViaConnect(page2, roomId);

      // Simulate join via webhook test endpoint
      await page.request.post('/webhooks/test/call-join', {
        data: {
          spaceId,
          roomId,
          userId: userB.id,
          displayName: userB.displayName,
          login: userB.login
        }
      });

      // Query participants — should now include User B
      const afterParticipants = await listCallParticipantsViaConnect(page, roomId);
      expect(afterParticipants).toHaveLength(1);
      expect(afterParticipants[0].user?.user?.id).toBe(userB.id);
      expect(afterParticipants[0].user?.user?.login).toBe(userB.login);

      // Simulate leave
      await page.request.post('/webhooks/test/call-leave', {
        data: { spaceId, roomId, userId: userB.id }
      });

      // Query again — should be empty
      await expect(listCallParticipantsViaConnect(page, roomId)).resolves.toEqual([]);
    });
  });

  test('observer panel appears when another user joins a call', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A loads the server and enters a room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceId = await chatPage.getServerScopeId();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');

    // Observer panel should NOT be visible initially
    await expect(page.getByTestId('call-observer-panel')).not.toBeVisible();

    // User B joins the same server and room
    await withServerUser(browser!, serverURL, async ({ page: page2, user: userB }) => {
      await joinRoomViaConnect(page2, roomId);

      // Simulate User B joining a voice call via test webhook endpoint
      await page.request.post('/webhooks/test/call-join', {
        data: {
          spaceId,
          roomId,
          userId: userB.id,
          displayName: userB.displayName,
          login: userB.login
        }
      });

      await openCallTab(page);

      // User A should see the call sidebar observer state with a Join button
      const observerPanel = page.getByTestId('call-observer-panel');
      await expect(observerPanel).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(page.getByTestId('call-join-button')).toBeVisible();

      // User A should see User B's display name in the participant list
      await expect(observerPanel.getByTitle(userB.displayName)).toBeVisible();

      // Simulate User B leaving — the open call tab falls back to its idle start-call state
      await page.request.post('/webhooks/test/call-leave', {
        data: { spaceId, roomId, userId: userB.id }
      });

      await expect(observerPanel.getByTitle(userB.displayName)).not.toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      await expect(page.getByTestId('call-join-button')).toHaveText('Start call');
    });
  });

  test('observer panel updates when participants join and leave', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A loads the server and enters a room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceId = await chatPage.getServerScopeId();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');

    await withServerUser(browser!, serverURL, async ({ page: page2, user: userB }) => {
      await joinRoomViaConnect(page2, roomId);

      await withServerUser(browser!, serverURL, async ({ page: page3, user: userC }) => {
        await joinRoomViaConnect(page3, roomId);

        // User B joins the call
        await page.request.post('/webhooks/test/call-join', {
          data: {
            spaceId,
            roomId,
            userId: userB.id,
            displayName: userB.displayName,
            login: userB.login
          }
        });

        await openCallTab(page);
        const observerPanel = page.getByTestId('call-observer-panel');

        await expect(observerPanel).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
        await expect(observerPanel.getByTitle(userB.displayName)).toBeVisible();

        // User C joins the call
        await page.request.post('/webhooks/test/call-join', {
          data: {
            spaceId,
            roomId,
            userId: userC.id,
            displayName: userC.displayName,
            login: userC.login
          }
        });

        // Both participants should be visible
        await expect(observerPanel.getByTitle(userC.displayName)).toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });
        await expect(observerPanel.getByTitle(userB.displayName)).toBeVisible();

        // User B leaves — User C should still be visible, panel still showing
        await page.request.post('/webhooks/test/call-leave', {
          data: { spaceId, roomId, userId: userB.id }
        });

        await expect(observerPanel.getByTitle(userB.displayName)).not.toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });
        await expect(observerPanel.getByTitle(userC.displayName)).toBeVisible();
        await expect(observerPanel).toBeVisible();

        // User C leaves — the open call tab falls back to its idle start-call state
        await page.request.post('/webhooks/test/call-leave', {
          data: { spaceId, roomId, userId: userC.id }
        });

        await expect(observerPanel.getByTitle(userC.displayName)).not.toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });
        await expect(page.getByTestId('call-join-button')).toHaveText('Start call');
      });
    });
  });

  test('active-call icon appears in room list sidebar during active call', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User A loads the server and enters a room
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceId = await chatPage.getServerScopeId();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');

    // No active-call icon in sidebar initially
    const roomList = chatPage.roomList;
    const callIcon = roomList.getByTestId('room-call-icon');
    await expect(callIcon).not.toBeVisible();

    await withServerUser(browser!, serverURL, async ({ page: page2, user: userB }) => {
      await joinRoomViaConnect(page2, roomId);

      // Simulate User B joining a voice call
      await page.request.post('/webhooks/test/call-join', {
        data: {
          spaceId,
          roomId,
          userId: userB.id,
          displayName: userB.displayName,
          login: userB.login
        }
      });

      // Active-call icon with phone pulse twin should appear in sidebar
      await expect(callIcon).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(callIcon.locator('.uil--phone').first()).toBeVisible();
      await expect(callIcon.getByTestId('active-call-pulse-icon')).toBeVisible();

      // Simulate User B leaving — active-call icon should disappear
      await page.request.post('/webhooks/test/call-leave', {
        data: { spaceId, roomId, userId: userB.id }
      });

      await expect(callIcon).not.toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    });
  });

  test('livekitUrl is exposed in instance info', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const data = await connectPost<ServerStateResponse>(
      page,
      'chatto.api.v1.ServerStateService/GetServerState'
    );
    expect(data.livekitUrl).toBe('ws://localhost:7880');
  });
});
