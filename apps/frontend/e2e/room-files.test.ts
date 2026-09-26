import { expect, type Page } from '@playwright/test';
import { TIMEOUTS } from './constants';
import { test } from './setup';
import { postMessageViaConnect, postThreadReplyViaConnect } from './fixtures/connectHelpers';
import { loginAndEnterRoom, withServerUser } from './fixtures/serverUser';

function roomIdFromUrl(page: Page): string {
  const match = page.url().match(/\/chat\/-\/([^/]+)/);
  if (!match) throw new Error(`Could not extract roomId from URL: ${page.url()}`);
  return match[1];
}

async function postFillerMessages(page: Page, roomId: string, prefix: string, count: number) {
  for (let index = 0; index < count; index++) {
    await postMessageViaConnect(page, roomId, `${prefix} ${index}`);
  }
}

test('room Files sidebar previews files and separately jumps to their messages', async ({
  page
}) => {
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
  await expect(roomPage.threadPane.getByTestId('composer-attachment-preview')).toBeVisible({
    timeout: TIMEOUTS.UI_STANDARD
  });
  await roomPage.threadReplyInput.fill(threadReplyText);
  await roomPage.threadReplyInput.press('Control+Enter');
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

  const rootRow = filesPanel.getByTestId('room-file-row').filter({ hasText: 'brighton.jpg' });
  const threadRow = filesPanel.getByTestId('room-file-row').filter({ hasText: 'brighton2.jpg' });
  const roomUrl = page.url();
  const scrollTop = await filesPanel.evaluate((element) => element.scrollTop);
  await rootRow.getByTestId('room-file-preview').click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByRole('dialog')).toContainText('brighton.jpg');
  await expect(page).toHaveURL(roomUrl);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await expect(filesPanel).toBeVisible();
  expect(await filesPanel.evaluate((element) => element.scrollTop)).toBe(scrollTop);

  await rootRow.getByRole('button', { name: 'Go to message', exact: true }).click();
  await expect(rootFileMessage.locator).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

  await threadRow.getByTestId('room-file-preview').focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.getByRole('dialog')).toContainText('brighton2.jpg');
  await expect(page).toHaveURL(roomUrl);
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await threadRow.getByRole('button', { name: 'Go to message', exact: true }).click();
  await roomPage.expectThreadRouteActive(threadRootEventId);
  await roomPage.expectTextInThreadPane(threadReplyText);
  await expect(roomPage.getThreadMessage(threadReplyText).locator).toHaveClass(/highlight-flash/, {
    timeout: TIMEOUTS.UI_STANDARD
  });
});

test('mobile Files panel stays open when Escape closes the file viewer', async ({ page }) => {
  const { roomPage } = await loginAndEnterRoom(page);
  await roomPage.sendAttachment('e2e/fixtures/brighton.jpg', 'Mobile file preview');
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole('button', { name: 'Actions for #general' }).click();
  await page.getByRole('button', { name: 'Show files', exact: true }).click();

  const filesPanel = page.locator(
    '[data-testid="room-sidebar-mobile-pane"] nav[aria-label="Files"]'
  );
  await filesPanel.getByTestId('room-file-preview').click();
  await expect(page.getByRole('dialog', { name: 'brighton.jpg' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).not.toBeVisible();
  await expect(filesPanel).toBeVisible();
  await expect(filesPanel.getByTestId('room-file-preview')).toBeFocused();
});

test('Files keeps its rows and scroll position during room and closed-thread posts', async ({
  page,
  browser,
  serverURL
}) => {
  await page.setViewportSize({ width: 1280, height: 600 });
  const { roomPage } = await loginAndEnterRoom(page);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  const roomId = roomIdFromUrl(page);
  await page
    .locator('[data-testid="room-view-region"] input[type="file"]')
    .first()
    .setInputFiles(
      Array.from({ length: 10 }, (_, index) => ({
        name: `stable-file-${index}.txt`,
        mimeType: 'text/plain',
        buffer: Buffer.from(`File ${index}`)
      }))
    );
  await expect(page.getByTestId('composer-attachment-preview')).toHaveCount(10);
  const root = await roomPage.sendMessage('Files regression root');
  const rootId = await root.getEventId();
  if (!rootId) throw new Error('Missing root message ID');
  await page
    .locator('[data-testid="room-sidebar-toggle"]:visible')
    .getByLabel('Show files')
    .click();
  const panel = page.locator('aside[aria-label="Room extras"] nav[aria-label="Files"]');
  await expect(panel.getByTestId('room-file-row')).toHaveCount(10);
  await panel.hover();
  await page.mouse.wheel(0, 300);
  await expect.poll(() => panel.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  const scrollTop = await panel.evaluate((element) => element.scrollTop);
  const original = await panel.getByTestId('room-file-row').first().elementHandle();
  const listRequests: string[] = [];
  page.on('request', (request) => {
    if (request.url().endsWith('/ListRoomAttachments')) listRequests.push(request.url());
  });

  await withServerUser(browser, serverURL, async ({ page: sender, chatPage }) => {
    await chatPage.enterRoom('general');
    const roomUpdate = page.waitForResponse(
      (response) => response.url().endsWith('/BatchGetMessages') && response.ok()
    );
    await postMessageViaConnect(sender, roomId, 'Receiver room update');
    await roomUpdate;
    await expect(page.getByText('Receiver room update', { exact: true })).toBeVisible();
    const threadUpdate = page.waitForResponse(
      (response) => response.url().endsWith('/BatchGetMessages') && response.ok()
    );
    await postThreadReplyViaConnect(sender, roomId, 'Receiver closed-thread update', rootId);
    await threadUpdate;
  });
  expect(await original!.evaluate((element) => element.isConnected)).toBe(true);
  expect(await panel.evaluate((element) => element.scrollTop)).toBe(scrollTop);
  expect(listRequests).toEqual([]);
  expect(errors).toEqual([]);
});
