import { expect } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  loginAsAdmin,
  denyUserPermission,
  clearUserPermissionOverride
} from './fixtures/testUser';
import { DMPage } from './pages/DMPage';
import { RoomPage } from './pages/RoomPage';
import { postMessageViaAPI } from './fixtures/graphqlHelpers';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

/**
 * Direct Messages — post-#330 phase 3 shape. DMs are rooms on the Server,
 * appear in the primary-space sidebar alongside channels, and use the same
 * `/chat/{instanceSegment}/{roomId}` URL shape. The dedicated /chat/dm
 * inbox is gone for the time being.
 *
 * These tests pin the regressions we just fixed (silent post + reload-redirect)
 * and the basic sidebar integration so future work doesn't quietly undo them.
 */

test.describe('Direct Messages (room-shaped)', () => {
  test('post a DM message, reload, and stay on the conversation', async ({
    page,
    browser,
    serverURL
  }) => {
    // Two users on the same server.
    const userA = await createAndLoginTestUser(page);

    const context2 = await browser.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();
    try {
      await createAndLoginTestUser(page2);

      // User B starts a DM with User A and seeds a message so the DM is in
      // User A's merged sidebar (the active DM-room list filters empty rooms).
      // The conversation ID is deterministic across the two users — pull it
      // from B's URL once the room loads.
      const dmPageB = new DMPage(page2);
      const roomB = await dmPageB.startConversation(userA.login);
      await roomB.sendMessage('seed from B');
      const conversationId = page2.url().split('/').pop()!;

      // User A navigates to the DM via the channel-shaped URL.
      await page.goto(routes.room(conversationId));
      await page.waitForURL(routes.patterns.anyRoom);

      // Bug #1 (the silent post): the ServerEventProvider must subscribe to
      // DM events too, so MessagePostedEvent reaches RoomEventsPane
      // and the new message renders without a reload.
      const roomA = new RoomPage(page);
      const postedBody = `dm round-trip ${Date.now()}`;
      await roomA.sendMessage(postedBody);
      await expect(page.getByText(postedBody)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Bug #2 (the reload-redirect): on reload the rooms store is briefly
      // unloaded — the layout must wait for it before resolving spaceId,
      // otherwise Room.svelte's not-found redirect bounces the user out.
      await page.reload();
      await page.waitForURL(routes.patterns.anyRoom);
      await expect(page.getByText(postedBody)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    } finally {
      await context2.close();
    }
  });

  test('a DM with messages renders in the primary-space sidebar and links to /chat/{seg}/{id}', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    const context2 = await browser.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();
    try {
      const userB = await createAndLoginTestUser(page2);

      // User B → User A: start DM and post so the DM survives the active
      // DM-room empty-room filter.
      const dmPageB = new DMPage(page2);
      const roomB = await dmPageB.startConversation(userA.login);
      await roomB.sendMessage('seed');

      // User A: land on chat root and look at the merged sidebar.
      await page.goto(routes.chat);
      await page.waitForURL(routes.chat);

      // The "Direct Messages" group header should be present, and User B's
      // displayName should be a sidebar item underneath it.
      await expect(
        page.getByRole('button', { name: /direct messages/i })
      ).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      const dmLink = page
        .locator('nav a.sidebar-item')
        .filter({ has: page.getByText(userB.displayName, { exact: true }) });
      await expect(dmLink).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Click it: the URL must be the channel-shaped /chat/-/{roomId}, not
      // the legacy /chat/dm/... path.
      await dmLink.click();
      await page.waitForURL(routes.patterns.anyRoom);
      expect(page.url()).not.toContain('/chat/dm/');
    } finally {
      await context2.close();
    }
  });

  test('an incoming DM bumps the conversation to the top and shows an unread dot', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    const ctxB = await browser.newContext({ baseURL: serverURL });
    const ctxC = await browser.newContext({ baseURL: serverURL });
    const pageB = await ctxB.newPage();
    const pageC = await ctxC.newPage();
    try {
      const userB = await createAndLoginTestUser(pageB);
      const userC = await createAndLoginTestUser(pageC);

      // Seed two existing DMs from User A's side, B last so it sorts above C
      // by last-activity (newest first). User A then leaves the chat root open
      // — *not* in either DM — so subsequent activity must bump via subscription.
      const dmA = new DMPage(page);
      const aToC = await dmA.startConversation(userC.login);
      await aToC.sendMessage('seed C');
      const aToB = await dmA.startConversation(userB.login);
      await aToB.sendMessage('seed B');

      await page.goto(routes.chat);
      await page.waitForURL(routes.chat);
      await expect(
        page.getByRole('button', { name: /direct messages/i })
      ).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Snapshot the order before C posts. dmRows() returns the visible DM
      // sidebar items; the order reflects the rooms-store array order.
      const dmRows = () =>
        page.locator('nav a.sidebar-item').filter({
          has: page.getByText(new RegExp(`^(${userB.displayName}|${userC.displayName})$`))
        });
      const initial = await dmRows().allTextContents();
      expect(initial[0]).toContain(userB.displayName);

      // User C posts into their existing DM with A. A's sidebar should bump
      // C's row to the top and mark it unread — both arrive over the
      // unified myEvents subscription (which carries channel and DM
      // events together), with RoomList listening for root
      // MessagePostedEvents on the server-wide stream for the unread
      // bookkeeping.
      const cToA = await new DMPage(pageC).startConversation(userA.login);
      await cToA.sendMessage(`bump ${Date.now()}`);

      // Bumped to top:
      await expect
        .poll(async () => (await dmRows().allTextContents())[0], {
          timeout: TIMEOUTS.REALTIME_EVENT
        })
        .toContain(userC.displayName);

      // Some indicator is present on C's row. An incoming DM creates a
      // persistent DMMessageNotification, so the row renders the
      // higher-priority notification dot — "new direct message" — rather
      // than the plain unread dot. Assert on whichever applies.
      const cRow = page
        .locator('nav a.sidebar-item')
        .filter({ has: page.getByText(userC.displayName, { exact: true }) });
      await expect(
        cRow.getByText(/new direct message|unread messages/)
      ).toBeAttached({ timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await ctxB.close();
      await ctxC.close();
    }
  });

  test('posting in a not-at-the-top DM bumps it to the top without reload', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    const ctxB = await browser.newContext({ baseURL: serverURL });
    const ctxC = await browser.newContext({ baseURL: serverURL });
    const pageB = await ctxB.newPage();
    const pageC = await ctxC.newPage();
    try {
      const userB = await createAndLoginTestUser(pageB);
      const userC = await createAndLoginTestUser(pageC);

      // Seed two DMs from User A. C goes second so it ends up at the top by
      // last-activity. We then post into B's DM (not at the top) and assert
      // the row jumps without a reload.
      const dmA = new DMPage(page);
      const aToB = await dmA.startConversation(userB.login);
      await aToB.sendMessage('seed B');
      const aToC = await dmA.startConversation(userC.login);
      await aToC.sendMessage('seed C');
      // C is now most-recent.

      await page.goto(routes.chat);
      await page.waitForURL(routes.chat);
      const dmRows = () =>
        page.locator('nav a.sidebar-item').filter({
          has: page.getByText(new RegExp(`^(${userB.displayName}|${userC.displayName})$`))
        });
      await expect
        .poll(async () => (await dmRows().allTextContents())[0], {
          timeout: TIMEOUTS.REALTIME_EVENT
        })
        .toContain(userC.displayName);

      // Open the not-at-the-top DM (B) and post a message in it.
      await dmA.openConversation(userB.displayName);
      const bRoom = new RoomPage(page);
      await bRoom.sendMessage(`A bumps B ${Date.now()}`);

      // The DM list (still in the sidebar of the same chrome) should re-sort
      // with B at the top. No reload — relies on the viewer's own
      // MessagePostedEvent flowing back to them via the unified live stream.
      await expect
        .poll(async () => (await dmRows().allTextContents())[0], {
          timeout: TIMEOUTS.REALTIME_EVENT
        })
        .toContain(userB.displayName);
    } finally {
      await ctxB.close();
      await ctxC.close();
    }
  });

  test('server icon picks up DM activity and clicking it opens the DM', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    const ctxB = await browser.newContext({ baseURL: serverURL });
    const pageB = await ctxB.newPage();
    try {
      await createAndLoginTestUser(pageB);

      // User A on chat root with no DMs yet — server icon has no indicator.
      await page.goto(routes.chat);
      await page.waitForURL(routes.chat);
      // Scope to the Server Gutter so we don't collide with notification
      // buttons rendered inside the Server Sidebar.
      const serverIconWrapper = page
        .locator('.server-gutter .server-icon-wrapper')
        .filter({ has: page.getByTestId('server-icon') });
      await expect(serverIconWrapper).toBeVisible();
      await expect(serverIconWrapper.getByRole('button')).toHaveCount(0);

      // User B starts a DM with User A and posts. The persistent
      // DMMessageNotification surfaces as a server-icon indicator on A's
      // sidebar, even though A is on the chat root and not in the DM.
      const dmB = new DMPage(pageB);
      const roomB = await dmB.startConversation(userA.login);
      await roomB.sendMessage(`server-icon DM ${Date.now()}`);

      const indicator = serverIconWrapper.getByRole('button');
      await expect(indicator).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

      // Clicking takes A straight to the DM conversation, not just to the
      // chat root — same affordance channel rooms get.
      await indicator.click();
      await page.waitForURL(routes.patterns.anyRoom);
      // The DM has a deterministic ID; we don't recompute it here, but the
      // post-click URL must be a /chat/-/{id} room URL.
      expect(page.url()).not.toMatch(/\/chat\/-\/?$/);
    } finally {
      await ctxB.close();
    }
  });

  test('collapsed Direct Messages section reveals freshly-unread DMs', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    const ctxB = await browser.newContext({ baseURL: serverURL });
    const ctxC = await browser.newContext({ baseURL: serverURL });
    const pageB = await ctxB.newPage();
    const pageC = await ctxC.newPage();
    try {
      const userB = await createAndLoginTestUser(pageB);
      const userC = await createAndLoginTestUser(pageC);

      // Seed two existing DMs from User A. Both have content, so both end
      // up in the merged sidebar.
      const dmA = new DMPage(page);
      const aToB = await dmA.startConversation(userB.login);
      await aToB.sendMessage('seed B');
      const aToC = await dmA.startConversation(userC.login);
      await aToC.sendMessage('seed C');

      await page.goto(routes.chat);
      await page.waitForURL(routes.chat);

      const groupHeader = page.getByRole('button', { name: /direct messages/i });
      const dmRow = (displayName: string) =>
        page.locator('nav a.sidebar-item').filter({
          has: page.getByText(displayName, { exact: true })
        });

      // Both DMs are visible in the expanded section.
      await expect(dmRow(userB.displayName)).toBeVisible();
      await expect(dmRow(userC.displayName)).toBeVisible();

      // Collapse the group. Both rows hide because neither is highlighted
      // (no unread, not active, no notification). The group header stays.
      await groupHeader.click();
      await expect(dmRow(userB.displayName)).toBeHidden({ timeout: TIMEOUTS.UI_STANDARD });
      await expect(dmRow(userC.displayName)).toBeHidden({ timeout: TIMEOUTS.UI_STANDARD });

      // User C posts into their existing DM with A. The fresh DM-message
      // notification flips the row's `isHighlighted` predicate, so the
      // collapsed group reveals the C row even though it's still
      // collapsed. The user can never miss a message because the section
      // is collapsed.
      const cToA = await new DMPage(pageC).startConversation(userA.login);
      await cToA.sendMessage(`reveal-on-collapse ${Date.now()}`);

      await expect(dmRow(userC.displayName)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      // The other DM row stays hidden because nothing has happened in it.
      await expect(dmRow(userB.displayName)).toBeHidden();
    } finally {
      await ctxB.close();
      await ctxC.close();
    }
  });

  test('user with denied message.post still sees existing DM conversations', async ({
    page,
    browser,
    serverURL
  }) => {
    test.setTimeout(60_000);

    // Admin context: also doubles as the DM partner so the regular user has
    // a real DM to filter out. All admin-side setup goes through the GraphQL
    // API to avoid the slow UI-driven path.
    const adminUser = await loginAsAdmin(page);

    const regularContext = await browser.newContext({ baseURL: serverURL });
    const regularPage = await regularContext.newPage();
    try {
      const regularUser = await createAndLoginTestUser(regularPage);

      // Admin starts a DM with the regular user (via API) and seeds it so
      // the conversation isn't filtered by the active DM-room list.
      const startResp = await page.request.post('/api/graphql', {
        headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
        data: {
          query: `mutation($input: StartDMInput!) { startDM(input: $input) { id } }`,
          variables: { input: { participantIds: [regularUser.id] } }
        }
      });
      const dmRoomId = (await startResp.json()).data.startDM.id as string;
      await postMessageViaAPI(page, dmRoomId, 'seed');

      // Deny message.post BEFORE the regular user navigates. This should stop
      // starting/sending DMs, not reading an existing DM.
      const denyRole = await denyUserPermission(page, regularUser.id!, 'message.post');
      try {
        const deniedStartResp = await regularPage.request.post('/api/graphql', {
          headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
          data: {
            query: `mutation($input: StartDMInput!) { startDM(input: $input) { id } }`,
            variables: { input: { participantIds: [adminUser.id!] } }
          }
        });
        const deniedStartJson = await deniedStartResp.json();
        expect(deniedStartJson.data?.startDM).toBeFalsy();
        expect(deniedStartJson.errors?.length ?? 0).toBeGreaterThan(0);

        await regularPage.goto(routes.chat);
        await regularPage.waitForURL(routes.chat);

        // Wait for the sidebar's room list to render so the assertion below
        // is comparing against a settled DOM — Overview is always there for a
        // primary-space member (post f7b1a9df the Browse Rooms link was
        // renamed to Overview).
        await expect(
          regularPage.getByRole('link', { name: /overview/i })
        ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

        // DM read access is membership-based, so the seeded conversation still
        // appears even while message.post is denied.
        await expect(
          regularPage.getByRole('button', { name: /direct messages/i })
        ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

        await regularPage.goto(routes.room(dmRoomId));
        await regularPage.waitForURL(routes.patterns.anyRoom);

        const roomPage = new RoomPage(regularPage);
        await expect(roomPage.getMessage('seed').locator).toBeVisible({
          timeout: TIMEOUTS.UI_STANDARD
        });
        await expect(roomPage.messageInput).toHaveAttribute('contenteditable', 'false');
        await expect(roomPage.sendButton).toBeDisabled();

        await roomPage
          .getMessage('seed')
          .locator.getByRole('button', { name: adminUser.displayName })
          .click();

        const profileDialog = regularPage.getByRole('dialog', { name: 'User profile' });
        await expect(profileDialog).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
        await expect(profileDialog.getByRole('button', { name: 'Send Message' })).toBeHidden();
      } finally {
        await clearUserPermissionOverride(
          page,
          regularUser.id!,
          'message.post',
          denyRole
        );
      }
    } finally {
      await regularContext.close();
    }
  });
});
