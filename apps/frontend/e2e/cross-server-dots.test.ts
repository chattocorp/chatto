import { expect, type Locator, type Page } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import {
  startSecondServer,
  stopSecondServer,
  createUserOnRemote,
  getPrimaryServerScopeOnRemote,
  joinDefaultRoomsOnRemote,
  getRoomOnRemote,
  postMessageOnRemote,
  postThreadReplyOnRemote,
  startDMOnRemote,
  connectRemoteInstance
} from './fixtures/multiServer';
import {
  getRoomIdByNameViaConnect,
  postMessageViaConnect,
  postThreadReplyViaConnect
} from './fixtures/connectHelpers';
import type { ServerInfo } from './fixtures/server';
import { TIMEOUTS } from './constants';
import * as routes from './routes';
import { waitForRoomReady } from './fixtures/realtimeSync';

/**
 * Returns the remote server's base URL using 127.0.0.1 instead of localhost so
 * the SPA can resolve it as a distinct instance hostname.
 */
function remoteBaseURL(server: ServerInfo): string {
  return server.baseURL.replace('localhost', '127.0.0.1');
}

async function openSwitcher(page: Page): Promise<Locator> {
  const isMac = process.platform === 'darwin';
  const key = isMac ? 'Meta+k' : 'Control+k';
  const dialog = page.locator('dialog.quick-switcher');

  await expect(async () => {
    await page.keyboard.press(key);
    await expect(dialog).toBeVisible({ timeout: 500 });
  }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [200, 500, 1000] });

  return dialog;
}

function switcherInput(dialog: Locator): Locator {
  return dialog.getByPlaceholder('Go to server, room, or conversation...');
}

function switcherResults(dialog: Locator): Locator {
  return dialog.locator('button.sidebar-item');
}

async function navigateWithinClient(page: Page, href: string): Promise<void> {
  await page.evaluate((target) => {
    const link = document.createElement('a');
    link.href = target;
    document.body.append(link);
    link.click();
    link.remove();
  }, href);
  await page.waitForURL((url) => url.pathname === href);
}

/**
 * Cross-instance dot indicator coverage.
 *
 * Most dot-rendering code is instance-agnostic (one render path keyed by
 * `serverId`), but a few timing windows and aggregation paths only manifest
 * for remote instances on cold loads. These tests cover those windows.
 */
