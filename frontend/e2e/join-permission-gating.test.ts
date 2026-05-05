import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  loginAsAdmin,
  verifyAdminEmail,
  denyUserInstancePermission
} from './fixtures/testUser';
import { ExplorePage } from './pages/ExplorePage';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

// ============================================================================
// GraphQL Helper Functions
// ============================================================================

async function createSpaceViaAPI(page: Page, name?: string): Promise<{ id: string; name: string }> {
  const spaceName = name ?? `Join Test Space ${Date.now()}`;
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: CreateSpaceInput!) { createSpace(input: $input) { id name } }`,
      variables: { input: { name: spaceName, description: 'Join permission test space' } }
    }
  });
  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.createSpace).toBeTruthy();
  return { id: data.data.createSpace.id, name: data.data.createSpace.name };
}

async function createRoomViaAPI(page: Page, spaceId: string, name?: string): Promise<string> {
  const roomName = name ?? `room${Date.now()}`;
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: CreateRoomInput!) { createRoom(input: $input) { id name } }`,
      variables: { input: { spaceId, name: roomName } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  const data = await resp.json();
  if (data.errors || !data.data?.createRoom) {
    throw new Error(`createRoom failed: ${JSON.stringify(data)}`);
  }
  return data.data.createRoom.id;
}

async function joinSpaceViaAPI(page: Page, spaceId: string): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: JoinSpaceInput!) { joinSpace(input: $input) }`,
      variables: { input: { spaceId } }
    }
  });
  expect(resp.ok()).toBeTruthy();
}

async function loginUser(page: Page, login: string, password: string): Promise<void> {
  const resp = await page.request.post('/auth/login', { data: { login, password } });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).success).toBe(true);
}

async function logoutUser(page: Page): Promise<void> {
  await page.request.post('/auth/logout');
}

async function denySpacePermission(
  page: Page,
  spaceId: string,
  role: string,
  permission: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: DenySpacePermissionInput!) { denySpacePermission(input: $input) }`,
      variables: { input: { spaceId, role, permission } }
    }
  });
  expect(resp.ok()).toBeTruthy();
  expect((await resp.json()).data?.denySpacePermission).toBe(true);
}

// ============================================================================
// Test Scenarios
// ============================================================================

// FIXME #330: relies on createSpaceViaAPI; see space-admin-members.test.ts.
test.describe.skip('Join Permission Gating', () => {
  test.describe('Browse Spaces - space.join permission', () => {
    test('user with denied space.join permission sees "No permission to join"', async ({
      page,
      browser,
      serverURL
    }) => {
      // Login as admin to create space and deny permission
      const admin = await loginAsAdmin(page);
      await verifyAdminEmail(page, admin.id!);
      const space = await createSpaceViaAPI(page, `Denied Join Space ${Date.now()}`);

      // Create a second context for the test user
      const context2 = await browser!.newContext({ baseURL: serverURL });
      const page2 = await context2.newPage();

      try {
        // Create a verified user in the second context
        const testUser = await createAndLoginTestUser(page2);

        // As admin, deny space.join permission for this user
        const denyRoleName = await denyUserInstancePermission(page, testUser.id!, 'space.join');
        expect(denyRoleName).toBeTruthy();

        // Refresh the explore page to pick up new permissions
        const explorePage = new ExplorePage(page2);
        await explorePage.goto();

        // The space should be visible
        const spaceCard = explorePage.getSpaceItem(space.name);
        await expect(spaceCard).toBeVisible();

        // But the Join button should NOT be visible
        await expect(spaceCard.getByRole('button', { name: 'Join' })).not.toBeVisible();

        // Instead, should show "No permission to join" text
        await expect(spaceCard.getByText('No permission to join')).toBeVisible();
      } finally {
        await context2.close();
      }
    });

    test('verified user WITH space.join permission sees Join button', async ({ page }) => {
      // Create a space as one user
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page, `Joinable Space ${Date.now()}`);
      await logoutUser(page);

      // Login as another verified user
      await createAndLoginTestUser(page);

      // Navigate to Browse Spaces
      const explorePage = new ExplorePage(page);
      await explorePage.goto();

      // The space should be visible
      const spaceCard = explorePage.getSpaceItem(space.name);
      await expect(spaceCard).toBeVisible();

      // The Join button SHOULD be visible
      await expect(spaceCard.getByRole('button', { name: 'Join', exact: true })).toBeVisible();

      // Should NOT show "No permission" text
      await expect(spaceCard.getByText('No permission to join')).not.toBeVisible();
    });
  });

  test.describe('Browse Rooms - room.join permission', () => {
    test('user with denied room.join permission sees "No permission" instead of Join button', async ({
      page
    }) => {
      // Create space and room as admin
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomName = `gated-room-${Date.now()}`;
      await createRoomViaAPI(page, space.id, roomName);

      // Deny room.join for everyone role (affects all members except higher-ranked roles)
      // The space creator has 'owner' role which outranks 'everyone', so they're not affected
      await denySpacePermission(page, space.id, 'everyone', 'room.join');

      // Create a second user who will join the space
      const member = await createAndLoginTestUser(page, { loginPrefix: 'member' });
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to Browse Rooms
      await page.goto(routes.browseRooms);

      // The page should load (user has room.list permission)
      await expect(page.getByRole('heading', { name: 'Browse Rooms' })).toBeVisible();

      // Find the room in the list
      const roomItem = page.locator('li', { hasText: `# ${roomName}` });
      await expect(roomItem).toBeVisible();

      // The Join button should NOT be visible (room.join is denied)
      await expect(roomItem.getByRole('button', { name: 'Join' })).not.toBeVisible();

      // Should show "No permission" text instead
      await expect(roomItem.getByText('No permission')).toBeVisible();
    });

    test('user WITH room.join permission sees Join button', async ({ page }) => {
      // Create space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomName = `joinable-room-${Date.now()}`;
      await createRoomViaAPI(page, space.id, roomName);

      // Create a second user who will join the space (room.join is granted by default)
      const member = await createAndLoginTestUser(page, { loginPrefix: 'member' });
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to Browse Rooms
      await page.goto(routes.browseRooms);

      // The page should load
      await expect(page.getByRole('heading', { name: 'Browse Rooms' })).toBeVisible();

      // Find the room in the list
      const roomItem = page.locator('li', { hasText: `# ${roomName}` });
      await expect(roomItem).toBeVisible();

      // The "Join" text SHOULD be visible (button accessible name includes room name)
      await expect(roomItem.getByText('Join', { exact: true })).toBeVisible();

      // Should NOT show "No permission" text
      await expect(roomItem.getByText('No permission')).not.toBeVisible();
    });

    test('clicking Join button works when permission is granted', async ({ page }) => {
      // Create space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomName = `clickable-room-${Date.now()}`;
      await createRoomViaAPI(page, space.id, roomName);

      // Create a second user who will join the space
      const member = await createAndLoginTestUser(page, { loginPrefix: 'member' });
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to Browse Rooms
      await page.goto(routes.browseRooms);

      // Find and click the Join button
      const roomItem = page.locator('li', { hasText: `# ${roomName}` });
      await roomItem.getByRole('button', { name: 'Join' }).click();

      // Should stay on Browse Rooms page and show "Joined" state
      await expect(page.getByRole('heading', { name: 'Browse Rooms' })).toBeVisible();
      await expect(roomItem.getByText('Joined')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    });
  });
});
