import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser, loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { ServerAdminPage } from './pages';
import { TIMEOUTS } from './constants';
import {
  connectPost,
  connectPostResponse,
  createRoomViaConnect,
  getDefaultRoomGroupIdViaConnect,
  joinRoomViaConnect
} from './fixtures/connectHelpers';
import * as routes from './routes';

// ============================================================================
// Types
// ============================================================================

interface TestServer {
  id: string;
  name: string;
}

interface RoomGroup {
  id: string;
  name: string;
  roomIds: string[];
}

interface AdminRoomLayoutRoomResponse {
  id?: string;
  name?: string;
  archived?: boolean;
}

interface AdminRoomLayoutGroupResponse {
  id?: string;
  name?: string;
  rooms?: AdminRoomLayoutRoomResponse[];
}

interface AdminRoomLayoutResponse {
  groups?: AdminRoomLayoutGroupResponse[];
}

async function usePrimaryServerViaAPI(page: Page, _name?: string): Promise<TestServer> {
  return loginAsAdminAndUsePrimaryServer(page);
}

async function createRoomViaAPI(page: Page, name: string): Promise<string> {
  const groupId = await getDefaultRoomGroupIdViaConnect(page);
  return createRoomViaConnect(page, name, groupId);
}

async function joinRoomViaAPI(page: Page, roomId: string): Promise<void> {
  await joinRoomViaConnect(page, roomId);
}

async function denyRoomPermissionViaAPI(
  page: Page,
  roomId: string,
  roleName: string,
  permission: string
): Promise<void> {
  const data = await connectPost<{ ok?: boolean }>(
    page,
    'chatto.admin.v1.AdminPermissionService/SetRolePermission',
    {
      roleName,
      permission,
      decision: 'PERMISSION_DECISION_DENY',
      scope: {
        kind: 'PERMISSION_SCOPE_KIND_ROOM',
        id: roomId
      }
    }
  );
  expect(data.ok).toBe(true);
}

