import { expect } from '@playwright/test';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withBootstrapAdminRequest } from './fixtures/adminRequest';
import {
  connectPost,
  createRoomViaConnect,
  getDefaultRoomGroupIdViaConnect,
  getRoomIdByNameViaConnect,
  getScopedNotificationPolicy,
  joinRoomViaConnect,
  markRoomAsReadViaConnect,
  postMessageViaConnect,
  updateScopedNotificationPolicy,
  waitForMessageViaConnect,
  waitForRoomReadViaConnect,
  waitForRoomUnreadViaConnect
} from './fixtures/connectHelpers';
import { withServerUser } from './fixtures/serverUser';
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
    await expect(page.locator('[data-notification-field="followedRooms"]')).toHaveCount(0);
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
    await expect(
      page.locator(
        'td[data-notification-scope^="roomGroup:"][data-notification-field="roomMessages"] button'
      )
    ).toBeVisible();

    await directMessages.click();
    await expect(directMessages).toHaveAttribute('aria-label', /Override: Off/);

    await directMessages.press('Enter');
    await expect(directMessages).toHaveAttribute('aria-label', /Override: Badge/);

    await directMessages.press('Enter');
    await expect(directMessages).toHaveAttribute('aria-label', /Override: Notification/);

    await page.reload();
    await expect(directMessages).toHaveAttribute('aria-label', /Override: Notification/);
  });

  test('Badge adds only neutral unread attention and clears when the room is read', async ({
    page,
    chatPage,
    notificationsPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60_000);

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');
    const messageEventId = await postMessageViaConnect(
      page,
      roomId,
      `Badge reaction target ${Date.now()}`
    );
    await waitForRoomReadViaConnect(page, roomId);
    await updateScopedNotificationPolicy(page, { server: {} }, { reactions: 'UNREAD_BADGE' });
    await chatPage.enterRoom('announcements');

    await withServerUser(browser!, serverURL, async ({ page: actorPage, chatPage: actorChat }) => {
      await actorChat.enterRoom('general');
      await connectPost(actorPage, 'chatto.api.v1.MessageService/AddReaction', {
        roomId,
        messageEventId,
        emoji: 'thumbsup'
      });
    });

    await waitForRoomUnreadViaConnect(page, roomId, true, 10_000);
    const roomLink = chatPage.getRoomLink('general');
    const unreadDot = roomLink.getByTestId('room-unread-dot');
    await expect(unreadDot).toBeVisible({ timeout: 10_000 });
    await expect(unreadDot).toHaveClass(/bg-neutral-action/);
    await expect(roomLink.getByTestId('room-notification-badge')).not.toBeVisible();
    await notificationsPage.expectBellIndicatorNotVisible();
    await notificationsPage.goto();
    await notificationsPage.expectEmptyState();

    await chatPage.goto();
    await chatPage.enterRoom('general');
    await waitForRoomReadViaConnect(page, roomId, 10_000);
    await chatPage.enterRoom('announcements');
    await expect(chatPage.getRoomLink('general').getByTestId('room-unread-dot')).not.toBeVisible();
  });

  test('Room messages Off removes the room dot but keeps the New messages separator', async ({
    page,
    chatPage,
    roomPage,
    browser,
    serverURL
  }) => {
    test.setTimeout(60_000);

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');
    const roomId = await getRoomIdByNameViaConnect(page, 'general');
    await postMessageViaConnect(page, roomId, `Read cursor baseline ${Date.now()}`);
    await markRoomAsReadViaConnect(page, roomId);
    await updateScopedNotificationPolicy(page, { roomId }, { roomMessages: 'OFF' });
    await chatPage.enterRoom('announcements');

    const newMessage = `No Badge, cursor retained ${Date.now()}`;
    const newMessageEventId = await withServerUser(browser!, serverURL, async ({ page: actorPage }) => {
      return postMessageViaConnect(actorPage, roomId, newMessage);
    });

    // Observe the source through this viewer's room timeline while the room is
    // still closed. This proves that the following absence checks run after
    // delivery rather than against the pre-message state.
    await waitForMessageViaConnect(page, roomId, newMessageEventId, 10_000);
    await waitForRoomUnreadViaConnect(page, roomId, false, 10_000);
    await expect(chatPage.getRoomLink('general').getByTestId('room-unread-dot')).not.toBeVisible();

    await chatPage.enterRoom('general');
    await roomPage.expectMessageVisible(newMessage);
    await roomPage.expectUnreadSeparator();
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
      { roomMessages: 'IN_APP_NOTIFICATION' }
    );
    await updateScopedNotificationPolicy(
      page,
      { roomGroupId: groupId },
      { roomMessages: 'PUSH_NOTIFICATION' }
    );
    let roomPolicy = await getScopedNotificationPolicy(page, { roomId });
    expect(roomPolicy.overrides.roomMessages).toBeNull();
    expect(roomPolicy.effective.roomMessages).toBe('PUSH_NOTIFICATION');

    roomPolicy = await updateScopedNotificationPolicy(page, { roomId }, { roomMessages: 'OFF' });
    expect(roomPolicy.overrides.roomMessages).toBe('OFF');
    expect(roomPolicy.effective.roomMessages).toBe('OFF');

    roomPolicy = await updateScopedNotificationPolicy(page, { roomId }, { roomMessages: null });
    expect(roomPolicy.overrides.roomMessages).toBeNull();
    expect(roomPolicy.effective.roomMessages).toBe('PUSH_NOTIFICATION');

    await page.goto(routes.settingsNotifications);
    await expect(page.locator(`th[data-notification-scope="roomGroup:${groupId}"]`)).toBeVisible();
    await expect(page.locator(`th[data-notification-scope="room:${roomId}"]`)).toBeVisible();
    await expect(page.locator(`th[data-notification-scope="room:${nonMemberRoomId}"]`)).toHaveCount(
      0
    );
  });
});
