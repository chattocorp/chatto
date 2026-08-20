import { expect } from '@playwright/test';
import { NotificationDeliveryMode } from '@chatto/api-types/api/v1/notifications_pb';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  getNotificationPolicy,
  getRoomIdByNameViaConnect,
  updateNotificationPolicy
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

    await updateNotificationPolicy(page, { followedRooms: 'SILENT' });
    let roomPolicy = await getNotificationPolicy(page, roomId);
    expect(roomPolicy.overrides.followedRooms).toBeNull();
    expect(roomPolicy.effective.followedRooms).toBe('SILENT');

    roomPolicy = await updateNotificationPolicy(page, { followedRooms: 'ALERT' }, roomId);
    expect(roomPolicy.overrides.followedRooms).toBe('ALERT');
    expect(roomPolicy.effective.followedRooms).toBe('ALERT');
  });
});
