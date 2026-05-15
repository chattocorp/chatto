import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  joinSpace,
  loginAsAdminAndUsePrimarySpace
} from './fixtures/testUser';
import { SpaceAdminPage } from './pages';
import { TIMEOUTS } from './constants';
import { postMessageViaAPI } from './fixtures/graphqlHelpers';
import * as routes from './routes';

// ============================================================================
// Types
// ============================================================================

interface TestSpace {
  id: string;
  name: string;
}

interface RoomSet {
  id: string;
  name: string;
  roomIds: string[];
}

// ============================================================================
// GraphQL Helpers (use page.request.post to avoid browser context issues)
// ============================================================================

async function gqlRequest<T>(
  page: Page,
  query: string,
  variables?: Record<string, unknown>
): Promise<T> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: { query, variables }
  });
  expect(resp.ok()).toBeTruthy();
  const json = await resp.json();
  if (json.errors) throw new Error(JSON.stringify(json.errors));
  return json.data;
}

async function createSpaceViaAPI(page: Page, _name?: string): Promise<TestSpace> {
  // Issue #330 / ADR-027: createSpace mutation is gone. Re-login as e2eadmin
  // (the bootstrap space owner) and return the primary space, so admin-style
  // operations in this test still run with sufficient permissions.
  return loginAsAdminAndUsePrimarySpace(page);
}

async function createRoomViaAPI(page: Page, name: string): Promise<string> {
  const data = await gqlRequest<{ createRoom: { id: string; name: string } }>(
    page,
    `mutation($input: CreateRoomInput!) { createRoom(input: $input) { id name } }`,
    { input: { name } }
  );
  return data.createRoom.id;
}

async function joinRoomViaAPI(page: Page, roomId: string): Promise<void> {
  const data = await gqlRequest<{ joinRoom: boolean }>(
    page,
    `mutation($input: JoinRoomInput!) { joinRoom(input: $input) }`,
    { input: { roomId } }
  );
  expect(data.joinRoom).toBe(true);
}

async function updateRoomLayoutViaAPI(page: Page, sets: RoomSet[]): Promise<void> {
  await gqlRequest(
    page,
    `mutation($input: UpdateRoomSetsInput!) {
			updateRoomSets(input: $input) { id name rooms { id } }
		}`,
    {
      input: {
        sets: sets.map((s) => ({
          id: s.id,
          name: s.name,
          roomIds: s.roomIds
        }))
      }
    }
  );
}

async function getRoomLayoutViaAPI(
  page: Page
): Promise<{ sets: { id: string; name: string; rooms: { id: string }[] }[] } | null> {
  const data = await gqlRequest<{
    server: { roomSets: { id: string; name: string; rooms: { id: string }[] }[] };
  }>(page, `query { server { roomSets { id name rooms { id } } } }`);
  return { sets: data.server.roomSets };
}

/**
 * Returns the ID of the first (seed) room set. Every server boots with a
 * "Rooms" set after #454; tests need its ID to construct layouts that
 * include the auto-created announcements/general rooms.
 */
async function getSeedSetId(page: Page): Promise<string> {
  const layout = await getRoomLayoutViaAPI(page);
  if (!layout || layout.sets.length === 0) {
    throw new Error('Expected the seed room set to exist');
  }
  return layout.sets[0].id;
}

async function archiveRoomViaAPI(page: Page, roomId: string): Promise<void> {
  await gqlRequest(
    page,
    `mutation($input: ArchiveRoomInput!) { archiveRoom(input: $input) { id archived } }`,
    { input: { roomId } }
  );
}

async function unarchiveRoomViaAPI(page: Page, roomId: string): Promise<void> {
  await gqlRequest(
    page,
    `mutation($input: UnarchiveRoomInput!) { unarchiveRoom(input: $input) { id archived } }`,
    { input: { roomId } }
  );
}