test.describe('Cross-instance dots', () => {
  let remoteServer: ServerInfo;

  test.beforeEach(async ({}, testInfo) => {
    remoteServer = await startSecondServer(testInfo);
  });

  test.afterEach(async ({}, testInfo) => {
    if (remoteServer) {
      await stopSecondServer(remoteServer, testInfo);
    }
  });

  test('@mention on an inactive remote server appears after background catch-up', async ({
    page,
    chatPage
  }) => {
    test.slow();
    // Home: log in so the SPA boots.
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Remote: owner loads the server, viewer connects, mentioner joins.
    const baseURL = remoteBaseURL(remoteServer);
    const ts = Date.now();
    const viewerLogin = `xviewer${ts}`;
    const owner = await createUserOnRemote(baseURL, `xowner${ts}`, 'password123');
    const spaceId = await getPrimaryServerScopeOnRemote(
      baseURL,
      owner.token,
      'Cross Instance Mention'
    );
    const viewer = await createUserOnRemote(baseURL, viewerLogin, 'password123');
    await joinDefaultRoomsOnRemote(baseURL, viewer.token);
    const mentioner = await createUserOnRemote(baseURL, `xmentioner${ts}`, 'password123');
    await joinDefaultRoomsOnRemote(baseURL, mentioner.token);
    const generalRoomId = await getRoomOnRemote(baseURL, owner.token, 'general');

    // Connect the remote instance as `viewer` and stay on /chat (away from the
    // remote server). This is the cold-load timing window where the bus has to
    // be ready and consumers have to attach reactively.
    await connectRemoteInstance(page, { ...remoteServer, baseURL }, viewer.userId);
    await navigateWithinClient(page, routes.chat);
    await page.waitForLoadState('networkidle');

    // Sanity: no dot on the remote server icon yet. Issue #330: home and
    // remote share the bootstrap server name "E2E Test Server", so
    // disambiguate the remote icon by the host segment in its href —
    // home links use "/chat/-" while remote links use "/chat/<host>".
    const remoteHostSegment = new URL(baseURL).hostname;
    const remoteSpaceWrapper = page.locator('.server-gutter .server-icon-wrapper').filter({
      has: page.locator(`a[data-testid="server-icon"][href*="/chat/${remoteHostSegment}"]`)
    });
    const remoteSpaceBadge = remoteSpaceWrapper.getByTestId('server-notification-badge');
    await expect(remoteSpaceBadge).not.toBeVisible();

    // Mentioner posts an @mention of the viewer in the remote server. No reload.
    await postMessageOnRemote(
      baseURL,
      mentioner.token,
      generalRoomId,
      `hey @${viewerLogin} ping ${ts}`
    );

    // The inactive server has no persistent socket. Its serialized resume poll
    // should still update the retained projection without a reload.
    await expect(remoteSpaceBadge).toBeVisible({ timeout: TIMEOUTS.BACKGROUND_SERVER_POLL });
    await expect(remoteSpaceBadge).toHaveText('1');
  });

  // "DM on a remote instance lights up the DM icon" was removed with the
  // cross-instance DM icon (#330 phase 3). Cross-server DM aggregation will
  // be re-tested when that view is reintroduced.

  test('clicking a remote thread notification badge remounts the containing room timeline', async ({
    page,
    chatPage,
    roomPage
  }) => {
    test.slow();
    // Home: mount a normal room first. This is the stale-state setup: the
    // currently rendered Room subtree belongs to the home server before the
    // remote notification badge routes to another server.
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const homeGeneralRoomId = await getRoomIdByNameViaConnect(page, 'general');
    const homeBody = `Home room before remote dot ${Date.now()}`;
    await roomPage.sendMessage(homeBody);

    // Remote: viewer will receive a mention on a thread reply.
    const baseURL = remoteBaseURL(remoteServer);
    const suffix = Date.now().toString(36);
    const viewerLogin = `tv${suffix}`;
    const owner = await createUserOnRemote(baseURL, `to${suffix}`, 'password123');
    const viewer = await createUserOnRemote(baseURL, viewerLogin, 'password123');
    const mentioner = await createUserOnRemote(baseURL, `tm${suffix}`, 'password123');
    const remoteGeneralRoomId = await getRoomOnRemote(baseURL, owner.token, 'general');
    const remoteRootBody = `Remote thread root ${suffix}`;
    const remoteRootEventId = await postMessageOnRemote(
      baseURL,
      owner.token,
      remoteGeneralRoomId,
      remoteRootBody
    );

    await connectRemoteInstance(page, { ...remoteServer, baseURL }, viewer.userId);
    await navigateWithinClient(page, routes.room(homeGeneralRoomId));
    await waitForRoomReady(page, 'general');
    await expect(page.getByText(homeBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    const remoteHostSegment = new URL(baseURL).hostname;
    const remoteSpaceWrapper = page.locator('.server-gutter .server-icon-wrapper').filter({
      has: page.locator(`a[data-testid="server-icon"][href*="/chat/${remoteHostSegment}"]`)
    });
    const remoteSpaceBadge = remoteSpaceWrapper.getByTestId('server-notification-badge');
    await expect(remoteSpaceBadge).not.toBeVisible();

    const remoteReplyBody = `@${viewerLogin} remote thread reply ${suffix}`;
    await postThreadReplyOnRemote(
      baseURL,
      mentioner.token,
      remoteGeneralRoomId,
      remoteReplyBody,
      remoteRootEventId
    );

    await expect(remoteSpaceBadge).toBeVisible({ timeout: TIMEOUTS.BACKGROUND_SERVER_POLL });
    await expect(remoteSpaceBadge).toHaveText('1');
    await remoteSpaceBadge.click();

    await page.waitForURL(
      (url) =>
        url.pathname === `/chat/${remoteHostSegment}/${remoteGeneralRoomId}/${remoteRootEventId}`
    );
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
    await roomPage.expectThreadPaneVisible();
    await roomPage.expectTextInThreadPane(remoteReplyBody);

    const mainRoomTimeline = page.locator('[data-testid="messages-container"]').first();
    await expect(mainRoomTimeline.locator('[role="article"]')).not.toHaveCount(0);
    await expect(mainRoomTimeline.getByText(remoteRootBody)).toBeVisible();
  });

  test('quick switching to a remote DM keeps that server room timelines populated', async ({
    page,
    chatPage,
    roomPage
  }) => {
    // Home: mount a room first so the quick switcher has to replace an
    // already-rendered room subtree when it jumps to the remote instance.
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const homeGeneralRoomId = await getRoomIdByNameViaConnect(page, 'general');
    const homeBody = `Home room before remote DM switch ${Date.now()}`;
    await roomPage.sendMessage(homeBody);

    // Remote: create a DM for the connected viewer, plus a normal room event
    // that must still be visible after navigating from the remote DM to #general.
    const baseURL = remoteBaseURL(remoteServer);
    const suffix = Date.now().toString(36);
    const viewer = await createUserOnRemote(baseURL, `qv${suffix}`, 'password123');
    const senderLogin = `qs${suffix}`;
    const sender = await createUserOnRemote(baseURL, senderLogin, 'password123');
    const remoteDmBody = `Remote DM before room navigation ${suffix}`;
    const remoteDmRoomId = await startDMOnRemote(
      baseURL,
      sender.token,
      viewer.userId,
      remoteDmBody
    );
    const remoteGeneralRoomId = await getRoomOnRemote(baseURL, viewer.token, 'general');
    const remoteRoomBody = `Remote general after DM switch ${suffix}`;
    await postMessageOnRemote(baseURL, sender.token, remoteGeneralRoomId, remoteRoomBody);

    await connectRemoteInstance(page, { ...remoteServer, baseURL }, viewer.userId);
    await navigateWithinClient(page, routes.room(homeGeneralRoomId));
    await waitForRoomReady(page, 'general');
    await expect(page.getByText(homeBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    const remoteHostSegment = new URL(baseURL).hostname;
    const dialog = await openSwitcher(page);
    const input = switcherInput(dialog);
    await expect(switcherResults(dialog).first()).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await input.fill(senderLogin);

    const dmResult = switcherResults(dialog).filter({ hasText: senderLogin }).first();
    await expect(dmResult).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await dmResult.click();

    await page.waitForURL((url) => url.pathname === `/chat/${remoteHostSegment}/${remoteDmRoomId}`);
    await expect(page.getByText(remoteDmBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    const remoteGeneralLink = chatPage.roomList.getByRole('link', { name: '# general' });
    await expect(remoteGeneralLink).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await remoteGeneralLink.click();
    await page.waitForURL(
      (url) => url.pathname === `/chat/${remoteHostSegment}/${remoteGeneralRoomId}`
    );
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();

    const mainRoomTimeline = page.locator('[data-testid="messages-container"]').first();
    await expect(mainRoomTimeline.locator('[role="article"]')).not.toHaveCount(0);
    await expect(mainRoomTimeline.getByText(remoteRoomBody)).toBeVisible();
  });

  test('mention on a thread message: clicking the server badge opens the thread', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    // Home: User A loads the server, posts a root message, then leaves the room.
    const userA = await createAndLoginTestUser(page);
    await chatPage.goto();

    await chatPage.enterRoom('general');
    const generalRoomId = await getRoomIdByNameViaConnect(page, 'general');
    const rootBody = `Thread root ${Date.now()}`;
    const rootEventId = await postMessageViaConnect(page, generalRoomId, rootBody);

    // Move A away from the room so the notification badge can show on the server.
    await chatPage.enterRoom('announcements');

    // User B joins, then posts a thread reply that @-mentions User A.
    await withServerUser(browser!, serverURL, async ({ page: pageB }) => {
      await postThreadReplyViaConnect(
        pageB,
        generalRoomId,
        `@${userA.login} look at this`,
        rootEventId
      );

      // User A: notification badge appears on the server icon.
      const spaceIcon = page.locator('.server-gutter [data-testid="server-icon"]').first();
      const spaceBadge = spaceIcon.locator('..').getByTestId('server-notification-badge');
      await expect(spaceBadge).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      // The reply both mentions User A and replies to their message, so the
      // count badge reflects both pending notification records.
      await expect(spaceBadge).toHaveText('2');

      // Click the badge. The mention is on a thread message, so clicking should
      // land in #general with the thread pane open and the reply highlighted.
      await spaceBadge.click();

      // Should land on the thread URL (/chat/-/{spaceId}/{roomId}/{threadId}).
      await page.waitForURL(routes.patterns.anyThread);
      await expect(page.getByRole('heading', { name: '# general' })).toBeVisible();
      await roomPage.expectThreadPaneVisible();
    });
  });
});
