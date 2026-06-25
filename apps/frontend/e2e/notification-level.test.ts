import { expect } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { graphqlQuery, getRoomIdByName } from './fixtures/graphqlHelpers';
import { test } from './setup';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

/**
 * Helper to set a server notification level via GraphQL mutation.
 */
async function setServerNotificationLevel(
  page: import('@playwright/test').Page,
  level: string
): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: SetServerNotificationLevelInput!) {
				setServerNotificationLevel(input: $input) {
					level effectiveLevel
				}
			}`,
      variables: { input: { level } }
    }
  });
  expect(response.ok()).toBeTruthy();
}

/**
 * Helper to set a room notification level via GraphQL mutation.
 */
async function setRoomNotificationLevel(
  page: import('@playwright/test').Page,
  roomId: string,
  level: string
): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: SetRoomNotificationLevelInput!) {
				setRoomNotificationLevel(input: $input) {
					level effectiveLevel
				}
			}`,
      variables: { input: { roomId, level } }
    }
  });
  expect(response.ok()).toBeTruthy();
}

test.describe('Notification Level - Notifications Settings', () => {
  test('notifications settings page renders with server-level and room sections', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to notification settings page
    await page.goto(routes.settingsNotifications);

    // Verify page heading
    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();

    // Verify server notification level section
    await expect(page.getByText('Server Notification Level')).toBeVisible();

    // Verify the three server-level option labels are visible
    await expect(page.getByText('No notifications or unread markers')).toBeVisible();
    await expect(
      page.getByText('Unread markers + mentions, DMs, and thread replies')
    ).toBeVisible();
    await expect(page.getByText('Normal + notification for every new message')).toBeVisible();

    // Verify room overrides section is visible
    await expect(page.getByText('Room Overrides')).toBeVisible();

    // The general room should be listed in the room overrides (use testid)
    await expect(page.getByTestId('room-notification-general')).toBeVisible();
  });

  test('can set server notification level via UI', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to notification settings
    await page.goto(routes.settingsNotifications);

    // Normal should be selected by default.
    const normalButton = page.locator('button', { hasText: 'Normal' }).filter({
      hasText: 'Unread markers'
    });
    await expect(normalButton).toHaveClass(/choice-row-selected/);

    // Click Muted button
    const mutedButton = page.locator('button', { hasText: 'Muted' }).filter({
      hasText: 'No notifications'
    });
    await mutedButton.click();

    // Wait for success toast
    await expect(page.getByText('Server notification level updated')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Verify Muted is now selected.
    await expect(mutedButton).toHaveClass(/choice-row-selected/);

    // Reload and verify persistence
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();
    const mutedButtonReloaded = page.locator('button', { hasText: 'Muted' }).filter({
      hasText: 'No notifications'
    });
    await expect(mutedButtonReloaded).toHaveClass(/choice-row-selected/);
  });

  test('can set room notification level via UI', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Navigate to notification settings
    await page.goto(routes.settingsNotifications);

    // Find the room override row for "general" and change its select
    const generalRow = page.getByTestId('room-notification-general');
    const select = generalRow.locator('select');

    // Default should be selected initially
    await expect(select).toHaveValue('DEFAULT');

    // Change to MUTED
    await select.selectOption('MUTED');

    // Wait for success toast
    await expect(page.getByText('Room notification level updated')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Verify it persists after reload
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();
    const generalRowAfterReload = page.getByTestId('room-notification-general');
    await expect(generalRowAfterReload.locator('select')).toHaveValue('MUTED');
  });

  test('notification levels are available from settings sidebar', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await page.goto(routes.settings);

    const notificationsLink = page
      .locator('nav')
      .getByRole('link', { name: 'Notifications', exact: true });
    await expect(notificationsLink).toBeVisible();
    await notificationsLink.click();

    await page.waitForURL(routes.settingsNotifications);
    await expect(page.getByText('Server Notification Level')).toBeVisible();
  });
});

test.describe('Notification Level - Server-Side Enforcement', () => {
  test('setting notification level persists via GraphQL roundtrip', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Set server level to MUTED via API
    await setServerNotificationLevel(page, 'MUTED');

    // Query it back
    const data = await graphqlQuery<{
      server: { viewerNotificationPreference: { level: string; effectiveLevel: string } };
    }>(page, `query { server { viewerNotificationPreference { level effectiveLevel } } }`);

    expect(data.server.viewerNotificationPreference.level).toBe('MUTED');
    expect(data.server.viewerNotificationPreference.effectiveLevel).toBe('MUTED');
  });

  test('room inherits server notification level when set to DEFAULT', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomId = await getRoomIdByName(page, 'general');

    // Set server level to MUTED
    await setServerNotificationLevel(page, 'MUTED');

    // Room (with DEFAULT) should inherit MUTED from server
    const data = await graphqlQuery<{
      room: { viewerNotificationPreference: { level: string; effectiveLevel: string } };
    }>(
      page,
      `query($roomId: ID!) { room(roomId: $roomId) {
					viewerNotificationPreference { level effectiveLevel }
				}
			}`,
      { roomId }
    );

    expect(data.room.viewerNotificationPreference.level).toBe('DEFAULT');
    expect(data.room.viewerNotificationPreference.effectiveLevel).toBe('MUTED');
  });

  test('room level overrides server level', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomId = await getRoomIdByName(page, 'general');

    // Set server level to MUTED
    await setServerNotificationLevel(page, 'MUTED');

    // Set room level to ALL_MESSAGES (overrides server MUTED)
    await setRoomNotificationLevel(page, roomId, 'ALL_MESSAGES');

    // Room should show ALL_MESSAGES as effective level
    const data = await graphqlQuery<{
      room: { viewerNotificationPreference: { level: string; effectiveLevel: string } };
    }>(
      page,
      `query($roomId: ID!) { room(roomId: $roomId) {
					viewerNotificationPreference { level effectiveLevel }
				}
			}`,
      { roomId }
    );

    expect(data.room.viewerNotificationPreference.level).toBe('ALL_MESSAGES');
    expect(data.room.viewerNotificationPreference.effectiveLevel).toBe('ALL_MESSAGES');
  });
});
