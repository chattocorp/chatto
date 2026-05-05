import { expect } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  loginAsAdmin,
  denyUserInstancePermission,
  clearUserInstancePermissionOverride
} from './fixtures/testUser';
import { DMPage } from './pages/DMPage';
import { RoomPage } from './pages/RoomPage';
import { postMessageViaAPI } from './fixtures/graphqlHelpers';
import { DM_SPACE_ID } from '../src/lib/constants';
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
      // User A's merged sidebar (ListDMConversations filters empty rooms).
      // The conversation ID is deterministic across the two users — pull it
      // from B's URL once the room loads.
      const dmPageB = new DMPage(page2);
      const roomB = await dmPageB.startConversation(userA.login);
      await roomB.sendMessage('seed from B');
      const conversationId = page2.url().split('/').pop()!;

      // User A navigates to the DM via the channel-shaped URL.
      await page.goto(routes.room(conversationId));
      await page.waitForURL(routes.patterns.anyRoom);

      // Bug #1 (the silent post): the SpaceEventProvider must subscribe to
      // DM-space events too, so MessagePostedEvent reaches RoomEventsPane
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

      // User B → User A: start DM and post so the DM survives the
      // ListDMConversations empty-room filter.
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

  test('user with denied dm.view sees no Direct Messages section', async ({
    page,
    browser,
    serverURL
  }) => {
    test.setTimeout(60_000);

    // Admin context: also doubles as the DM partner so the regular user has
    // a real DM to filter out. All admin-side setup goes through the GraphQL
    // API to avoid the slow UI-driven path.
    await loginAsAdmin(page);

    const regularContext = await browser.newContext({ baseURL: serverURL });
    const regularPage = await regularContext.newPage();
    try {
      const regularUser = await createAndLoginTestUser(regularPage);

      // Admin starts a DM with the regular user (via API) and seeds it so
      // the conversation isn't filtered by ListDMConversations.
      const startResp = await page.request.post('/api/graphql', {
        headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
        data: {
          query: `mutation($input: StartDMInput!) { startDM(input: $input) { id } }`,
          variables: { input: { participantIds: [regularUser.id] } }
        }
      });
      const dmRoomId = (await startResp.json()).data.startDM.id as string;
      await postMessageViaAPI(page, DM_SPACE_ID, dmRoomId, 'seed');

      // Deny dm.view BEFORE the regular user navigates, so their first sidebar
      // load already reflects the deny. (Reloading after a deny works too but
      // double-loads the page; keeping the test short.)
      const denyRole = await denyUserInstancePermission(page, regularUser.id!, 'dm.view');
      try {
        await regularPage.goto(routes.chat);
        await regularPage.waitForURL(routes.chat);

        // Wait for the sidebar's room list to render so the assertion below
        // is comparing against a settled DOM — Browse Rooms is always there
        // for a primary-space member.
        await expect(
          regularPage.getByRole('link', { name: /browse rooms/i })
        ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

        // dm.view denied → backend short-circuits the DM merge in Space.rooms,
        // the rooms store has no DMs, the sidebar header never renders.
        await expect(
          regularPage.getByRole('button', { name: /direct messages/i })
        ).not.toBeVisible();
      } finally {
        await clearUserInstancePermissionOverride(
          page,
          regularUser.id!,
          'dm.view',
          denyRole
        );
      }
    } finally {
      await regularContext.close();
    }
  });
});
