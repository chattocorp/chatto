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

// FIXME #330: relies on createSpaceViaAPI; see space-admin-members.test.ts.
test.describe.skip('Space navigation race condition fix', () => {

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
