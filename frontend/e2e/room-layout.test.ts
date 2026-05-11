import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  joinSpace,
  loginAsAdminAndUsePrimarySpace
} from './fixtures/testUser';
import { SpaceAdminPage } from './pages';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

// ============================================================================
// Types
// ============================================================================

interface TestSpace {
  id: string;
  name: string;
}

interface RoomLayoutSection {
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

async function updateRoomLayoutViaAPI(
  page: Page,
  sections: RoomLayoutSection[]
): Promise<void> {
  await gqlRequest(
    page,
    `mutation($input: UpdateRoomLayoutInput!) {
			updateRoomLayout(input: $input) {
				sections { id name rooms { id } }
			}
		}`,
    {
      input: { sections: sections.map((s) => ({
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
): Promise<{ sections: { id: string; name: string; rooms: { id: string }[] }[] } | null> {
  const data = await gqlRequest<{
    server: {
      roomLayout: { sections: { id: string; name: string; rooms: { id: string }[] }[] } | null;
    };
  }>(
    page,
    `query { server { roomLayout { sections { id name rooms { id } } } } }`
  );
  return data.server.roomLayout;
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

async function setRoomAutoJoinViaAPI(
  page: Page,
  roomId: string,
  autoJoin: boolean
): Promise<void> {
  await gqlRequest(
    page,
    `mutation($input: SetRoomAutoJoinInput!) { setRoomAutoJoin(input: $input) { id autoJoin } }`,
    { input: { roomId, autoJoin } }
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
async function waitForSidebarSections(page: Page, expectedCount: number): Promise<string[]> {
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
    test('no layout configured — rooms display alphabetically', async ({ page }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create rooms with names that aren't already alphabetical
      const charlieId = await createRoomViaAPI(page, 'charlie');
      const alphaId = await createRoomViaAPI(page, 'alpha');
      const bravoId = await createRoomViaAPI(page, 'bravo');

      // Join all rooms (owner auto-joins announcements + general, but not these)
      await joinRoomViaAPI(page, charlieId);
      await joinRoomViaAPI(page, alphaId);
      await joinRoomViaAPI(page, bravoId);

      await navigateToSpace(page);

      // 5 rooms total: announcements + general (default) + alpha, bravo, charlie
      const roomNames = await waitForSidebarRooms(page, 5);
      expect(roomNames).toEqual(['alpha', 'announcements', 'bravo', 'charlie', 'general']);
    });

    test('layout sections render in sidebar', async ({ page }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');
      const bravoId = await createRoomViaAPI(page, 'bravo');
      const deltaId = await createRoomViaAPI(page, 'delta');

      await joinRoomViaAPI(page, alphaId);
      await joinRoomViaAPI(page, bravoId);
      await joinRoomViaAPI(page, deltaId);

      // Configure layout with 2 sections (include all rooms to avoid unsectioned)
      await updateRoomLayoutViaAPI(page, [
        { id: 'sec-general', name: 'General', roomIds: [announcementsId, generalId, alphaId] },
        { id: 'sec-projects', name: 'Projects', roomIds: [bravoId, deltaId] }
      ]);

      await navigateToSpace(page);

      // Verify section headers
      const headers = await waitForSidebarSections(page, 2);
      expect(headers).toEqual(['General', 'Projects']);

      // Verify rooms are visible in configured order (5 total)
      const roomNames = await waitForSidebarRooms(page, 5);
      expect(roomNames).toEqual(['announcements', 'general', 'alpha', 'bravo', 'delta']);
    });

    test('unsectioned rooms appear at bottom of sidebar', async ({ page }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const { generalId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');
      const extraId = await createRoomViaAPI(page, 'extra');

      await joinRoomViaAPI(page, alphaId);
      await joinRoomViaAPI(page, extraId);

      // Only put general and alpha in a section; announcements + extra are unsectioned
      await updateRoomLayoutViaAPI(page, [
        { id: 'sec-main', name: 'Main', roomIds: [generalId, alphaId] }
      ]);

      await navigateToSpace(page);

      // Sectioned rooms first (general, alpha), then unsectioned alphabetically (announcements, extra)
      const roomNames = await waitForSidebarRooms(page, 4);
      expect(roomNames).toEqual(['general', 'alpha', 'announcements', 'extra']);
    });

    test('empty sections are hidden from sidebar', async ({ page, browser, serverURL }) => {
      // User A (owner) creates space and configures layout
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const secretId = await createRoomViaAPI(page, 'secret');

      // Configure layout: "Public" has both default rooms, "Secret" has secret
      await updateRoomLayoutViaAPI(page, [
        { id: 'sec-public', name: 'Public', roomIds: [announcementsId, generalId] },
        { id: 'sec-secret', name: 'Secret', roomIds: [secretId] }
      ]);

      // User B joins space — auto-joins announcements + general, but not secret
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, "");

        await navigateToSpace(page2);

        // User B should only see the "Public" section, not "Secret"
        const headers = await waitForSidebarSections(page2, 1);
        expect(headers).toEqual(['Public']);

        // And only see the 2 default rooms
        const roomNames = await waitForSidebarRooms(page2, 2);
        expect(roomNames).toEqual(['announcements', 'general']);
      } finally {
        await context2.close();
      }
    });

    test('section collapse/expand persists across navigation', async ({ page }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');
      const bravoId = await createRoomViaAPI(page, 'bravo');

      await joinRoomViaAPI(page, alphaId);
      await joinRoomViaAPI(page, bravoId);

      await updateRoomLayoutViaAPI(page, [
        { id: 'sec-main', name: 'Main', roomIds: [announcementsId, generalId, alphaId] },
        { id: 'sec-other', name: 'Other', roomIds: [bravoId] }
      ]);

      await navigateToSpace(page);

      // Verify both sections visible with all rooms
      const headers = await waitForSidebarSections(page, 2);
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
      const space = await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');

      await joinRoomViaAPI(page, alphaId);

      // User B joins the space
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, "");
        await joinRoomViaAPI(page2, alphaId);

        // User B navigates to space — no layout yet, rooms render under
        // the default "Rooms" collapsible group.
        await navigateToSpace(page2);
        await waitForSidebarRooms(page2, 3); // announcements + general + alpha
        const headersBefore = await waitForSidebarSections(page2, 1);
        expect(headersBefore).toEqual(['Rooms']);

        // User A configures a layout
        await updateRoomLayoutViaAPI(page, [
          { id: 'sec-main', name: 'Organized', roomIds: [announcementsId, generalId, alphaId] }
        ]);

        // User B should see the section header appear in real-time
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

      // Update layout
      await updateRoomLayoutViaAPI(page, [
        {
          id: 'sec-one',
          name: 'Section One',
          roomIds: [bravoId, alphaId, generalId]
        }
      ]);

      // Query it back
      const layout = await getRoomLayoutViaAPI(page);
      expect(layout).not.toBeNull();
      expect(layout!.sections).toHaveLength(1);
      expect(layout!.sections[0].name).toBe('Section One');
      expect(layout!.sections[0].rooms.map((r) => r.id)).toEqual([bravoId, alphaId, generalId]);
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
            query: `mutation($input: UpdateRoomLayoutInput!) {
							updateRoomLayout(input: $input) {
								sections { id name }
							}
						}`,
            variables: {
              input: { sections: [{ id: 'sec-hack', name: 'Hacked', roomIds: [generalId] }]
              }
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

      // Create a section
      await spaceAdminRoomsPage.createSection('My Section');
      await spaceAdminRoomsPage.expectSectionVisible('My Section');

      // Rename the section
      await spaceAdminRoomsPage.renameSection('Renamed Section');
      await spaceAdminRoomsPage.expectSectionVisible('Renamed Section');

      // Delete the section
      await spaceAdminRoomsPage.deleteSection();
      await spaceAdminRoomsPage.expectSectionNotVisible('Renamed Section');
    });

    test('layout auto-saves and persists', async ({ page, spaceAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create extra rooms
      await createRoomViaAPI(page, 'alpha');
      await createRoomViaAPI(page, 'bravo');

      await spaceAdminRoomsPage.goto(space.id);

      // Create a section
      await spaceAdminRoomsPage.createSection('Important');
      await spaceAdminRoomsPage.expectSectionVisible('Important');

      // Verify layout auto-saves (poll API until it appears)
      await expect(async () => {
        const layout = await getRoomLayoutViaAPI(page);
        expect(layout).not.toBeNull();
        expect(layout!.sections).toHaveLength(1);
        expect(layout!.sections[0].name).toBe('Important');
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [250, 500, 1000] });
    });
  });

  test.describe('Edge Cases', () => {
    test('clearing layout reverts to alphabetical display', async ({ page }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');

      await joinRoomViaAPI(page, alphaId);

      // Configure layout with a section
      await updateRoomLayoutViaAPI(page, [
        { id: 'sec-main', name: 'Main', roomIds: [generalId, alphaId, announcementsId] }
      ]);

      await navigateToSpace(page);

      // Verify section appears
      const headers = await waitForSidebarSections(page, 1);
      expect(headers).toEqual(['Main']);

      // Clear layout by setting empty sections
      await updateRoomLayoutViaAPI(page, []);

      // Wait for real-time update to swap the "Main" section header for
      // the default "Rooms" group that holds an unsectioned room list.
      await expect(async () => {
        const headers = await page.locator('.room-list button.uppercase').allTextContents();
        expect(headers.map((h) => h.trim())).toEqual(['Rooms']);
      }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });

      // Rooms should now display alphabetically inside the Rooms group
      const roomNames = await waitForSidebarRooms(page, 3);
      expect(roomNames).toEqual(['alpha', 'announcements', 'general']);
    });

    test('rooms user has not joined are hidden from sections', async ({
      page,
      browser,
      serverURL
    }) => {
      // User A creates space with extra rooms
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const privateId = await createRoomViaAPI(page, 'private');
      const publicId = await createRoomViaAPI(page, 'public');

      await joinRoomViaAPI(page, privateId);
      await joinRoomViaAPI(page, publicId);

      // Configure layout with all rooms in one section
      await updateRoomLayoutViaAPI(page, [
        {
          id: 'sec-all',
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

      // Room should still be visible (now in Archived zone) and removed from layout
      await expect(async () => {
        await spaceAdminRoomsPage.expectRoomVisible('to-archive');
        const layout = await getRoomLayoutViaAPI(page);
        if (layout) {
          const allRoomIds = layout.sections.flatMap((s) => s.rooms.map((r) => r.id));
          expect(allRoomIds).not.toContain(roomId);
        }
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

  test.describe('Auto-Join', () => {
    test('admin can toggle auto-join on a room', async ({ page, spaceAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      await createRoomViaAPI(page, 'toggle-me');

      await spaceAdminRoomsPage.goto(space.id);

      // Enable auto-join
      await spaceAdminRoomsPage.toggleAutoJoin('toggle-me');
      await expect(async () => {
        await spaceAdminRoomsPage.expectAutoJoinEnabled('toggle-me');
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });

      // Disable auto-join
      await spaceAdminRoomsPage.toggleAutoJoin('toggle-me');
      await expect(async () => {
        await spaceAdminRoomsPage.expectAutoJoinDisabled('toggle-me');
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    });

    test('new members auto-join rooms with auto_join enabled', async ({
      page,
      browser,
      serverURL
    }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const autoRoom = await createRoomViaAPI(page, 'welcome');
      const manualRoom = await createRoomViaAPI(page, 'opt-in');
      await joinRoomViaAPI(page, autoRoom);
      await joinRoomViaAPI(page, manualRoom);

      // Enable auto_join on the welcome room only
      await setRoomAutoJoinViaAPI(page, autoRoom, true);

      // New user joins the space
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        await createAndLoginTestUser(page2);
        await joinSpace(page2, "");

        // Navigate to space — should see auto-joined rooms in sidebar
        await navigateToSpace(page2);

        // Should see the auto-join room (announcements, general are also auto-joined by default)
        const roomNames = await waitForSidebarRooms(page2, 3);
        expect(roomNames).toContain('welcome');
        expect(roomNames).toContain('announcements');
        expect(roomNames).toContain('general');
        // Should NOT see the manual room
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

      // Create a room
      await spaceAdminRoomsPage.createRoom('fresh-room');

      // Room should appear in admin page
      await spaceAdminRoomsPage.expectRoomVisible('fresh-room', TIMEOUTS.UI_STANDARD);
    });

    test('deleting section with rooms moves them to unsorted', async ({
      page,
      spaceAdminRoomsPage
    }) => {
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);

      // Create a section with rooms via API
      await updateRoomLayoutViaAPI(page, [
        {
          id: 'doomed',
          name: 'Doomed Section',
          roomIds: [generalId, announcementsId]
        }
      ]);

      await spaceAdminRoomsPage.goto(space.id);
      await spaceAdminRoomsPage.expectSectionVisible('Doomed Section');

      // Delete the section (confirms dialog)
      await spaceAdminRoomsPage.deleteSection();

      // Section should be gone, rooms should still be on the page (moved to Unsorted)
      await spaceAdminRoomsPage.expectSectionNotVisible('Doomed Section');
      await spaceAdminRoomsPage.expectRoomVisible('general');
      await spaceAdminRoomsPage.expectRoomVisible('announcements');

      // Verify via API that layout no longer has the section
      await expect(async () => {
        const layout = await getRoomLayoutViaAPI(page);
        if (layout === null) return; // Layout cleared entirely = also fine
        const sectionNames = layout.sections.map((s) => s.name);
        expect(sectionNames).not.toContain('Doomed Section');
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [250, 500, 1000] });
    });
  });
});
