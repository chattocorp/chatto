import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import {
  getIdsFromUrlViaConnect,
  postMessageViaConnect,
  postMessagesViaConnect,
  postReplyViaConnect
} from './fixtures/connectHelpers';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

async function clickReplyAttributionJump(page: Page, replyBody: string): Promise<void> {
  const replyAttribution = page
    .locator('[role="article"]', { hasText: replyBody })
    .getByTestId('reply-attribution');

  // The nested author button opens the user popover and stops propagation.
  // Click the left edge of the attribution container so the container's jump
  // handler receives the event.
  await replyAttribution.click({ position: { x: 8, y: 8 } });
}

async function gotoMessageAndWaitForTarget(
  page: Page,
  roomId: string,
  eventId: string,
  targetBody: string
): Promise<void> {
  await expect(async () => {
    await page.goto(routes.messageLink(roomId, eventId));
    await expect(page.locator('p', { hasText: targetBody })).toBeVisible({
      timeout: TIMEOUTS.UI_FAST
    });
  }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [100, 250, 500, 1000] });
}

test.describe('jump to message', () => {
  // These tests post 60+ messages via API — needs more time than the default
  test.describe.configure({ timeout: 60_000 });

  test('clicking reply link on a message jumps to the referenced message', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrlViaConnect(page);
    const timestamp = Date.now();

    // Post an early message that will be the reply target
    const targetBody = `Target message - ${timestamp}`;
    const targetEventId = await postMessageViaConnect(page, roomId, targetBody);

    // Post enough messages to push the target well out of the initial load window
    const fillerMessages = Array.from({ length: 60 }, (_, i) => `Filler ${i + 1} - ${timestamp}`);
    await postMessagesViaConnect(page, roomId, fillerMessages);

    // Post a reply that references the target (the old message)
    const replyBody = `Reply pointing to target - ${timestamp}`;
    await postReplyViaConnect(page, roomId, replyBody, targetEventId);

    // Reload so we get a clean state with only the latest ~50 messages
    await page.reload();
    await page.waitForURL(/\/chat\/-\/[a-zA-Z0-9_-]+$/);

    // Wait for the reply message to be visible (it's in the latest batch)
    await expect(page.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // The target message should NOT be visible (it's too old, not in the loaded window).
    // Scope to <p> tags to avoid matching the reply attribution preview text.
    await expect(page.locator('p', { hasText: targetBody })).not.toBeVisible();

    // Click the reply link ("In reply to ...")
    await clickReplyAttributionJump(page, replyBody);

    // The target message should now be visible after the jump
    await expect(page.locator('p', { hasText: targetBody })).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // The "Jump to Present" button should appear
    await expect(page.getByTestId('jump-to-present')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // The latest filler messages should no longer be visible (cache was replaced)
    await expect(page.getByText(`Filler 60 - ${timestamp}`)).not.toBeVisible();
  });

  test('Jump to Present returns to latest messages', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrlViaConnect(page);
    const timestamp = Date.now();

    // Post an early message that will be the reply target
    const targetBody = `JTP target - ${timestamp}`;
    const targetEventId = await postMessageViaConnect(page, roomId, targetBody);

    // Post enough messages to push the target out of view
    const fillerMessages = Array.from(
      { length: 60 },
      (_, i) => `JTP filler ${i + 1} - ${timestamp}`
    );
    await postMessagesViaConnect(page, roomId, fillerMessages);

    // Post a reply referencing the target
    const replyBody = `JTP reply - ${timestamp}`;
    await postReplyViaConnect(page, roomId, replyBody, targetEventId);

    // Reload for clean state
    await page.reload();
    await page.waitForURL(/\/chat\/-\/[a-zA-Z0-9_-]+$/);
    await expect(page.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Jump to the old message via the direct message route. The reply-link
    // interaction is covered above; this test focuses on returning to present.
    await gotoMessageAndWaitForTarget(page, roomId, targetEventId, targetBody);

    // The floating button can sit over a moving scroll layer, so avoid pointer
    // interception from timeline content while still exercising the click handler.
    await page.getByTestId('jump-to-present').evaluate((button: HTMLElement) => button.click());

    // Should return to the latest messages
    await expect(page.getByText(`JTP filler 60 - ${timestamp}`)).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // The "Jump to Present" button should disappear
    await expect(page.getByTestId('jump-to-present')).not.toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // The old target message should no longer be visible (scope to <p> to exclude reply preview)
    await expect(page.locator('p', { hasText: targetBody })).not.toBeVisible();
  });

  test('jump to message works for nearby messages already in DOM', async ({
    page,
    chatPage,
    roomPage: _roomPage
  }) => {
    // Use smaller viewport to make scrolling meaningful
    await page.setViewportSize({ width: 1280, height: 500 });

    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrlViaConnect(page);
    const timestamp = Date.now();

    // Post the target message first, then enough messages to scroll it off screen
    // but NOT out of the loaded cache (within the 50-message window)
    const targetBody = `Nearby target - ${timestamp}`;
    const targetEventId = await postMessageViaConnect(page, roomId, targetBody);

    // Post 30 messages (still within the 50-message initial load)
    const fillerMessages = Array.from(
      { length: 30 },
      (_, i) => `Nearby filler ${i + 1} - ${timestamp}`
    );
    await postMessagesViaConnect(page, roomId, fillerMessages);

    // Post a reply to the target
    const replyBody = `Nearby reply - ${timestamp}`;
    await postReplyViaConnect(page, roomId, replyBody, targetEventId);

    // Reload for clean state
    await page.reload();
    await page.waitForURL(/\/chat\/-\/[a-zA-Z0-9_-]+$/);
    await expect(page.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    // Click the reply link — this should use the in-DOM scroll path
    // (no API fetch needed since the message is in the loaded cache)
    await clickReplyAttributionJump(page, replyBody);

    // The target should be scrolled into view and highlighted
    await expect(page.getByText(targetBody)).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('switching rooms resets jump state', async ({ page, chatPage, roomPage: _roomPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.enterRoom('general');

    const { roomId } = await getIdsFromUrlViaConnect(page);
    const timestamp = Date.now();

    // Set up: target message, filler, reply
    const targetBody = `Reset target - ${timestamp}`;
    const targetEventId = await postMessageViaConnect(page, roomId, targetBody);

    const fillerMessages = Array.from(
      { length: 60 },
      (_, i) => `Reset filler ${i + 1} - ${timestamp}`
    );
    await postMessagesViaConnect(page, roomId, fillerMessages);

    const replyBody = `Reset reply - ${timestamp}`;
    await postReplyViaConnect(page, roomId, replyBody, targetEventId);

    // Reload and jump
    await page.reload();
    await page.waitForURL(/\/chat\/-\/[a-zA-Z0-9_-]+$/);
    await expect(page.getByText(replyBody)).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });

    await clickReplyAttributionJump(page, replyBody);
    await expect(page.getByTestId('jump-to-present')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Create and switch to a new room
    const newRoomName = await chatPage.createRoom(`other-room-${timestamp}`);
    await expect(page.getByRole('heading', { name: `# ${newRoomName}` })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // "Jump to Present" should be gone
    await expect(page.getByTestId('jump-to-present')).not.toBeVisible();

    // Switch back to general
    await chatPage.enterRoom('general');

    // Should show the latest messages, not the jumped state
    await expect(page.getByText(`Reset filler 60 - ${timestamp}`)).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // "Jump to Present" should still not be visible
    await expect(page.getByTestId('jump-to-present')).not.toBeVisible();
  });
});