// updateRoomLayoutViaAPI reshapes the room-group layout to match the
// `groups` argument using the per-key delta mutations (the bulk
// `updateRoomGroups` mutation was removed when storage was split — see
// ADR-031). The `id` on each input group is treated as a "stable name
// for this test": existing real IDs are used in place; anything else
// triggers a `createRoomGroup` call and the returned server-side ID is
// substituted in. After this returns, the server-side layout matches
// the input exactly — same group order, same room order within each
// group, no extras.
async function updateRoomLayoutViaAPI(page: Page, groups: RoomGroup[]): Promise<void> {
  // Snapshot the current state once so we can decide create vs. update.
  type CurrentGroup = { id: string; name: string; roomIds: string[] };
  const currentData = await getRoomLayoutViaAPI(page);
  const currentById = new Map<string, CurrentGroup>();
  for (const g of currentData?.groups ?? []) {
    currentById.set(g.id, { id: g.id, name: g.name, roomIds: g.rooms.map((r) => r.id) });
  }

  // Resolve every desired group to a real server-side ID, creating new
  // groups as needed and renaming existing ones if the name drifted.
  const resolvedIds: string[] = [];
  for (const desired of groups) {
    const existing = currentById.get(desired.id);
    if (existing) {
      if (existing.name !== desired.name) {
        await connectPost(page, 'chatto.admin.v1.AdminRoomLayoutService/UpdateRoomGroup', {
          groupId: existing.id,
          name: desired.name
        });
      }
      resolvedIds.push(existing.id);
    } else {
      const created = await connectPost<{ group?: { id?: string } }>(
        page,
        'chatto.admin.v1.AdminRoomLayoutService/CreateRoomGroup',
        { name: desired.name }
      );
      const newId = created.group?.id;
      if (!newId) throw new Error('CreateRoomGroup did not return a group id');
      currentById.set(newId, { id: newId, name: desired.name, roomIds: [] });
      resolvedIds.push(newId);
    }
  }

  // Move every room that is not already in its target group. After this
  // pass each group's membership set matches the desired set; positions
  // may still be wrong because MoveRoomToGroup appends to the end. The
  // per-key model requires every channel room to live in some group, so
  // any room the input didn't mention is parked in the first desired
  // group — preserves the bulk-replace spirit of the old API without
  // violating the new invariant.
  const targetByRoom = new Map<string, string>();
  for (let i = 0; i < groups.length; i++) {
    for (const roomId of groups[i].roomIds) targetByRoom.set(roomId, resolvedIds[i]);
  }
  if (resolvedIds.length > 0) {
    const fallbackId = resolvedIds[0];
    for (const g of currentById.values()) {
      for (const roomId of g.roomIds) {
        if (!targetByRoom.has(roomId)) targetByRoom.set(roomId, fallbackId);
      }
    }
  }
  for (const [roomId, targetId] of targetByRoom) {
    let currentGroup = '';
    for (const g of currentById.values()) {
      if (g.roomIds.includes(roomId)) {
        currentGroup = g.id;
        break;
      }
    }
    if (currentGroup === targetId) continue;
    await connectPost(page, 'chatto.admin.v1.AdminRoomLayoutService/MoveRoomToGroup', {
      roomId,
      groupId: targetId
    });
  }

  // Reorder rooms inside each desired group so the final sequence
  // matches the input. Read fresh state to confirm the moves landed
  // before validating room sets.
  const refreshed = await getRoomLayoutViaAPI(page);
  const refreshedRooms = new Map<string, string[]>();
  for (const g of refreshed?.groups ?? []) {
    refreshedRooms.set(
      g.id,
      g.rooms.map((r) => r.id)
    );
  }
  for (let i = 0; i < groups.length; i++) {
    const targetId = resolvedIds[i];
    const desired = groups[i].roomIds;
    const after = refreshedRooms.get(targetId) ?? [];
    const same = desired.length === after.length && desired.every((id, j) => id === after[j]);
    if (same) continue;
    await connectPost(page, 'chatto.admin.v1.AdminRoomLayoutService/ReorderSidebarItemsInGroup', {
      groupId: targetId,
      items: desired.map((id) => ({ kind: 'ADMIN_ROOM_LAYOUT_ITEM_KIND_ROOM', id }))
    });
  }

  // Drop any pre-existing group that isn't part of the desired layout.
  // Delete requires the group be empty — moves above already evicted
  // every targeted room, but a leftover group may still hold rooms the
  // test didn't enumerate (defensive: leave those groups in place).
  const desiredSet = new Set(resolvedIds);
  for (const g of currentById.values()) {
    if (desiredSet.has(g.id)) continue;
    const fresh = refreshedRooms.get(g.id) ?? [];
    if (fresh.length > 0) continue;
    const response = await connectPost<{ deleted?: boolean }>(
      page,
      'chatto.admin.v1.AdminRoomLayoutService/DeleteRoomGroup',
      { groupId: g.id }
    );
    expect(response.deleted).toBe(true);
  }

  // Finally, force the layout's group order to match the input.
  if (resolvedIds.length > 1) {
    await connectPost(page, 'chatto.admin.v1.AdminRoomLayoutService/ReorderRoomGroups', {
      orderedGroupIds: resolvedIds
    });
  }
}

async function getRoomLayoutViaAPI(page: Page): Promise<{
  groups: { id: string; name: string; rooms: { id: string; name: string; archived: boolean }[] }[];
} | null> {
  const data = await connectPost<AdminRoomLayoutResponse>(
    page,
    'chatto.admin.v1.AdminRoomLayoutService/ListAdminRoomLayout'
  );
  return {
    groups: (data.groups ?? []).map((group) => ({
      id: group.id ?? '',
      name: group.name ?? '',
      rooms: (group.rooms ?? []).map((room) => ({
        id: room.id ?? '',
        name: room.name ?? '',
        archived: room.archived ?? false
      }))
    }))
  };
}

/**
 * Returns the ID of the first (seed) room group. Every server boots with a
 * "Lobby" group after #454; tests need its ID to construct layouts that
 * include the auto-created announcements/general rooms.
 */
