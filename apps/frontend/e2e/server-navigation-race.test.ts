import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { TIMEOUTS } from './constants';
import {
  createRoomViaConnect,
  getDefaultRoomGroupIdViaConnect,
  joinRoomViaConnect
} from './fixtures/connectHelpers';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import * as routes from './routes';

interface TestServer {
  id: string;
  name: string;
}

/** Log in as the bootstrap admin and return the primary server metadata. */
async function usePrimaryServerViaAPI(page: Page, _name: string): Promise<TestServer> {
  return loginAsAdminAndUsePrimaryServer(page);
}

/** Creates a room via ConnectRPC API and joins it. */
async function createRoomViaAPI(page: Page, _spaceId: string, name: string): Promise<string> {
  const groupId = await getDefaultRoomGroupIdViaConnect(page);
  const roomId = await createRoomViaConnect(page, name, groupId);
  await joinRoomViaConnect(page, roomId);
  return roomId;
}

/** Uploads a server banner via UI (General settings page). */
async function uploadBannerViaUI(page: Page, _spaceId: string): Promise<void> {
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
  await expect(page.getByText('Banner uploaded successfully')).toBeVisible({
    timeout: TIMEOUTS.COMPLEX_OPERATION
  });
}

test.describe('Server navigation race condition fix', () => {
  test('rapid navigation between room and admin does not break room loading', async ({
    page,
    adminPage
  }) => {
    // Prepare the server with a banner and room.
    const space = await usePrimaryServerViaAPI(page, 'Rapid Nav Test');
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
    await expect(page.getByTestId('message-input')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await expect(page.locator('img[alt="Server banner"]')).toBeVisible();
  });
});
