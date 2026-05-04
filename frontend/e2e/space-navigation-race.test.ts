import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { TIMEOUTS } from './constants';
import { loginAsAdmin, verifyAdminEmail } from './fixtures/testUser';
import * as routes from './routes';

interface TestSpace {
  id: string;
  name: string;
}

/**
 * Creates a space via GraphQL API.
 */
async function createSpaceViaAPI(page: Page, name: string): Promise<TestSpace> {
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation CreateSpace($input: CreateSpaceInput!) {
					createSpace(input: $input) {
						id
						name
					}
				}
			`,
      variables: { input: { name } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.createSpace).toBeTruthy();

  return {
    id: data.data.createSpace.id,
    name: data.data.createSpace.name
  };
}

/**
 * Creates a room in a space via GraphQL API and joins it.
 */
async function createRoomViaAPI(page: Page, spaceId: string, name: string): Promise<string> {
  // Create the room
  const createResponse = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation CreateRoom($input: CreateRoomInput!) {
					createRoom(input: $input) {
						id
					}
				}
			`,
      variables: { input: { spaceId, name } }
    }
  });

  expect(createResponse.ok()).toBeTruthy();
  const createData = await createResponse.json();
  expect(createData.data?.createRoom).toBeTruthy();

  const roomId = createData.data.createRoom.id;

  // Join the room
  const joinResponse = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation JoinRoom($input: JoinRoomInput!) {
					joinRoom(input: $input)
				}
			`,
      variables: { input: { spaceId, roomId } }
    }
  });

  expect(joinResponse.ok()).toBeTruthy();
  const joinData = await joinResponse.json();
  expect(joinData.data?.joinRoom).toBe(true);

  return roomId;
}

/**
 * Uploads a banner to a space via UI (General settings page).
 */
async function uploadBannerViaUI(page: Page, spaceId: string): Promise<void> {
  // Navigate to General settings page (where banner upload is)
  await page.goto(routes.serverAdminGeneral);
  await expect(page.locator('h1', { hasText: 'General' })).toBeVisible();

  // Create a minimal valid 1x1 red PNG
  const pngData = Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==',
    'base64'
  );

  // Upload banner via file chooser
  const fileChooserPromise = page.waitForEvent('filechooser');
  await page.getByRole('button', { name: /Upload Banner/ }).click();
  const fileChooser = await fileChooserPromise;
  await fileChooser.setFiles({
    name: 'test-banner.png',
    mimeType: 'image/png',
    buffer: pngData
  });

  // Wait for upload success
  await expect(page.getByText('Banner uploaded successfully')).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });
}

test.describe('Space navigation race condition fix', () => {
  // FIXME: navigates between rooms in two different spaces — multi-space,
  // doesn't apply post-collapse. Re-write in next phase-2 PR.
  test.skip('room views load correctly after navigating to admin and back from space with banner', async ({
    page,
    chatPage: _chatPage,
    adminPage
  }) => {
    // Login as admin (so we can access the admin panel)
    const adminUser = await loginAsAdmin(page);
    await verifyAdminEmail(page, adminUser.id!);

    // Create two spaces - one without banner, one with banner
    const spaceNoBanner = await createSpaceViaAPI(page, 'Space Without Banner');
    const spaceWithBanner = await createSpaceViaAPI(page, 'Space With Banner');

    // Create a room in each space
    const roomNoBannerId = await createRoomViaAPI(page, spaceNoBanner.id, 'test-room');
    const roomWithBannerId = await createRoomViaAPI(page, spaceWithBanner.id, 'test-room');

    // Upload a banner to the second space via UI
    await uploadBannerViaUI(page, spaceWithBanner.id);

    // Step 1: Navigate to room in space WITHOUT banner
    await page.goto(routes.room(roomNoBannerId));
    await expect(page.getByRole('heading', { name: '# test-room' })).toBeVisible();
    await expect(page.getByTestId('message-input')).toBeVisible();

    // Step 2: Navigate to admin
    await adminPage.goto();
    await adminPage.expectDashboardVisible();

    // Step 3: Navigate back to space without banner
    await page.goto(routes.room(roomNoBannerId));
    await expect(page.getByRole('heading', { name: '# test-room' })).toBeVisible();
    await expect(page.getByTestId('message-input')).toBeVisible();

    // Step 4: Navigate to room in space WITH banner
    await page.goto(routes.room(roomWithBannerId));
    await expect(page.getByRole('heading', { name: '# test-room' })).toBeVisible();
    await expect(page.getByTestId('message-input')).toBeVisible();

    // Verify banner is visible in sidebar
    await expect(page.locator('img[alt="Space banner"]')).toBeVisible();

    // Step 5: Navigate to admin
    await adminPage.goto();
    await adminPage.expectDashboardVisible();

    // Step 6: Navigate back to space WITH banner
    // This is the critical step - before the fix, this would fail to load room content
    await page.goto(routes.room(roomWithBannerId));
    await expect(page.getByRole('heading', { name: '# test-room' })).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await expect(page.getByTestId('message-input')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Step 7: Verify first space still works (cascading failure check)
    await page.goto(routes.room(roomNoBannerId));
    await expect(page.getByRole('heading', { name: '# test-room' })).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await expect(page.getByTestId('message-input')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
  });

  test('rapid navigation between spaces and admin does not break room loading', async ({
    page,
    adminPage
  }) => {
    // Login as admin
    const adminUser = await loginAsAdmin(page);
    await verifyAdminEmail(page, adminUser.id!);

    // Create a space with banner
    const space = await createSpaceViaAPI(page, 'Rapid Nav Test');
    const roomId = await createRoomViaAPI(page, space.id, 'test-room');
    await uploadBannerViaUI(page, space.id);

    // Rapidly navigate back and forth 5 times
    for (let i = 0; i < 5; i++) {
      await page.goto(routes.room(roomId));
      // Don't wait for full load, immediately go to admin
      await adminPage.goto();
    }

    // Final navigation - room should still load correctly
    await page.goto(routes.room(roomId));
    await expect(page.getByRole('heading', { name: '# test-room' })).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await expect(page.getByTestId('message-input')).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(page.locator('img[alt="Space banner"]')).toBeVisible();
  });
});
