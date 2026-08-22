import { expect } from '@playwright/test';
import { test } from './setup';
import { TIMEOUTS } from './constants';
import { createAndLoginTestUser, loginAsAdmin, openServer } from './fixtures/testUser';
import * as routes from './routes';

test('Slow Mode updates live and shares one room timer with threads', async ({
  browser,
  page,
  chatPage,
  roomPage,
  serverURL
}) => {
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');

  const adminContext = await browser.newContext({ baseURL: serverURL });
  const adminPage = await adminContext.newPage();
  try {
    await loginAsAdmin(adminPage);
    await openServer(adminPage);
    await adminPage.goto(routes.serverAdminRooms);
    const generalRow = adminPage.locator('.cursor-grab', { hasText: 'general' });
    await generalRow.getByTitle('Edit room').click();
    const slowModeSelect = adminPage.locator('#room-settings-slow-mode');
    await expect(slowModeSelect).toBeVisible();

    await slowModeSelect.selectOption('5');
    await adminPage.getByRole('button', { name: 'Save Changes' }).click();
    await expect(adminPage.getByText('Room updated')).toBeVisible();

    const roomStatus = page.getByTestId('slow-mode-status');
    await expect(roomStatus).toHaveText('Slow Mode: one message every 5 seconds.', {
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    const rootText = `Slow Mode root ${Date.now()}`;
    const root = await roomPage.sendMessage(rootText);
    await expect(roomStatus).toContainText('Slow Mode: send again in 0:0');
    await expect(roomPage.sendButton).toBeDisabled();

    await root.openThread();
    await roomPage.expectThreadPaneVisible();
    const threadStatus = roomPage.threadPane.getByTestId('slow-mode-status');
    const threadSend = roomPage.threadPane.getByRole('button', { name: 'Send message' });
    await expect(threadStatus).toContainText('Slow Mode: send again in 0:0');
    await expect(threadSend).toBeDisabled();

    await expect(threadStatus).toHaveText('Slow Mode: one message every 5 seconds.', {
      timeout: 8_000
    });
    const replyText = `Slow Mode reply ${Date.now()}`;
    await roomPage.threadReplyInput.fill(replyText);
    await expect(threadSend).toBeEnabled();
    await roomPage.threadReplyInput.press('Control+Enter');
    await roomPage.expectTextInThreadPane(replyText);
    await expect(threadSend).toBeDisabled();

    await slowModeSelect.selectOption('10');
    await adminPage.getByRole('button', { name: 'Save Changes' }).click();
    await expect(threadStatus).toHaveText(/Slow Mode: send again in 0:(?:0[6-9]|10)\./, {
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await roomPage.threadReplyInput.fill('Draft preserved while Slow Mode is active');
    await expect(threadSend).toBeDisabled();

    await slowModeSelect.selectOption('0');
    await adminPage.getByRole('button', { name: 'Save Changes' }).click();
    await expect(threadStatus).toBeHidden({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(threadSend).toBeEnabled();
  } finally {
    await adminContext.close();
  }
});
