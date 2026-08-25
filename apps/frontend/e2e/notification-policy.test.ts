import { expect } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withBootstrapAdminRequest } from './fixtures/adminRequest';
import {
  createRoomViaConnect,
  getDefaultRoomGroupIdViaConnect,
  getRoomIdByNameViaConnect,
  getScopedNotificationPolicy,
  joinRoomViaConnect,
  updateScopedNotificationPolicy
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

    const directMessages = page.locator(
      'td[data-notification-scope="server"][data-notification-field="directMessages"] button'
    );
    await expect(directMessages).toBeVisible();
    await expect(page.locator('[data-notification-field="roomInvitations"]')).toHaveCount(0);
    await expect(
      page.locator('td[data-notification-scope="server"] [data-notification-field]')
    ).toHaveCount(9);
    await expect(directMessages).toHaveAttribute('aria-label', /Default: Push notification/);
    await expect(
      page.locator(
        'td[data-notification-scope="server"][data-notification-field="directMessages"] [class~="icon-[uil--link]"]'
      )
    ).toHaveCount(0);
    const groupDirectMessages = page.locator(
      'td[data-notification-scope^="roomGroup:"][data-notification-field="directMessages"]'
    );
    await expect(groupDirectMessages.getByRole('button')).toHaveCount(0);
    await expect(groupDirectMessages.getByRole('img')).toHaveAttribute(
      'aria-label',
      /Not applicable/
    );

    await directMessages.click();
    await expect(directMessages).toHaveAttribute('aria-label', /Override: Off/);

    await directMessages.press('Enter');
    await expect(directMessages).toHaveAttribute('aria-label', /Override: Notification/);

    await page.reload();
    await expect(directMessages).toHaveAttribute('aria-label', /Override: Notification/);
  });

  test('resolves server, group, and room overrides and shows member rooms only', async ({
    page,
    chatPage,
    serverURL
  }) => {
    await createAndLoginTestUser(page, { skipDefaultRooms: true });
    await chatPage.goto();
    const groupId = await getDefaultRoomGroupIdViaConnect(page);
    const roomId = await getRoomIdByNameViaConnect(page, 'general');
    const nonMemberRoomId = await withBootstrapAdminRequest(serverURL, (adminRequest) =>
      createRoomViaConnect(adminRequest, 'matrix-hidden', groupId)
    );
    await joinRoomViaConnect(page, roomId);

    await updateScopedNotificationPolicy(
      page,
      { server: {} },
      { followedRooms: 'IN_APP_NOTIFICATION' }
    );
    await updateScopedNotificationPolicy(
      page,
      { roomGroupId: groupId },
      { followedRooms: 'PUSH_NOTIFICATION' }
    );
    let roomPolicy = await getScopedNotificationPolicy(page, { roomId });
    expect(roomPolicy.overrides.followedRooms).toBeNull();
    expect(roomPolicy.effective.followedRooms).toBe('PUSH_NOTIFICATION');

    roomPolicy = await updateScopedNotificationPolicy(
      page,
      { roomId },
      { followedRooms: 'OFF' }
    );
    expect(roomPolicy.overrides.followedRooms).toBe('OFF');
    expect(roomPolicy.effective.followedRooms).toBe('OFF');

    roomPolicy = await updateScopedNotificationPolicy(
      page,
      { roomId },
      { followedRooms: null }
    );
    expect(roomPolicy.overrides.followedRooms).toBeNull();
    expect(roomPolicy.effective.followedRooms).toBe('PUSH_NOTIFICATION');

    await page.goto(routes.settingsNotifications);
    await expect(page.locator(`th[data-notification-scope="roomGroup:${groupId}"]`)).toBeVisible();
    await expect(page.locator(`th[data-notification-scope="room:${roomId}"]`)).toBeVisible();
    await expect(page.locator(`th[data-notification-scope="room:${nonMemberRoomId}"]`)).toHaveCount(
      0
    );
  });
});