async function getSeedSetId(page: Page): Promise<string> {
  const layout = await getRoomLayoutViaAPI(page);
  if (!layout || layout.groups.length === 0) {
    throw new Error('Expected the seed room group to exist');
  }
  return layout.groups[0].id;
}

async function archiveRoomViaAPI(page: Page, roomId: string): Promise<void> {
  const data = await connectPost<{ room?: { archived?: boolean } }>(
    page,
    'chatto.api.v1.RoomService/ArchiveRoom',
    { roomId }
  );
  expect(data.room?.archived).toBe(true);
}

async function unarchiveRoomViaAPI(page: Page, roomId: string): Promise<void> {
  const data = await connectPost<{ room?: { archived?: boolean } }>(
    page,
    'chatto.api.v1.RoomService/UnarchiveRoom',
    { roomId }
  );
  expect(data.room).toBeTruthy();
  expect(data.room?.archived ?? false).toBe(false);
}

/** Returns IDs of both default rooms (announcements, general) created for the server. */
async function getDefaultRoomIds(
  page: Page
): Promise<{ announcementsId: string; generalId: string }> {
  const layout = await getRoomLayoutViaAPI(page);
  const rooms = layout?.groups.flatMap((group) => group.rooms) ?? [];
  const gen = rooms.find((r) => r.name === 'general');
  const ann = rooms.find((r) => r.name === 'announcements');
  if (!gen) throw new Error('Default "general" room not found');
  if (!ann) throw new Error('Default "announcements" room not found');
  return { announcementsId: ann.id, generalId: gen.id };
}

