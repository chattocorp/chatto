import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { connectPost, postMessageViaConnect } from './fixtures/connectHelpers';
import { TIMEOUTS } from './constants';

/**
 * Opens the quick switcher palette via Cmd/Ctrl+K.
 * Returns the dialog locator.
 *
 * Retries the keypress: if the chat layout's <svelte:window onkeydown> handler
 * isn't fully wired up yet (race after navigation), the first Meta+k can be
 * dropped and the dialog never opens. quickSwitcher.open() is idempotent, so
 * re-pressing while open is a safe no-op.
 */
async function openSwitcher(page: import('@playwright/test').Page) {
  const isMac = process.platform === 'darwin';
  const key = isMac ? 'Meta+k' : 'Control+k';
  const dialog = page.locator('dialog.quick-switcher');

  await expect(async () => {
    await page.keyboard.press(key);
    await expect(dialog).toBeVisible({ timeout: 500 });
  }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [200, 500, 1000] });

  return dialog;
}

/** Returns the search input inside the quick switcher. */
function switcherInput(dialog: import('@playwright/test').Locator) {
  return dialog.getByPlaceholder('Go somewhere, or type ? to search messages...');
}

/** Returns all result buttons inside the quick switcher. */
function switcherResults(dialog: import('@playwright/test').Locator) {
  return dialog.getByRole('navigation').getByRole('button');
}

test.describe('Quick Switcher (Cmd-K)', () => {
  test('opens with Cmd-K and closes with Escape', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const dialog = await openSwitcher(page);
    await expect(switcherInput(dialog)).toBeFocused();

    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  });

  test('opens via the header button', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await page.getByRole('button', { name: 'Open quick switcher' }).click();

    const dialog = page.locator('dialog.quick-switcher');
    await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await expect(switcherInput(dialog)).toBeFocused();
  });

  test('closes when clicking outside the dialog', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const dialog = await openSwitcher(page);

    await page.mouse.click(5, 5);
    await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  });

  test('clicking a result navigates to it', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const dialog = await openSwitcher(page);

    await expect(switcherResults(dialog).first()).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    await switcherResults(dialog).filter({ hasText: 'general' }).click();

    await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
    await expect(page.getByRole('heading', { name: '# general' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('uses a known user until their DM has message history', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await withServerUser(browser, serverURL, async ({ user: userB }) => {
      const dialog = await openSwitcher(page);
      const input = switcherInput(dialog);

      // Wait for the initial load to settle.
      await expect(switcherResults(dialog).first()).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });

      const memberSearches: string[] = [];
      page.on('request', (request) => {
        if (request.url().endsWith('/chatto.api.v1.UserService/ListUsers'))
          memberSearches.push(request.url());
      });
      const dm = await connectPost<{ room?: { id?: string } }>(
        page,
        'chatto.api.v1.RoomService/StartDM',
        { participantIds: [userB.id] }
      );
      const roomId = dm.room?.id;
      if (!roomId) throw new Error('DM fixture did not return a room');
      await input.fill(userB.login);
      const knownUser = switcherResults(dialog).filter({ hasText: `@${userB.login}` });
      await expect(knownUser).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(switcherResults(dialog)).toHaveCount(1);
      await knownUser.click();
      await expect(page).toHaveURL(new RegExp(`/chat/-/${roomId}$`));
      const reopened = await openSwitcher(page);
      await switcherInput(reopened).fill(userB.login);

      const body = 'First message in quick finder conversation';
      await postMessageViaConnect(page, roomId, body);
      const result = switcherResults(dialog).filter({ hasText: userB.displayName });
      await expect(result).toBeVisible({ timeout: TIMEOUTS.REALTIME_EVENT });
      await expect(result).not.toContainText(`@${userB.login}`);
      await expect(switcherResults(dialog)).toHaveCount(1);

      const startedDMs: string[] = [];
      page.on('request', (request) => {
        if (request.url().endsWith('/chatto.api.v1.RoomService/StartDM'))
          startedDMs.push(request.url());
      });
      await result.click();
      await expect(page).toHaveURL(new RegExp(`/chat/-/${roomId}$`));
      await expect(page.getByText(body, { exact: true })).toBeVisible();

      expect(memberSearches).toEqual([]);
      expect(startedDMs).toEqual([]);
    });
  });
});
