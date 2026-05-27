import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  startSecondServer,
  stopSecondServer,
  createUserOnRemote,
  createSpaceOnRemote,
  joinSpaceOnRemote,
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

    // Remote instance: owner creates a space, browser user joins via API
    const baseURL = remoteBaseURL(remoteServer);
    const remoteOwner = await createUserOnRemote(baseURL, 'remoteowner1', 'password123');
    const spaceId = await createSpaceOnRemote(baseURL, remoteOwner.token, 'Remote Edit Test');
    const remoteBrowser = await createUserOnRemote(baseURL, 'remotebrowser1', 'password123');
    await joinSpaceOnRemote(baseURL, remoteBrowser.token);
    const roomId = await getRoomOnRemote(baseURL, remoteOwner.token, 'general');

    // Connect remote instance and navigate directly to the room
    await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteBrowser.userId);
    await page.goto(routes.remote.room('127.0.0.1', roomId));
    await waitForRoomReady(page, 'general');

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

  test('user does not see own typing indicator on remote instance', async ({ page, chatPage }) => {
    // Home instance: log in so the SPA works
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Remote instance: owner creates a space, browser user joins via API
    const baseURL = remoteBaseURL(remoteServer);
    const remoteOwner = await createUserOnRemote(baseURL, 'remoteowner2', 'password123');
    const spaceId = await createSpaceOnRemote(baseURL, remoteOwner.token, 'Remote Typing Test');
    const remoteBrowser = await createUserOnRemote(baseURL, 'remotebrowser2', 'password123');
    await joinSpaceOnRemote(baseURL, remoteBrowser.token);
    const roomId = await getRoomOnRemote(baseURL, remoteOwner.token, 'general');

    // Connect remote instance and navigate directly to the room
    await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteBrowser.userId);
    await page.goto(routes.remote.room('127.0.0.1', roomId));
    await waitForRoomReady(page, 'general');

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

    // Remote instance: owner creates the space, viewer joins
    const baseURL = remoteBaseURL(remoteServer);
    const remoteOwner = await createUserOnRemote(baseURL, 'remoteowner3', 'password123');
    const spaceId = await createSpaceOnRemote(baseURL, remoteOwner.token, 'Remote Typing Visible');
    const remoteViewer = await createUserOnRemote(baseURL, 'remoteviewer3', 'password123');
    await joinSpaceOnRemote(baseURL, remoteViewer.token);
    const roomId = await getRoomOnRemote(baseURL, remoteOwner.token, 'general');

    // Connect remote instance with the viewer user and navigate directly
    await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteViewer.userId);
    await page.goto(routes.remote.room('127.0.0.1', roomId));
    await waitForRoomReady(page, 'general');

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