async function setRoomGlobalViaAPI(
  page: Page,
  roomId: string,
  isGlobal: boolean
): Promise<void> {
  await gqlRequest(
    page,
    `mutation($input: SetRoomGlobalInput!) { setRoomGlobal(input: $input) { id isGlobal } }`,
    { input: { roomId, isGlobal } }
  );
}

/** Returns IDs of both default rooms (announcements, general) created with every space. */
async function getDefaultRoomIds(
  page: Page
): Promise<{ announcementsId: string; generalId: string }> {
  const data = await gqlRequest<{ server: { rooms: { id: string; name: string }[] } }>(
    page,
    `query { server { rooms(type: CHANNEL) { id name } } }`
  );
  const gen = data.server.rooms.find((r) => r.name === 'general');
  const ann = data.server.rooms.find((r) => r.name === 'announcements');
  if (!gen) throw new Error('Default "general" room not found');
  if (!ann) throw new Error('Default "announcements" room not found');
  return { announcementsId: ann.id, generalId: gen.id };
}

// ============================================================================
// Sidebar Helpers
// ============================================================================

async function navigateToSpace(page: Page): Promise<void> {
  await page.goto(routes.space());
  await expect(page.locator('.room-list')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
}

/**
 * Wait for exactly `expectedCount` rooms to appear in the sidebar, then return their names in order.
 */
async function waitForSidebarRooms(page: Page, expectedCount: number): Promise<string[]> {
  const roomLinks = page.locator('.room-list a .truncate');
  await expect(async () => {
    expect(await roomLinks.count()).toBe(expectedCount);
  }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });

  const names = await roomLinks.allTextContents();
  return names.map((n) => n.trim());
}

/**
 * Wait for exactly `expectedCount` section headers to appear, then return their names in order.
 */
async function waitForSidebarSets(page: Page, expectedCount: number): Promise<string[]> {
  const headers = page.locator('.room-list button.uppercase');

  if (expectedCount === 0) {
    // Confirm no headers appeared — use toPass() to give time for any
    // late-rendering headers to appear before asserting their absence
    await expect(async () => {
      expect(await headers.count()).toBe(0);
    }).toPass({ timeout: TIMEOUTS.SERVER_MUTATION_SYNC, intervals: [200, 500] });
    return [];
  }

  await expect(async () => {
    expect(await headers.count()).toBe(expectedCount);
  }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });

  const names: string[] = [];
  for (let i = 0; i < expectedCount; i++) {
    const text = await headers.nth(i).textContent();
    if (text) names.push(text.trim());
  }
  return names;
}

// ============================================================================
// Tests
// ============================================================================

