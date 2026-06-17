import { expect, type Page } from '@playwright/test';
import { TIMEOUTS } from './constants';
import { test } from './setup';
import { postMessageViaAPI } from './fixtures/graphqlHelpers';
import { loginAndEnterRoom } from './fixtures/serverUser';

function roomIdFromUrl(page: Page): string {
  const match = page.url().match(/\/chat\/-\/([^/]+)/);
  if (!match) throw new Error(`Could not extract roomId from URL: ${page.url()}`);
  return match[1];
}

async function postFillerMessages(page: Page, roomId: string, prefix: string, count: number) {
  for (let index = 0; index < count; index++) {
    await postMessageViaAPI(page, roomId, `${prefix} ${index}`);
  }
}

test('room Files sidebar jumps to root files and opens thread reply files', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 820 });
  const { roomPage } = await loginAndEnterRoom(page);

  const roomId = roomIdFromUrl(page);
  const stamp = Date.now();

  const rootFileText = `Root file anchor ${stamp}`;
  const rootFileMessage = await roomPage.sendAttachment('e2e/fixtures/brighton.jpg', rootFileText);

  const threadRootText = `Thread file root ${stamp}`;
  const threadRoot = await roomPage.sendMessage(threadRootText);
  const threadRootEventId = await threadRoot.getEventId();
  if (!threadRootEventId) throw new Error('Thread root did not expose a data-event-id');

  await threadRoot.openThread();
  await roomPage.expectThreadPaneVisible();

  const threadReplyText = `Thread file reply ${stamp}`;
  await roomPage.threadPane
    .locator('input[type="file"]')
    .setInputFiles('e2e/fixtures/brighton2.jpg');
  await expect(roomPage.threadPane.locator('img.h-16.w-16')).toBeVisible({
    timeout: TIMEOUTS.UI_STANDARD
  });
  await roomPage.threadReplyInput.fill(threadReplyText);
  await roomPage.threadReplyInput.press('Enter');
  await roomPage.expectTextInThreadPane(threadReplyText);
  await roomPage.closeThread();

  await postFillerMessages(page, roomId, `Files filler ${stamp}`, 80);
  await expect(page.getByText(`Files filler ${stamp} 79`)).toBeVisible({
    timeout: TIMEOUTS.REALTIME_EVENT
  });

  await page
    .locator('[data-testid="room-sidebar-toggle"]:visible')
    .getByLabel('Show files')
    .click();
  await expect(page.getByRole('heading', { name: 'Files' })).toBeVisible();

  const filesPanel = page.locator('aside[aria-label="Room extras"] nav[aria-label="Files"]');
  await expect(
    filesPanel.getByTestId('room-file-row').filter({ hasText: 'brighton.jpg' })
  ).toBeVisible();
  await expect(
    filesPanel.getByTestId('room-file-row').filter({ hasText: 'brighton2.jpg' })
  ).toBeVisible();

  await filesPanel.getByTestId('room-file-row').filter({ hasText: 'brighton.jpg' }).click();
  await expect(rootFileMessage.locator).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

  await filesPanel.getByTestId('room-file-row').filter({ hasText: 'brighton2.jpg' }).click();
  await roomPage.expectThreadRouteActive(threadRootEventId);
  await roomPage.expectTextInThreadPane(threadReplyText);
  await expect(roomPage.getThreadMessage(threadReplyText).locator).toHaveClass(/highlight-flash/, {
    timeout: TIMEOUTS.UI_STANDARD
  });
});
