import { test, expect } from './setup';
import type { Page } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  startSecondServer,
  stopSecondServer,
  createUserOnRemote,
  getPrimaryServerScopeOnRemote,
  joinDefaultRoomsOnRemote,
  sendTypingOnRemote,
  getRoomOnRemote,
  connectRemoteInstance
} from './fixtures/multiServer';
import { RoomPage } from './pages';
import type { ServerInfo } from './fixtures/server';
import { TIMEOUTS, POLLING_INTERVALS } from './constants';
import { waitForRoomReady } from './fixtures/realtimeSync';
import * as routes from './routes';

/**
 * Returns the remote server's base URL using 127.0.0.1 instead of localhost.
 * This gives the remote instance a distinct hostname for URL-based routing,
 * which would otherwise fail because both instances use "localhost".
 */
function remoteBaseURL(server: ServerInfo): string {
  return server.baseURL.replace('localhost', '127.0.0.1');
}

async function gotoRemoteRoom(page: Page, roomId: string): Promise<void> {
  const roomPath = routes.remote.room('127.0.0.1', roomId);
  await expect(async () => {
    await page.goto(roomPath);
    await page.waitForURL((url) => url.pathname === roomPath, { timeout: TIMEOUTS.UI_STANDARD });
    await waitForRoomReady(page, 'general', { timeout: TIMEOUTS.UI_STANDARD });
  }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
}

test.describe('Multi-Instance Identity', () => {
  let remoteServer: ServerInfo;

  test.beforeEach(async ({}, testInfo) => {
    remoteServer = await startSecondServer(testInfo);
  });

  test.afterEach(async ({}, testInfo) => {
    if (remoteServer) {
      await stopSecondServer(remoteServer, testInfo);
    }
  });

  test('user can edit own message on remote instance', async ({ page, chatPage }) => {
    // Home instance: log in so the SPA works
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Remote instance: owner loads the server, browser user connects via API
    const baseURL = remoteBaseURL(remoteServer);
    const remoteOwner = await createUserOnRemote(baseURL, 'remoteowner1', 'password123');
    const spaceId = await getPrimaryServerScopeOnRemote(
      baseURL,
      remoteOwner.token,
      'Remote Edit Test'
    );
    const remoteBrowser = await createUserOnRemote(baseURL, 'remotebrowser1', 'password123');
    await joinDefaultRoomsOnRemote(baseURL, remoteBrowser.token);
    const roomId = await getRoomOnRemote(baseURL, remoteOwner.token, 'general');

    // Connect remote instance and navigate directly to the room
    await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteBrowser.userId);
    await gotoRemoteRoom(page, roomId);

    // Send a message on the remote instance
    const roomPage = new RoomPage(page);
    await roomPage.waitForInputEditable();
    const testMessage = `Remote msg ${Date.now()}`;
    const msg = await roomPage.sendMessage(testMessage);

    // Hover over the message — the edit button should be visible (isAuthor = true).
    // This verifies getCurrentUser() returns the remote instance user ID, not the
    // home instance user ID, because isAuthor compares currentUser.user.id to actorId.
    await msg.revealHoverToolbar();
    await expect(msg.hoverToolbar.getByLabel('Edit message')).toBeVisible({
      timeout: TIMEOUTS.UI_FAST
    });
  });

  test('recreates scoped resources when a signed-out remote server reconnects', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const baseURL = remoteBaseURL(remoteServer);
    const remoteUser = await createUserOnRemote(baseURL, 'remotereconnect', 'password123');
    await getPrimaryServerScopeOnRemote(baseURL, remoteUser.token, 'Remote Reconnect Test');
    await joinDefaultRoomsOnRemote(baseURL, remoteUser.token);
    const roomId = await getRoomOnRemote(baseURL, remoteUser.token, 'general');

    await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);
    await gotoRemoteRoom(page, roomId);

    // Sign out of only the remote server. Reauthentication replaces the same
    // server ID's token, connection, and store, so the route subtree must use
    // the replacement resources even though the server ID is unchanged.
    await page.getByTitle('Sign out').click();
    await expect(page.getByRole('dialog')).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await page.getByRole('button', { name: 'Current Server' }).click();
    await expect(page).toHaveURL(/\/chat\/-/);
    await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);

    // Stay inside the running SPA so a document reload cannot mask a missing
    // same-ID resource remount.
    const generalRoom = chatPage.roomList.getByRole('link', { name: '# general' });
    await expect(generalRoom).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await generalRoom.click();
    await page.waitForURL((url) => url.pathname === routes.remote.room('127.0.0.1', roomId));
    await waitForRoomReady(page, 'general', { timeout: TIMEOUTS.UI_STANDARD });

    const roomPage = new RoomPage(page);
    await roomPage.waitForInputEditable();
    await roomPage.sendMessage(`Message after reconnect ${Date.now()}`);
  });

  test('user does not see own typing indicator on remote instance', async ({ page, chatPage }) => {
    // Home instance: log in so the SPA works
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Remote instance: owner loads the server, browser user connects via API
    const baseURL = remoteBaseURL(remoteServer);
    const remoteOwner = await createUserOnRemote(baseURL, 'remoteowner2', 'password123');
    const spaceId = await getPrimaryServerScopeOnRemote(
      baseURL,
      remoteOwner.token,
      'Remote Typing Test'
    );
    const remoteBrowser = await createUserOnRemote(baseURL, 'remotebrowser2', 'password123');
    await joinDefaultRoomsOnRemote(baseURL, remoteBrowser.token);
    const roomId = await getRoomOnRemote(baseURL, remoteOwner.token, 'general');

    // Connect remote instance and navigate directly to the room
    await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteBrowser.userId);
    await gotoRemoteRoom(page, roomId);

    const roomPage = new RoomPage(page);
    await roomPage.waitForInputEditable();

    // Start typing (simulates keystrokes to trigger typing indicator mutation)
    await roomPage.messageInput.pressSequentially('Hello remote', { delay: 50 });

    // Wait and assert typing dots do NOT appear for own typing.
    // The backend filters own typing events, and the frontend uses the per-instance
    // user ID for defense-in-depth filtering.
    await expect(async () => {
      await expect(page.locator('.typing-dots')).not.toBeVisible();
    }).toPass({
      timeout: TIMEOUTS.POLLING_EXTENDED,
      intervals: POLLING_INTERVALS
    });
  });

  test('user sees other user typing on remote instance', async ({ page, chatPage }) => {
    // Home instance: log in so the SPA works
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Remote instance: owner loads the server, viewer connects
    const baseURL = remoteBaseURL(remoteServer);
    const remoteOwner = await createUserOnRemote(baseURL, 'remoteowner3', 'password123');
    const spaceId = await getPrimaryServerScopeOnRemote(
      baseURL,
      remoteOwner.token,
      'Remote Typing Visible'
    );
    const remoteViewer = await createUserOnRemote(baseURL, 'remoteviewer3', 'password123');
    await joinDefaultRoomsOnRemote(baseURL, remoteViewer.token);
    const roomId = await getRoomOnRemote(baseURL, remoteOwner.token, 'general');

    // Connect remote instance with the viewer user and navigate directly
    await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteViewer.userId);
    await gotoRemoteRoom(page, roomId);

    const roomPage = new RoomPage(page);
    await roomPage.waitForInputEditable();

    // Owner sends typing indicator via API
    await sendTypingOnRemote(baseURL, remoteOwner.token, roomId);

    // Viewer should see the typing indicator
    await expect(page.locator('.typing-dots')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  });
});
