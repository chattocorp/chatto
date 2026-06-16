import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
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
  return dialog.getByPlaceholder('Go to server, room, or conversation...');
}

/** Returns all result buttons inside the quick switcher. */
function switcherResults(dialog: import('@playwright/test').Locator) {
  return dialog.locator('button.sidebar-item');
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

  test('surfaces users without an existing DM', async ({ page, chatPage, browser, serverURL }) => {
    // Two users on the same deployment. createAndLoginTestUser auto-joins
    // the bootstrap primary space, so A and B share a space — pre-fix,
    // this is exactly what made B show up in QuickSwitcherSpaceMembersSearch.
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const context2 = await browser.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();
    try {
      const userB = await createAndLoginTestUser(page2);

      const dialog = await openSwitcher(page);
      const input = switcherInput(dialog);

      // Wait for the initial load to settle.
      await expect(switcherResults(dialog).first()).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });

      // userB.login is unique enough not to fuzzy-match any space, room, or
      // destination label. With no DM open with userB, this proves Cmd-K is
      // searching the server member directory rather than just existing DMs.
      await input.fill(userB.login);

      await expect(switcherResults(dialog).filter({ hasText: userB.login })).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });
    } finally {
      await context2.close();
    }
  });
});
