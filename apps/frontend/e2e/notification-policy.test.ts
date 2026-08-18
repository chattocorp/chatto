import { expect } from '@playwright/test';
import { NotificationDeliveryMode } from '@chatto/api-types/api/v1/notifications_pb';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  getNotificationPolicy,
  getRoomIdByNameViaConnect,
  setNotificationPolicyPreference
} from './fixtures/connectHelpers';
import { test } from './setup';
import * as routes from './routes';

test.describe('Notification policy', () => {
  test('renders every supported cause and persists a server override', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await page.goto(routes.settingsNotifications);

    await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible();
    await expect(page.getByText('Notification policy')).toBeVisible();

    const directMessages = page.getByRole('combobox', { name: 'Direct messages' });
    await expect(directMessages).toBeVisible();
    await expect(page.getByRole('combobox', { name: 'Room invitations' })).toHaveCount(0);
    await directMessages.selectOption(String(NotificationDeliveryMode.OFF));
    await expect(directMessages).toHaveValue(String(NotificationDeliveryMode.OFF));

    await page.reload();
    await expect(page.getByRole('combobox', { name: 'Direct messages' })).toHaveValue(
      String(NotificationDeliveryMode.OFF)
    );
  });

  test('inherits server policy and supports a room override through Connect', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const roomId = await getRoomIdByNameViaConnect(page, 'general');

    await setNotificationPolicyPreference(page, 'FOLLOWED_ROOM', 'BADGE');
    let roomPolicy = await getNotificationPolicy(page, roomId);
    expect(roomPolicy.find(({ category }) => category === 'FOLLOWED_ROOM')).toMatchObject({
      override: 'UNSPECIFIED',
      effective: 'BADGE'
    });

    roomPolicy = await setNotificationPolicyPreference(page, 'FOLLOWED_ROOM', 'ALERT', roomId);
    expect(roomPolicy.find(({ category }) => category === 'FOLLOWED_ROOM')).toMatchObject({
      override: 'ALERT',
      effective: 'ALERT'
    });
  });
});
