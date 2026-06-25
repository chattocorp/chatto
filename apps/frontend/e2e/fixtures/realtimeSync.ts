import { expect, type Page } from '@playwright/test';
import { TIMEOUTS } from '../constants';
import { RoomPage } from '../pages';

/**
 * Helper to verify real-time sync is working between two users.
 *
 * In multi-user E2E tests, there's a race condition window after a user navigates
 * to a room but before their WebSocket subscription is fully connected. During this
 * window, real-time events from other users may be missed.
 *
 * This helper sends a "sync" message from one user and waits for the other to receive it,
 * proving the real-time channel is operational before proceeding with test assertions.
 *
 * @example
 * ```typescript
 * // After both users are in the room, verify sync works before testing
 * await verifyRealtimeSync(roomPage1, roomPage2);
 *
 * // Now safe to test real-time features
 * await roomPage2.sendMessage('hello');
 * await roomPage1.expectMessageVisible('hello');
 * ```
 */
export async function verifyRealtimeSync(
  senderRoom: RoomPage,
  receiverRoom: RoomPage,
  options: { timeout?: number } = {}
): Promise<void> {
  const { timeout = TIMEOUTS.REALTIME_EVENT } = options;
  const syncId = `__sync_${Date.now()}_${Math.random().toString(36).slice(2)}__`;

  // Sender sends sync message
  await senderRoom.messageInput.fill(syncId);
  await senderRoom.messageInput.press('Enter');

  // Receiver waits to see it (proves subscription is connected)
  await expect(receiverRoom.page.getByText(syncId)).toBeVisible({ timeout });
}

/**
 * Wait for a page's WebSocket subscription to be ready by watching for
 * any real-time event. This is useful when you need to ensure a user's
 * subscription is connected before another user performs an action.
 *
 * The strategy is to wait for the room to be fully loaded, including:
 * - The room header is visible
 * - The message input is ready
 * - Any existing messages have loaded
 *
 * This gives the subscription time to connect during the initial load.
 *
 * @example
 * ```typescript
 * await chatPage2.enterRoom('general');
 * await waitForRoomReady(page2);
 * // Now User 2's subscription should be connected
 * ```
 */
export async function waitForRoomReady(
  page: Page,
  roomName?: string,
  options: { timeout?: number } = {}
): Promise<void> {
  const { timeout = TIMEOUTS.REALTIME_EVENT } = options;

  // Wait for room header (proves navigation completed)
  if (roomName) {
    await expect(page.getByRole('heading', { name: `# ${roomName}` })).toBeVisible({
      timeout
    });
  }

  // Wait for message input to be ready (proves room component loaded).
  // Note: don't check contenteditable="true" here - the editor may be read-only
  // if the user doesn't have posting permission (e.g., announcements room).
  await expect(page.getByTestId('message-input')).toBeVisible({ timeout });

  // Wait for ServerEventProvider to have mounted and initiated the subscription.
  // The hidden marker element proves the component rendered, and since
  // the `myEvents` subscription is started in the first $effect cycle after
  // render, the subscription request has been sent by the time this resolves.
  await expect(page.getByTestId('server-subscription-active')).toBeAttached({
    timeout
  });
}
