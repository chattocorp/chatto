import { expect } from '@playwright/test';
import { test } from './setup';
import { TIMEOUTS } from './constants';
import { createAndLoginTestUser, loginAsAdmin, openServer } from './fixtures/testUser';
import * as routes from './routes';

test('Slow Mode updates channel and thread timers independently', async ({
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
    const threadSlowModeSelect = adminPage.locator('#room-settings-thread-slow-mode');
    const saveSlowMode = adminPage
      .locator('form', { has: slowModeSelect })
      .getByRole('button', { name: 'Save changes' });
    await expect(slowModeSelect).toBeVisible();
    await expect(threadSlowModeSelect).toBeVisible();

    await slowModeSelect.selectOption('30');
    await expect(slowModeSelect).toHaveValue('30');
    await threadSlowModeSelect.selectOption('5');
    await expect(threadSlowModeSelect).toHaveValue('5');
    await saveSlowMode.click();
    await expect(adminPage.getByText('Room updated').last()).toBeVisible();

    const roomStatus = page.getByTestId('slow-mode-status');
    await expect(roomStatus).toBeHidden({
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    const rootText = `Slow Mode root ${Date.now()}`;
    const root = await roomPage.sendMessage(rootText);
    await expect(roomStatus).toHaveAttribute(
      'aria-label',
      /Slow Mode: send again in 0:(?:[12][0-9]|30)/
    );
    await expect(roomPage.sendButton).toBeDisabled();

    await root.openThread();
    await roomPage.expectThreadPaneVisible();
    const threadStatus = roomPage.threadPane.getByTestId('slow-mode-status');
    const threadSend = roomPage.threadPane.getByRole('button', { name: 'Send message' });
    await expect(threadStatus).toHaveAttribute('aria-label', /Slow Mode: send again in 0:0/);
    await expect(threadSend).toBeDisabled();

    await expect(threadStatus).toBeHidden({ timeout: 8_000 });
    const replyText = `Slow Mode reply ${Date.now()}`;
    await roomPage.threadReplyInput.fill(replyText);
    await expect(threadSend).toBeEnabled();
    await roomPage.threadReplyInput.press('Control+Enter');
    await roomPage.expectTextInThreadPane(replyText);
    await expect(threadSend).toBeDisabled();

    await threadSlowModeSelect.selectOption('10');
    await saveSlowMode.click();
    await expect(threadStatus).toHaveText(/0:(?:0[6-9]|10)/, {
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await roomPage.threadReplyInput.fill('Draft preserved while Slow Mode is active');
    await expect(threadSend).toBeEnabled();
    await threadSend.click();
    await expect(roomPage.threadReplyInput).toHaveText('Draft preserved while Slow Mode is active');
    await expect(roomPage.threadReplyInput).toBeFocused();
    await expect(threadStatus).toBeVisible();

    await threadSlowModeSelect.selectOption('0');
    await saveSlowMode.click();
    await expect(threadStatus).toBeHidden({ timeout: TIMEOUTS.REALTIME_EVENT });
    await expect(threadSend).toBeEnabled();
    await expect(slowModeSelect).toHaveValue('30');
    await roomPage.threadReplyInput.press('Control+Enter');
    await roomPage.expectTextInThreadPane('Draft preserved while Slow Mode is active');
  } finally {
    await adminContext.close();
  }
});
