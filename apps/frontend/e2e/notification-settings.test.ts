import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import * as routes from './routes';

test.describe('Notification Settings', () => {
  test.use({ serverOptions: { env: { CHATTO_PUSH_ENABLED: 'false' } } });

  test('notification settings page is accessible from chat settings', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await page.goto(routes.settings);
    await page.locator('nav').getByRole('link', { name: 'Notifications', exact: true }).click();

    await page.waitForURL(routes.settingsNotifications);
    await expect(page.getByRole('heading', { name: 'Notifications', exact: true })).toBeVisible();
    await expect(page.getByText('Notification Sound')).toBeVisible();

    const pushSettings = page.getByTestId('push-notification-settings');
    await expect(
      pushSettings.getByText(
        'Push notifications are unavailable on this server. The server administrator has not configured them.'
      )
    ).toBeVisible();
    await expect(pushSettings.getByRole('checkbox')).toHaveCount(0);
    await expect(pushSettings.getByRole('button')).toHaveCount(0);
  });
});