test.describe('Room Layout', () => {
  test.describe('Sidebar Display', () => {
    test('rooms render in their seed-set insertion order', async ({ page }) => {
      // Post-ADR-031, every channel room is in a set. A fresh server boots
      // with one seed "Rooms" set containing the auto-created announcements
      // and general rooms; subsequently-created rooms are appended in the
      // order they were created. There is no "no layout" / alphabetical
      // fallback anymore.
      await createAndLoginTestUser(page);
      await createSpaceViaAPI(page);

      const charlieId = await createRoomViaAPI(page, 'charlie');
      const alphaId = await createRoomViaAPI(page, 'alpha');
      const bravoId = await createRoomViaAPI(page, 'bravo');

      await joinRoomViaAPI(page, charlieId);
      await joinRoomViaAPI(page, alphaId);
      await joinRoomViaAPI(page, bravoId);

      await navigateToSpace(page);

      // 5 rooms total: announcements + general (default) + charlie, alpha,
      // bravo (in creation order, since CreateRoom appends to the seed set).
      const roomNames = await waitForSidebarRooms(page, 5);
      expect(roomNames).toEqual(['announcements', 'general', 'charlie', 'alpha', 'bravo']);
    });

    test('layout sets render in sidebar', async ({ page }) => {
      await createAndLoginTestUser(page);
      await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');
      const bravoId = await createRoomViaAPI(page, 'bravo');
      const deltaId = await createRoomViaAPI(page, 'delta');

      await joinRoomViaAPI(page, alphaId);
      await joinRoomViaAPI(page, bravoId);
      await joinRoomViaAPI(page, deltaId);

      // Reshape the layout into two sets — every room must appear in exactly
      // one set (the seed set is replaced by these two named sets).
      const seedSetId = await getSeedSetId(page);
      await updateRoomLayoutViaAPI(page, [
        { id: seedSetId, name: 'General', roomIds: [announcementsId, generalId, alphaId] },
        { id: 'sec-projects', name: 'Projects', roomIds: [bravoId, deltaId] }
      ]);

      await navigateToSpace(page);

      const headers = await waitForSidebarSets(page, 2);
      expect(headers).toEqual(['General', 'Projects']);

      // Rooms in configured set order (5 total).
      const roomNames = await waitForSidebarRooms(page, 5);
      expect(roomNames).toEqual(['announcements', 'general', 'alpha', 'bravo', 'delta']);
    });

    test('empty sets are hidden from sidebar', async ({ page, browser, serverURL }) => {
      // User A (owner) creates space and configures layout
      await createAndLoginTestUser(page);
      await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const secretId = await createRoomViaAPI(page, 'secret');
      const seedSetId = await getSeedSetId(page);

      // Reshape: "Public" set holds the default rooms, "Secret" holds secret.
      await updateRoomLayoutViaAPI(page, [
        { id: seedSetId, name: 'Public', roomIds: [announcementsId, generalId] },
        { id: 'sec-secret', name: 'Secret', roomIds: [secretId] }
      ]);

      // User B joins the server — implicit membership in the default global
      // rooms (announcements, general), but not in secret.
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, '');

        await navigateToSpace(page2);

        // User B should only see the "Public" set, not "Secret" (empty for them).
        const headers = await waitForSidebarSets(page2, 1);
        expect(headers).toEqual(['Public']);

        const roomNames = await waitForSidebarRooms(page2, 2);
        expect(roomNames).toEqual(['announcements', 'general']);
      } finally {
        await context2.close();
      }
    });

    test('set collapse/expand persists across navigation', async ({ page }) => {
      await createAndLoginTestUser(page);
      await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');
      const bravoId = await createRoomViaAPI(page, 'bravo');

      await joinRoomViaAPI(page, alphaId);
      await joinRoomViaAPI(page, bravoId);

      const seedSetId = await getSeedSetId(page);
      await updateRoomLayoutViaAPI(page, [
        { id: seedSetId, name: 'Main', roomIds: [announcementsId, generalId, alphaId] },
        { id: 'sec-other', name: 'Other', roomIds: [bravoId] }
      ]);

      // Navigate to bravo (in the Other set) so the collapsed-but-active-room
      // visibility rule doesn't keep a Main room visible during the test.
      await page.goto(routes.room(bravoId));
      await expect(page.locator('.room-list')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Verify both sections visible with all rooms
      const headers = await waitForSidebarSets(page, 2);
      expect(headers).toEqual(['Main', 'Other']);
      await waitForSidebarRooms(page, 4);

      // Click section header to collapse "Main"
      await page.locator('.room-list button.uppercase', { hasText: 'Main' }).click();

      // "alpha", "general", "announcements" should be hidden
      await expect(
        page.locator('.room-list a .truncate', { hasText: 'general' })
      ).not.toBeVisible();
      await expect(page.locator('.room-list a .truncate', { hasText: 'alpha' })).not.toBeVisible();

      // "bravo" should still be visible (in Other section)
      await expect(page.locator('.room-list a .truncate', { hasText: 'bravo' })).toBeVisible();

      // Navigate away and back — collapsed state should persist.
      // Navigate directly to bravo (in the expanded "Other" section) so the
      // auto-redirect doesn't place the active room inside collapsed "Main".
      await page.goto('/chat');
      await page.goto(routes.room(bravoId));
      await expect(page.locator('.room-list')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

      // Main should still be collapsed — only bravo visible
      await waitForSidebarRooms(page, 1);
      await expect(
        page.locator('.room-list a .truncate', { hasText: 'general' })
      ).not.toBeVisible();
      await expect(page.locator('.room-list a .truncate', { hasText: 'bravo' })).toBeVisible();

      // Click to expand again
      await page.locator('.room-list button.uppercase', { hasText: 'Main' }).click();
      await expect(page.locator('.room-list a .truncate', { hasText: 'general' })).toBeVisible();
    });
  });

  test.describe('Real-time Sync', () => {
    test('layout change propagates to other users in real-time', async ({
      page,
      browser,
      serverURL
    }) => {
      // User A (owner) creates space and rooms
      await createAndLoginTestUser(page);
      await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');

      await joinRoomViaAPI(page, alphaId);

      // User B joins the space
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, '');
        await joinRoomViaAPI(page2, alphaId);

        // User B navigates to space — rooms render under the seed "Rooms" set.
        await navigateToSpace(page2);
        await waitForSidebarRooms(page2, 3); // announcements + general + alpha
        const headersBefore = await waitForSidebarSets(page2, 1);
        expect(headersBefore).toEqual(['Rooms']);

        // User A renames the seed set (keep the ID — renaming via the same
        // set preserves its permission grants).
        const seedSetId = await getSeedSetId(page);
        await updateRoomLayoutViaAPI(page, [
          { id: seedSetId, name: 'Organized', roomIds: [announcementsId, generalId, alphaId] }
        ]);

        // User B should see the new set name appear in real-time
        await expect(
          page2.locator('.room-list button.uppercase', { hasText: 'Organized' })
        ).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      } finally {
        await context2.close();
      }
    });
  });

  test.describe('API & Permissions', () => {
    test('admin can configure room layout via API', async ({ page }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const { generalId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');
      const bravoId = await createRoomViaAPI(page, 'bravo');

      // Owner must join rooms to see them in layout query (rooms are filtered by membership)
      await joinRoomViaAPI(page, alphaId);
      await joinRoomViaAPI(page, bravoId);

      // Reshape the seed set to a single named "Section One" with the test rooms.
      const seedSetId = await getSeedSetId(page);
      await updateRoomLayoutViaAPI(page, [
        {
          id: seedSetId,
          name: 'Section One',
          roomIds: [bravoId, alphaId, generalId]
        }
      ]);

      // Query it back
      const layout = await getRoomLayoutViaAPI(page);
      expect(layout).not.toBeNull();
      expect(layout!.sets).toHaveLength(1);
      expect(layout!.sets[0].name).toBe('Section One');
      expect(layout!.sets[0].rooms.map((r) => r.id)).toEqual([bravoId, alphaId, generalId]);
    });

    test('regular member cannot update layout (permission denied)', async ({
      page,
      browser,
      serverURL
    }) => {
      // User A (owner) creates space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const { generalId } = await getDefaultRoomIds(page);

      // User B joins as regular member
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, "");

        // User B tries to update room layout — should fail
        const resp = await page2.request.post('/api/graphql', {
          headers: {
            'Content-Type': 'application/json',
            'X-REQUEST-TYPE': 'GraphQL'
          },
          data: {
            query: `mutation($input: UpdateRoomSetsInput!) {
							updateRoomSets(input: $input) { id name }
						}`,
            variables: {
              input: { sets: [{ id: 'sec-hack', name: 'Hacked', roomIds: [generalId] }] }
            }
          }
        });

        const data = await resp.json();
        expect(data.errors).toBeTruthy();
        expect(data.errors[0].message).toContain('permission denied');
      } finally {
        await context2.close();
      }
    });

    test('regular member does not see Rooms nav item in space admin', async ({
      page,
      browser,
      serverURL
    }) => {
      // User A (owner) creates space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // User B joins as regular member
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, "");

        // Navigate to admin area directly — User B shouldn't see "Rooms" nav
        await page2.goto(routes.serverAdmin());
        // Wait for page to load
        await page2.waitForLoadState('networkidle');

        // User B shouldn't see the Rooms nav item (requires room.manage)
        const spaceAdminPage2 = new SpaceAdminPage(page2);
        await expect(spaceAdminPage2.roomsNavItem).not.toBeVisible();
      } finally {
        await context2.close();
      }
    });
  });

  test.describe('Admin UI', () => {
    test('admin can navigate to rooms page and see layout editor', async ({
      page,
      spaceAdminPage,
      spaceAdminRoomsPage
    }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to space admin
      await spaceAdminPage.goto(space.id);

      // Click Rooms nav item
      await expect(spaceAdminPage.roomsNavItem).toBeVisible();
      await spaceAdminPage.roomsNavItem.click();

      // Should see the rooms admin page with action buttons and default rooms
      await spaceAdminRoomsPage.expectVisible();
      await spaceAdminRoomsPage.expectRoomVisible('general');
    });

    test('admin can create, rename, and delete sections', async ({ page, spaceAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      await spaceAdminRoomsPage.goto(space.id);

      // Create a section (the seed "Rooms" set is also present)
      await spaceAdminRoomsPage.createSet('My Section');
      await spaceAdminRoomsPage.expectSetVisible('My Section');

      // Rename the section
      await spaceAdminRoomsPage.renameSet('My Section', 'Renamed Section');
      await spaceAdminRoomsPage.expectSetVisible('Renamed Section');

      // Delete the section
      await spaceAdminRoomsPage.deleteSet('Renamed Section');
      await spaceAdminRoomsPage.expectSetNotVisible('Renamed Section');
    });

    test('layout auto-saves and persists', async ({ page, spaceAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create extra rooms
      await createRoomViaAPI(page, 'alpha');
      await createRoomViaAPI(page, 'bravo');

      await spaceAdminRoomsPage.goto(space.id);

      // Create a section
      await spaceAdminRoomsPage.createSet('Important');
      await spaceAdminRoomsPage.expectSetVisible('Important');

      // Verify layout auto-saves (poll API until it appears)
      await expect(async () => {
        const layout = await getRoomLayoutViaAPI(page);
        expect(layout).not.toBeNull();
        // The original seed set + the new "Important" set = 2 sets.
        const names = layout!.sets.map((s) => s.name);
        expect(names).toContain('Important');
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [250, 500, 1000] });
    });
  });

  test.describe('Edge Cases', () => {
    test('rooms user has not joined are hidden from sets', async ({
      page,
      browser,
      serverURL
    }) => {
      // User A creates space with extra rooms
      await createAndLoginTestUser(page);
      await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const privateId = await createRoomViaAPI(page, 'private');
      const publicId = await createRoomViaAPI(page, 'public');

      await joinRoomViaAPI(page, privateId);
      await joinRoomViaAPI(page, publicId);

      // Put every channel room into the seed set.
      const seedSetId = await getSeedSetId(page);
      await updateRoomLayoutViaAPI(page, [
        {
          id: seedSetId,
          name: 'All',
          roomIds: [announcementsId, generalId, privateId, publicId]
        }
      ]);

      // User B joins space and only the public room (plus default announcements + general)
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, "");
        await joinRoomViaAPI(page2, publicId);

        await navigateToSpace(page2);

        // User B should see announcements, general, and public, but NOT private
        const roomNames = await waitForSidebarRooms(page2, 3);
        expect(roomNames).toContain('announcements');
        expect(roomNames).toContain('general');
        expect(roomNames).toContain('public');
        expect(roomNames).not.toContain('private');
      } finally {
        await context2.close();
      }
    });
  });

  test.describe('Archiving', () => {
    test('admin can archive a room via admin UI', async ({ page, spaceAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'to-archive');
      await joinRoomViaAPI(page, roomId);

      await spaceAdminRoomsPage.goto(space.id);

      // Archive the room via UI (click Archive, confirm dialog)
      await spaceAdminRoomsPage.archiveRoom('to-archive');

      // Room stays in its set (archive only flips the archived flag) but
      // its row now shows the Unarchive affordance instead of Archive.
      await expect(async () => {
        await spaceAdminRoomsPage.expectRoomVisible('to-archive');
        const layout = await getRoomLayoutViaAPI(page);
        if (layout) {
          const allRoomIds = layout.sets.flatMap((s) => s.rooms.map((r) => r.id));
          expect(allRoomIds).toContain(roomId);
        }
        await expect(
          spaceAdminRoomsPage.roomRow('to-archive').getByTitle('Unarchive room')
        ).toBeVisible();
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    });

    test('admin can unarchive a room via admin UI', async ({ page, spaceAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'was-archived');
      await joinRoomViaAPI(page, roomId);

      // Archive via API first
      await archiveRoomViaAPI(page, roomId);

      await spaceAdminRoomsPage.goto(space.id);

      // Unarchive the room via UI
      await spaceAdminRoomsPage.unarchiveRoom('was-archived');

      // Room should be unarchived via API
      await expect(async () => {
        const data = await gqlRequest<{ server: { rooms: { id: string; archived: boolean }[] } }>(
          page,
          `query { server { rooms(type: CHANNEL) { id archived } } }`
        );
        const room = data.server.rooms.find((r) => r.id === roomId);
        expect(room).toBeTruthy();
        expect(room!.archived).toBe(false);
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    });

    test('cancel archive dialog keeps room in place', async ({ page, spaceAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'stay-put');

      await spaceAdminRoomsPage.goto(space.id);

      // Click Archive but cancel the dialog
      await spaceAdminRoomsPage.clickArchive('stay-put');
      await spaceAdminRoomsPage.cancelDialog();

      // Room should still be non-archived — verify via API
      const data = await gqlRequest<{ server: { rooms: { id: string; archived: boolean }[] } }>(
        page,
        `query { server { rooms(type: CHANNEL) { id archived } } }`
      );
      const room = data.server.rooms.find((r) => r.id === roomId);
      expect(room).toBeTruthy();
      expect(room!.archived).toBe(false);
    });

    test('archived room disappears from member sidebar', async ({ page, browser, serverURL }) => {
      // User A (owner) creates space and rooms
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'will-vanish');
      await joinRoomViaAPI(page, roomId);

      // User B joins space and the room
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, "");
        await joinRoomViaAPI(page2, roomId);

        // User B navigates to the space and sees the room
        await navigateToSpace(page2);
        const initialRooms = await waitForSidebarRooms(page2, 3);
        expect(initialRooms).toContain('will-vanish');

        // User A archives the room
        await archiveRoomViaAPI(page, roomId);

        // User B's sidebar should update — room disappears
        await expect(async () => {
          const roomNames = await waitForSidebarRooms(page2, 2);
          expect(roomNames).not.toContain('will-vanish');
        }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [500, 1000, 2000] });
      } finally {
        await context2.close();
      }
    });

    test('archived room excluded from Browse Rooms', async ({ page, browser, serverURL }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const visibleId = await createRoomViaAPI(page, 'visible-room');
      const hiddenId = await createRoomViaAPI(page, 'hidden-room');
      await joinRoomViaAPI(page, visibleId);
      await joinRoomViaAPI(page, hiddenId);

      // Archive one room
      await archiveRoomViaAPI(page, hiddenId);

      // User B joins the space
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, "");

        // Navigate to Browse Rooms
        await page2.goto(routes.browseRooms);
        await expect(page2.getByRole('heading', { name: 'Browse Rooms' })).toBeVisible();

        // The non-archived room should be visible (not yet joined by User B)
        await expect(page2.getByText('visible-room')).toBeVisible();

        // The archived room should NOT be visible
        await expect(page2.getByText('hidden-room')).not.toBeVisible();
      } finally {
        await context2.close();
      }
    });

    test('unarchived room reappears in member sidebar', async ({ page, browser, serverURL }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'comeback');
      await joinRoomViaAPI(page, roomId);

      // User B joins space and the room, then room gets archived
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, "");
        await joinRoomViaAPI(page2, roomId);

        // Archive the room
        await archiveRoomViaAPI(page, roomId);

        // User B navigates to space — room should not be visible
        await navigateToSpace(page2);
        const roomsAfterArchive = await waitForSidebarRooms(page2, 2);
        expect(roomsAfterArchive).not.toContain('comeback');

        // Unarchive the room
        await unarchiveRoomViaAPI(page, roomId);

        // User B's sidebar should update — room reappears
        await expect(async () => {
          const roomNames = await waitForSidebarRooms(page2, 3);
          expect(roomNames).toContain('comeback');
        }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [500, 1000, 2000] });
      } finally {
        await context2.close();
      }
    });
  });

  test.describe('Global rooms', () => {
    test('admin can toggle the global flag on a room', async ({ page, spaceAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      await createRoomViaAPI(page, 'toggle-me');

      await spaceAdminRoomsPage.goto(space.id);

      // Enable global
      await spaceAdminRoomsPage.toggleGlobal('toggle-me');
      await expect(async () => {
        await spaceAdminRoomsPage.expectGlobalEnabled('toggle-me');
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });

      // Disable global
      await spaceAdminRoomsPage.toggleGlobal('toggle-me');
      await expect(async () => {
        await spaceAdminRoomsPage.expectGlobalDisabled('toggle-me');
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    });

    test('implicit member receives + posts in a global room live', async ({
      page,
      browser,
      serverURL
    }) => {
      // Admin owns the server and a global "broadcast" room.
      await createAndLoginTestUser(page);
      await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'broadcast');
      await setRoomGlobalViaAPI(page, roomId, true);

      // Seed one historical message before User B exists.
      const stamp = Date.now();
      const historicalBody = `pre-arrival ${stamp}`;
      await postMessageViaAPI(page, roomId, historicalBody);

      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        // User B is brand new — they never explicitly join any room.
        await createAndLoginTestUser(page2);
        await joinSpace(page2, '');

        // Navigate directly to the global room. Implicit membership means
        // User B should be able to open it without a Join step.
        await page2.goto(routes.room(roomId));
        await expect(page2.getByTestId('message-input')).toBeVisible({
          timeout: TIMEOUTS.UI_STANDARD
        });

        // Historical message is visible (read path works for implicit members).
        await expect(page2.getByText(historicalBody)).toBeVisible({
          timeout: TIMEOUTS.UI_STANDARD
        });

        // Admin posts while User B has the room open. User B must see it
        // arrive via the live subscription — this is the implicit-member
        // memberRooms cache path.
        const liveBody = `live one ${stamp}`;
        await postMessageViaAPI(page, roomId, liveBody);
        await expect(page2.getByText(liveBody)).toBeVisible({
          timeout: TIMEOUTS.REALTIME_EVENT
        });

        // User B posts via the UI — verifies the write path for an
        // implicit member (requireRoomMember passes via the is_global
        // short-circuit, permissions resolve at set scope).
        const replyBody = `reply from B ${stamp}`;
        const input = page2.getByTestId('message-input');
        await input.fill(replyBody);
        await input.press('Enter');
        await expect(page2.getByText(replyBody)).toBeVisible({
          timeout: TIMEOUTS.UI_STANDARD
        });
      } finally {
        await context2.close();
      }
    });

    test('new members see global rooms in their sidebar without joining', async ({
      page,
      browser,
      serverURL
    }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const globalRoom = await createRoomViaAPI(page, 'welcome');
      const manualRoom = await createRoomViaAPI(page, 'opt-in');
      await joinRoomViaAPI(page, globalRoom);
      await joinRoomViaAPI(page, manualRoom);

      // Mark the welcome room as global (implicit membership for all users).
      await setRoomGlobalViaAPI(page, globalRoom, true);

      // A brand-new user shows up.
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, "");
        await navigateToSpace(page2);

        // Should see the global room (and the default global ones —
        // announcements, general — which the bootstrap marks global).
        const roomNames = await waitForSidebarRooms(page2, 3);
        expect(roomNames).toContain('welcome');
        expect(roomNames).toContain('announcements');
        expect(roomNames).toContain('general');
        // Non-global rooms stay opt-in.
        expect(roomNames).not.toContain('opt-in');
      } finally {
        await context2.close();
      }
    });
  });

  test.describe('Admin Room Management', () => {
    test('admin can edit room name and description', async ({ page, spaceAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      await createRoomViaAPI(page, 'old-name');

      await spaceAdminRoomsPage.goto(space.id);

      // Edit the room
      await spaceAdminRoomsPage.editRoom('old-name', 'new-name', 'A shiny new description');

      // Should see updated name in the list
      await expect(async () => {
        await spaceAdminRoomsPage.expectRoomVisible('new-name');
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    });

    test('admin can create a room from admin page', async ({ page, spaceAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      await spaceAdminRoomsPage.goto(space.id);

      // Create a room from the seed "Rooms" set's header.
      await spaceAdminRoomsPage.createRoom('Rooms', 'fresh-room');

      // Room should appear in admin page
      await spaceAdminRoomsPage.expectRoomVisible('fresh-room', TIMEOUTS.UI_STANDARD);
    });

    test('admin can create a room in a non-seed set', async ({ page, spaceAdminRoomsPage }) => {
      // Regression: previously, creating a room from a set other than the
      // seed "Rooms" set silently dropped the setId or the room didn't
      // appear after refetch. Verify the room lands in the chosen set.
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Pre-create a second set via API so we don't race the autosave.
      const seedSetId = await getSeedSetId(page);
      const otherSetId = 'set-other-' + Math.random().toString(36).slice(2, 10);
      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      await updateRoomLayoutViaAPI(page, [
        { id: seedSetId, name: 'Rooms', roomIds: [generalId, announcementsId] },
        { id: otherSetId, name: 'Projects', roomIds: [] }
      ]);

      await spaceAdminRoomsPage.goto(space.id);
      await spaceAdminRoomsPage.expectSetVisible('Projects');

      // Create a room from the "Projects" set's header.
      await spaceAdminRoomsPage.createRoom('Projects', 'project-room');

      // Room must show up in the admin layout, inside the Projects set.
      await spaceAdminRoomsPage.expectRoomVisible('project-room', TIMEOUTS.UI_STANDARD);
      await expect(async () => {
        const layout = await getRoomLayoutViaAPI(page);
        expect(layout).not.toBeNull();
        const projects = layout!.sets.find((s) => s.id === otherSetId);
        expect(projects).toBeTruthy();
        expect(projects!.rooms.length).toBe(1);
        // And the seed "Rooms" set is unchanged.
        const rooms = layout!.sets.find((s) => s.id === seedSetId);
        expect(rooms!.rooms.length).toBe(2);
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    });

    test('delete button is disabled while a set still has rooms', async ({
      page,
      spaceAdminRoomsPage
    }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);

      const seedSetId = await getSeedSetId(page);
      await updateRoomLayoutViaAPI(page, [
        {
          id: seedSetId,
          name: 'Has Rooms',
          roomIds: [generalId, announcementsId]
        }
      ]);

      await spaceAdminRoomsPage.goto(space.id);
      await spaceAdminRoomsPage.expectSetVisible('Has Rooms');

      // With Unsorted gone, deletion of a non-empty set would orphan the
      // rooms — so the Delete button is disabled until they're moved out.
      const deleteBtn = spaceAdminRoomsPage
        .setHeaderRow('Has Rooms')
        .getByTitle('Move all rooms out of this set before deleting');
      await expect(deleteBtn).toBeVisible();
      await expect(deleteBtn).toBeDisabled();
    });
  });
});
