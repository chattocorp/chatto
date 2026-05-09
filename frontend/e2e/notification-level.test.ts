import { expect } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  graphqlQuery,
  getRoomIdByName
} from './fixtures/graphqlHelpers';
import { test } from './setup';
import { TIMEOUTS } from './constants';

/**
 * Helper to set a space notification level via GraphQL mutation.
 */
async function setSpaceNotificationLevel(
  page: import('@playwright/test').Page,
  spaceId: string,
  level: string
): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: SetSpaceNotificationLevelInput!) {
				setSpaceNotificationLevel(input: $input) {
					level effectiveLevel
				}
			}`,
      variables: { input: { spaceId, level } }
    }
  });
  expect(response.ok()).toBeTruthy();
}

/**
 * Helper to set a room notification level via GraphQL mutation.
 */
async function setRoomNotificationLevel(
  page: import('@playwright/test').Page,
  spaceId: string,
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
      variables: { input: { spaceId, roomId, level } }
    }
  });
  expect(response.ok()).toBeTruthy();
}

test.describe('Notification Level - Preferences Page', () => {
  test('preferences page renders with space-level and room sections', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Test Space');

    // Navigate to preferences page
    await page.getByRole('link', { name: 'Preferences' }).click();
    await page.waitForURL(/\/preferences/);

    // Verify page heading
    await expect(page.getByRole('heading', { name: 'Preferences' })).toBeVisible();

    // Verify space notification level section
    await expect(page.getByText('Space Notification Level')).toBeVisible();

    // Verify the three space-level option labels are visible
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

  test('can set space notification level via UI', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Test Space');

    // Navigate to preferences
    await page.getByRole('link', { name: 'Preferences' }).click();
    await page.waitForURL(/\/preferences/);

    // Normal should be selected by default (check for accent border on button)
    const normalButton = page.locator('button', { hasText: 'Normal' }).filter({
      hasText: 'Unread markers'
    });
    await expect(normalButton).toHaveClass(/border-accent/);

    // Click Muted button
    const mutedButton = page.locator('button', { hasText: 'Muted' }).filter({
      hasText: 'No notifications'
    });
    await mutedButton.click();

    // Wait for success toast
    await expect(page.getByText('Space notification level updated')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Verify Muted is now selected (has accent border)
    await expect(mutedButton).toHaveClass(/border-accent/);

    // Reload and verify persistence
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Preferences' })).toBeVisible();
    const mutedButtonReloaded = page.locator('button', { hasText: 'Muted' }).filter({
      hasText: 'No notifications'
    });
    await expect(mutedButtonReloaded).toHaveClass(/border-accent/);
  });

  test('can set room notification level via UI', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Test Space');

    // Navigate to preferences
    await page.getByRole('link', { name: 'Preferences' }).click();
    await page.waitForURL(/\/preferences/);

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
    await expect(page.getByRole('heading', { name: 'Preferences' })).toBeVisible();
    const generalRowAfterReload = page.getByTestId('room-notification-general');
    await expect(generalRowAfterReload.locator('select')).toHaveValue('MUTED');
  });

  test('preferences link is visible in space sidebar', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Test Space');

    // Verify Preferences link is visible in sidebar
    const preferencesLink = page.getByRole('link', { name: 'Preferences' });
    await expect(preferencesLink).toBeVisible();
  });
});


test.describe('Notification Level - Server-Side Enforcement', () => {
  test('setting notification level persists via GraphQL roundtrip', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('API Test');
    const spaceId = await chatPage.getSpaceId();

    // Set space level to MUTED via API
    await setSpaceNotificationLevel(page, spaceId, 'MUTED');

    // Query it back
    const data = await graphqlQuery<{
      instance: { viewerNotificationPreference: { level: string; effectiveLevel: string } };
    }>(
      page,
      `query { instance { viewerNotificationPreference { level effectiveLevel } } }`
    );

    expect(data.instance.viewerNotificationPreference.level).toBe('MUTED');
    expect(data.instance.viewerNotificationPreference.effectiveLevel).toBe('MUTED');
  });

  test('room inherits space notification level when set to DEFAULT', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Inherit Test');
    const spaceId = await chatPage.getSpaceId();
    const roomId = await getRoomIdByName(page, spaceId, 'general');

    // Set space level to MUTED
    await setSpaceNotificationLevel(page, spaceId, 'MUTED');

    // Room (with DEFAULT) should inherit MUTED from space
    const data = await graphqlQuery<{
      room: { viewerNotificationPreference: { level: string; effectiveLevel: string } };
    }>(
      page,
      `query($spaceId: ID!, $roomId: ID!) {
				room(spaceId: $spaceId, roomId: $roomId) {
					viewerNotificationPreference { level effectiveLevel }
				}
			}`,
      { spaceId, roomId }
    );

    expect(data.room.viewerNotificationPreference.level).toBe('DEFAULT');
    expect(data.room.viewerNotificationPreference.effectiveLevel).toBe('MUTED');
  });

  test('room level overrides space level', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Override Test');
    const spaceId = await chatPage.getSpaceId();
    const roomId = await getRoomIdByName(page, spaceId, 'general');

    // Set space level to MUTED
    await setSpaceNotificationLevel(page, spaceId, 'MUTED');

    // Set room level to ALL_MESSAGES (overrides space MUTED)
    await setRoomNotificationLevel(page, spaceId, roomId, 'ALL_MESSAGES');

    // Room should show ALL_MESSAGES as effective level
    const data = await graphqlQuery<{
      room: { viewerNotificationPreference: { level: string; effectiveLevel: string } };
    }>(
      page,
      `query($spaceId: ID!, $roomId: ID!) {
				room(spaceId: $spaceId, roomId: $roomId) {
					viewerNotificationPreference { level effectiveLevel }
				}
			}`,
      { spaceId, roomId }
    );

    expect(data.room.viewerNotificationPreference.level).toBe('ALL_MESSAGES');
    expect(data.room.viewerNotificationPreference.effectiveLevel).toBe('ALL_MESSAGES');
  });
});