async function getRoomArchivedViaAPI(page: Page, roomId: string): Promise<boolean> {
  const layout = await getRoomLayoutViaAPI(page);
  const room = layout?.groups.flatMap((group) => group.rooms).find((entry) => entry.id === roomId);
  if (!room) throw new Error(`Room "${roomId}" not found in admin layout`);
  return room.archived ?? false;
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
      // with one seed "Lobby" group containing the auto-created announcements
      // and general rooms; subsequently-created rooms are appended in the
      // order they were created. There is no "no layout" / alphabetical
      // fallback anymore.
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);

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
      await usePrimaryServerViaAPI(page);

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
      // User A (owner) loads the server and configures layout
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const secretId = await createRoomViaAPI(page, 'secret');
      await denyRoomPermissionViaAPI(page, secretId, 'everyone', 'room.list');
      const seedSetId = await getSeedSetId(page);

      // Reshape: "Public" set holds the default rooms, "Secret" holds secret.
      await updateRoomLayoutViaAPI(page, [
        { id: seedSetId, name: 'Public', roomIds: [announcementsId, generalId] },
        { id: 'sec-secret', name: 'Secret', roomIds: [secretId] }
      ]);

      // User B joins the server — implicit membership in the default global
      // rooms (announcements, general), but not in secret.
      await withServerUser(browser!, serverURL, async ({ page: page2 }) => {
        await navigateToSpace(page2);

        // User B should only see the "Public" set, not "Secret" (empty for them
        // because room.list is denied on the only room inside it).
        const headers = await waitForSidebarSets(page2, 1);
        expect(headers).toEqual(['Public']);

        const roomNames = await waitForSidebarRooms(page2, 2);
        expect(roomNames).toEqual(['announcements', 'general']);
      });
    });

    test('set collapse/expand persists across navigation', async ({ page }) => {
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);

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
      // User A (owner) loads the server and rooms
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');

      await joinRoomViaAPI(page, alphaId);

      // User B opens the server
      await withServerUser(browser!, serverURL, async ({ page: page2 }) => {
        await joinRoomViaAPI(page2, alphaId);

        // User B navigates to the server — rooms render under the seed "Lobby" group.
        await navigateToSpace(page2);
        await waitForSidebarRooms(page2, 3); // announcements + general + alpha
        const headersBefore = await waitForSidebarSets(page2, 1);
        expect(headersBefore).toEqual(['Lobby']);

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
      });
    });
  });

  test.describe('API & Permissions', () => {
    test('admin can configure room layout via API', async ({ page }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      const alphaId = await createRoomViaAPI(page, 'alpha');
      const bravoId = await createRoomViaAPI(page, 'bravo');

      // Owner must join rooms to see them in layout query (rooms are filtered by membership)
      await joinRoomViaAPI(page, alphaId);
      await joinRoomViaAPI(page, bravoId);

      // Reshape the seed set to a single named "Section One". The per-key
      // model requires every channel room to live in some group, so the
      // input enumerates the full membership rather than just the rooms
      // the test cares about.
      const seedSetId = await getSeedSetId(page);
      await updateRoomLayoutViaAPI(page, [
        {
          id: seedSetId,
          name: 'Section One',
          roomIds: [bravoId, alphaId, generalId, announcementsId]
        }
      ]);

      // Query it back
      const layout = await getRoomLayoutViaAPI(page);
      expect(layout).not.toBeNull();
      expect(layout!.groups).toHaveLength(1);
      expect(layout!.groups[0].name).toBe('Section One');
      expect(layout!.groups[0].rooms.map((r) => r.id)).toEqual([
        bravoId,
        alphaId,
        generalId,
        announcementsId
      ]);
    });

    test('regular member cannot update layout (permission denied)', async ({
      page,
      browser,
      serverURL
    }) => {
      // User A (owner) loads the server
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);
      const { generalId } = await getDefaultRoomIds(page);

      // User B joins as regular member
      await withServerUser(browser!, serverURL, async ({ page: page2 }) => {
        // User B tries to mutate the room layout — should fail. Hits
        // createRoomGroup since it shares the role.manage gate with every
        // other layout mutator (the old bulk updateRoomGroups was retired
        // when storage was split per ADR-031).
        void generalId;
        const resp = await connectPostResponse(
          page2,
          'chatto.admin.v1.AdminRoomLayoutService/CreateRoomGroup',
          { name: 'Hacked' }
        );

        const body = await resp.text();
        expect(resp.status(), body).toBe(403);
        expect(body).toContain('permission denied');
      });
    });

    test('regular member does not see Rooms nav item in server admin', async ({
      page,
      browser,
      serverURL
    }) => {
      // User A (owner) loads the server
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);

      // User B joins as regular member
      await withServerUser(browser!, serverURL, async ({ page: page2 }) => {
        // Navigate to Rooms directly — User B should be denied.
        await page2.goto(routes.serverAdminRooms);
        await expect(page2.getByText('Access Denied', { exact: true })).toBeVisible();

        // User B shouldn't see the Rooms nav item (requires room.manage)
        const serverAdminPage2 = new ServerAdminPage(page2);
        await expect(serverAdminPage2.roomsNavItem).not.toBeVisible();
      });
    });
  });

  test.describe('Admin UI', () => {
    test('admin can navigate to rooms page and see layout editor', async ({
      page,
      serverAdminPage,
      serverAdminRoomsPage
    }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);

      // Navigate to server admin
      await serverAdminPage.goto(space.id);

      // Click Rooms nav item
      await expect(serverAdminPage.roomsNavItem).toBeVisible();
      await serverAdminPage.roomsNavItem.click();

      // Should see the rooms admin page with action buttons and default rooms
      await serverAdminRoomsPage.expectVisible();
      await serverAdminRoomsPage.expectRoomVisible('general');
    });

    test('admin can create, rename, and delete sections', async ({
      page,
      serverAdminRoomsPage
    }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);

      await serverAdminRoomsPage.goto(space.id);

      // Create a section (the seed "Lobby" group is also present)
      await serverAdminRoomsPage.createGroup('My Section');
      await serverAdminRoomsPage.expectGroupVisible('My Section');

      // Rename the section
      await serverAdminRoomsPage.renameGroup('My Section', 'Renamed Section');
      await serverAdminRoomsPage.expectGroupVisible('Renamed Section');

      // Delete the section
      await serverAdminRoomsPage.deleteGroup('Renamed Section');
      await serverAdminRoomsPage.expectGroupNotVisible('Renamed Section');
    });

    test('layout auto-saves and persists', async ({ page, serverAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);

      // Create extra rooms
      await createRoomViaAPI(page, 'alpha');
      await createRoomViaAPI(page, 'bravo');

      await serverAdminRoomsPage.goto(space.id);

      // Create a section
      await serverAdminRoomsPage.createGroup('Important');
      await serverAdminRoomsPage.expectGroupVisible('Important');

      // Verify layout auto-saves (poll API until it appears)
      await expect(async () => {
        const layout = await getRoomLayoutViaAPI(page);
        expect(layout).not.toBeNull();
        // The original seed set + the new "Important" set = 2 sets.
        const names = layout!.groups.map((s) => s.name);
        expect(names).toContain('Important');
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [250, 500, 1000] });
    });
  });

  test.describe('Edge Cases', () => {
    test('listable rooms user has not joined are shown faded in sets', async ({
      page,
      browser,
      serverURL
    }) => {
      // User A loads the server and creates extra rooms
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);

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

      // User B opens the server and joins only the public room (plus default announcements + general)
      await withServerUser(browser!, serverURL, async ({ page: page2 }) => {
        await joinRoomViaAPI(page2, publicId);

        await navigateToSpace(page2);

        // User B should see announcements, general, joined public, and listable
        // non-member private. Non-member channel rows use a leading + affordance.
        const roomNames = await waitForSidebarRooms(page2, 4);
        expect(roomNames).toContain('announcements');
        expect(roomNames).toContain('general');
        expect(roomNames).toContain('public');
        expect(roomNames).toContain('private');
        await expect(page2.getByRole('link', { name: '+ private' })).toHaveClass(/opacity-60/);
      });
    });
  });

  test.describe('Archiving', () => {
    test('admin can archive a room via admin UI', async ({ page, serverAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'to-archive');
      await joinRoomViaAPI(page, roomId);

      await serverAdminRoomsPage.goto(space.id);

      // Archive the room via UI (click Archive, confirm dialog)
      await serverAdminRoomsPage.archiveRoom('to-archive');

      // Room stays in its set (archive only flips the archived flag) but
      // its row now shows the Unarchive affordance instead of Archive.
      await expect(async () => {
        await serverAdminRoomsPage.expectRoomVisible('to-archive');
        const layout = await getRoomLayoutViaAPI(page);
        if (layout) {
          const allRoomIds = layout.groups.flatMap((s) => s.rooms.map((r) => r.id));
          expect(allRoomIds).toContain(roomId);
        }
        await expect(
          serverAdminRoomsPage.roomRow('to-archive').getByTitle('Unarchive room')
        ).toBeVisible();
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    });

    test('admin can unarchive a room via admin UI', async ({ page, serverAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'was-archived');
      await joinRoomViaAPI(page, roomId);

      // Archive via API first
      await archiveRoomViaAPI(page, roomId);

      await serverAdminRoomsPage.goto(space.id);

      // Unarchive the room via UI
      await serverAdminRoomsPage.unarchiveRoom('was-archived');

      // Room should be unarchived via API
      await expect(async () => {
        expect(await getRoomArchivedViaAPI(page, roomId)).toBe(false);
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    });

    test('cancel archive dialog keeps room in place', async ({ page, serverAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'stay-put');

      await serverAdminRoomsPage.goto(space.id);

      // Click Archive but cancel the dialog
      await serverAdminRoomsPage.clickArchive('stay-put');
      await serverAdminRoomsPage.cancelDialog();

      // Room should still be non-archived — verify via API
      expect(await getRoomArchivedViaAPI(page, roomId)).toBe(false);
    });

    test('archived room disappears from member sidebar', async ({ page, browser, serverURL }) => {
      // User A (owner) loads the server and rooms
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'will-vanish');
      await joinRoomViaAPI(page, roomId);

      // User B opens the server and enters the room
      await withServerUser(browser!, serverURL, async ({ page: page2 }) => {
        await joinRoomViaAPI(page2, roomId);

        // User B navigates to the server and sees the room
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
      });
    });

    test('archived room excluded from Browse Rooms', async ({ page, browser, serverURL }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);
      const visibleId = await createRoomViaAPI(page, 'visible-room');
      const hiddenId = await createRoomViaAPI(page, 'hidden-room');
      await joinRoomViaAPI(page, visibleId);
      await joinRoomViaAPI(page, hiddenId);

      // Archive one room
      await archiveRoomViaAPI(page, hiddenId);

      // User B opens the server
      await withServerUser(browser!, serverURL, async ({ page: page2 }) => {
        // Navigate to the Overview page (where the room directory now lives)
        await page2.goto(routes.browseRooms);
        await expect(page2.getByRole('heading', { name: 'Overview' })).toBeVisible();

        // The non-archived room should be visible in the directory (not yet
        // joined by User B). Scope to the directory list because the sidebar
        // also shows listable non-member rooms now.
        await expect(page2.getByRole('list').getByText('visible-room')).toBeVisible();

        // The archived room should NOT be visible
        await expect(page2.getByText('hidden-room')).not.toBeVisible();
      });
    });

    test('unarchived room reappears in member sidebar', async ({ page, browser, serverURL }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);
      const roomId = await createRoomViaAPI(page, 'comeback');
      await joinRoomViaAPI(page, roomId);

      // User B opens the server and enters the room, then room gets archived
      await withServerUser(browser!, serverURL, async ({ page: page2 }) => {
        await joinRoomViaAPI(page2, roomId);

        // Archive the room
        await archiveRoomViaAPI(page, roomId);

        // User B navigates to the server — room should not be visible
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
      });
    });
  });

  test.describe('Admin Room Management', () => {
    test('admin can edit room name and description', async ({ page, serverAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);
      await createRoomViaAPI(page, 'old-name');

      await serverAdminRoomsPage.goto(space.id);

      // Edit the room
      await serverAdminRoomsPage.editRoom('old-name', 'new-name', 'A shiny new description');

      // Should see updated name in the list
      await expect(async () => {
        await serverAdminRoomsPage.expectRoomVisible('new-name');
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    });

    test('admin can create a room from admin page', async ({ page, serverAdminRoomsPage }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);

      await serverAdminRoomsPage.goto(space.id);

      // Create a room from the seed "Lobby" group's header.
      await serverAdminRoomsPage.createRoom('Lobby', 'fresh-room');

      // Room should appear in admin page
      await serverAdminRoomsPage.expectRoomVisible('fresh-room', TIMEOUTS.UI_STANDARD);
    });

    test('admin can create a room in a non-seed set', async ({ page, serverAdminRoomsPage }) => {
      // Regression: previously, creating a room from a set other than the
      // seed "Lobby" group silently dropped the groupId or the room didn't
      // appear after refetch. Verify the room lands in the chosen set.
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);

      // Pre-create a second set via API so we don't race the autosave.
      // The per-key delta path generates real group IDs server-side, so
      // the assertions below look up groups by name instead of a
      // pre-allocated client-side ID.
      const seedSetId = await getSeedSetId(page);
      const { generalId, announcementsId } = await getDefaultRoomIds(page);
      await updateRoomLayoutViaAPI(page, [
        { id: seedSetId, name: 'Lobby', roomIds: [generalId, announcementsId] },
        { id: 'projects-placeholder', name: 'Projects', roomIds: [] }
      ]);

      await serverAdminRoomsPage.goto(space.id);
      await serverAdminRoomsPage.expectGroupVisible('Projects');

      // Create a room from the "Projects" set's header.
      await serverAdminRoomsPage.createRoom('Projects', 'project-room');

      // Room must show up in the admin layout, inside the Projects set.
      await serverAdminRoomsPage.expectRoomVisible('project-room', TIMEOUTS.UI_STANDARD);
      await expect(async () => {
        const layout = await getRoomLayoutViaAPI(page);
        expect(layout).not.toBeNull();
        const projects = layout!.groups.find((s) => s.name === 'Projects');
        expect(projects).toBeTruthy();
        expect(projects!.rooms.length).toBe(1);
        // And the seed "Lobby" group is unchanged.
        const rooms = layout!.groups.find((s) => s.id === seedSetId);
        expect(rooms!.rooms.length).toBe(2);
      }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [100, 250, 500, 1000] });
    });

    test('delete button is disabled while a set still has rooms', async ({
      page,
      serverAdminRoomsPage
    }) => {
      await createAndLoginTestUser(page);
      const space = await usePrimaryServerViaAPI(page);

      const { generalId, announcementsId } = await getDefaultRoomIds(page);

      const seedSetId = await getSeedSetId(page);
      await updateRoomLayoutViaAPI(page, [
        {
          id: seedSetId,
          name: 'Has Rooms',
          roomIds: [generalId, announcementsId]
        }
      ]);

      await serverAdminRoomsPage.goto(space.id);
      await serverAdminRoomsPage.expectGroupVisible('Has Rooms');

      // With Unsorted gone, deletion of a non-empty set would orphan the
      // contents — so the Delete button is disabled until they're moved out.
      const deleteBtn = serverAdminRoomsPage
        .groupHeaderRow('Has Rooms')
        .getByTitle('Move all items out of this group before deleting');
      await expect(deleteBtn).toBeVisible();
      await expect(deleteBtn).toBeDisabled();
    });
  });

  test.describe('Overview — Join all (group)', () => {
    test('one click joins every joinable room in the group', async ({
      page,
      browser,
      serverURL
    }) => {
      // Admin owns the server and creates three new rooms (which land in
      // the seed "Lobby" group alongside the bootstrap rooms).
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);
      await createRoomViaAPI(page, 'alpha');
      await createRoomViaAPI(page, 'bravo');
      await createRoomViaAPI(page, 'charlie');

      // User B shows up empty-handed — no auto-join, no rooms in their
      // sidebar yet.
      await withServerUser(
        browser!,
        serverURL,
        async ({ page: page2 }) => {
          // Go to the server Overview (which hosts the room directory).
          await page2.goto(routes.browseRooms);
          await expect(page2.getByRole('heading', { name: 'Overview' })).toBeVisible({
            timeout: TIMEOUTS.UI_STANDARD
          });

          // Click the group's "Join all" button.
          const joinAll = page2.getByRole('button', { name: 'Join all' }).first();
          await expect(joinAll).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
          await joinAll.click();
          // Move the cursor off the group card so no row stays in :hover
          // (which would swap a freshly-joined row's button label from
          // "Joined" to "Leave" and break the regex below).
          await page2.mouse.move(0, 0);

          // After the bulk join finishes, the rows for all three rooms
          // should render the "Joined" pill in the directory. The
          // button's accessible name resolves to its visible text
          // ("Joined" when off-hover, "Leave" on hover); we matched the
          // hover away above so the off-hover label is stable.
          for (const name of ['alpha', 'bravo', 'charlie']) {
            const row = page2.locator('li', { hasText: `# ${name}` });
            await expect(row.getByRole('button', { name: 'Joined' })).toBeVisible({
              timeout: TIMEOUTS.REALTIME_EVENT
            });
          }

          // And the rooms now appear in the sidebar (alongside the
          // bootstrap rooms, which "Join all" also joined since they
          // share the group). The seed "Lobby" group has 5 rooms total:
          // announcements, general, alpha, bravo, charlie.
          await navigateToSpace(page2);
          await expect(async () => {
            const roomNames = await waitForSidebarRooms(page2, 5);
            expect(roomNames).toEqual(expect.arrayContaining(['alpha', 'bravo', 'charlie']));
          }).toPass({
            timeout: TIMEOUTS.REALTIME_EVENT,
            intervals: [500, 1000, 2000]
          });
        },
        { userOptions: { skipDefaultRooms: true } }
      );
    });
  });
});
