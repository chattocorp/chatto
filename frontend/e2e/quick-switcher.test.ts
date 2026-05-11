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
  return dialog.getByPlaceholder('Go to space, room, or conversation...');
}

/** Returns all result buttons inside the quick switcher. */
function switcherResults(dialog: import('@playwright/test').Locator) {
  return dialog.locator('button.sidebar-item');
}

test.describe('Quick Switcher (Cmd-K)', () => {
  test('opens with Cmd-K and closes with Escape', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Switcher Test');

    // Open
    const dialog = await openSwitcher(page);
    await expect(switcherInput(dialog)).toBeFocused();

    // Close with Escape
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
    await chatPage.createSpace('Switcher Test');

    const dialog = await openSwitcher(page);

    // Click backdrop (top-left corner, outside the dialog content)
    await page.mouse.click(5, 5);
    await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  });

  test('shows joined rooms', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    void (await chatPage.createSpace());
    const roomName = await chatPage.createRoom();

    const dialog = await openSwitcher(page);

    // Wait for loading to finish
    await expect(dialog.locator('.animate-spin')).not.toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Post-#330 PR(a) the QuickSwitcher no longer surfaces a "Space" tier —
    // every joined room shows up as a Room entry directly.
    await expect(
      dialog.getByRole('button', { name: new RegExp(`#${roomName}.*Room`) })
    ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('fuzzy search filters results', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    await chatPage.createRoom('xylophone-chat');

    const dialog = await openSwitcher(page);
    const input = switcherInput(dialog);
    const results = switcherResults(dialog);

    // Wait for data to load
    await expect(results.first()).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    const totalCount = await results.count();

    // Search for a specific room — should narrow results
    await input.fill('xylophone');
    await expect(results.filter({ hasText: 'xylophone-chat' })).toBeVisible();
    const filteredCount = await results.count();
    expect(filteredCount).toBeLessThan(totalCount);

    // Search for something that doesn't exist
    await input.fill('zzzznothing');
    await expect(dialog.getByText('No results')).toBeVisible({
      timeout: TIMEOUTS.UI_FAST
    });
  });

  test('# prefix filters to rooms only', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const roomName = await chatPage.createRoom();

    const dialog = await openSwitcher(page);
    const input = switcherInput(dialog);
    const results = switcherResults(dialog);

    // Wait for data to load (no filter — shows spaces + rooms)
    await expect(results.first()).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    const countBefore = await results.count();
    // Should have at least a space + default rooms + created room
    expect(countBefore).toBeGreaterThan(1);

    // Type "#" — should filter to rooms only (fewer results than unfiltered)
    await input.fill('#');
    await expect(results.filter({ hasText: `#${roomName}` })).toBeVisible();
    const countAfter = await results.count();
    expect(countAfter).toBeLessThan(countBefore);
  });

  test('shows "No results" for non-matching query', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();

    const dialog = await openSwitcher(page);
    const input = switcherInput(dialog);

    // Wait for data to load, then search for something that doesn't exist
    await expect(switcherResults(dialog).first()).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await input.fill('zzzznonexistent');

    await expect(dialog.getByText('No results')).toBeVisible({
      timeout: TIMEOUTS.UI_FAST
    });
  });

  test('keyboard navigation: arrow keys and Enter', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const roomName = await chatPage.createRoom();

    const dialog = await openSwitcher(page);
    const input = switcherInput(dialog);

    // Wait for results
    await expect(switcherResults(dialog).first()).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Filter to just the room
    await input.fill(`#${roomName}`);
    await expect(switcherResults(dialog).filter({ hasText: roomName })).toBeVisible();

    // Press Enter to navigate
    await page.keyboard.press('Enter');

    // Dialog should close
    await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Should have navigated to the room
    await expect(page.getByRole('heading', { name: `# ${roomName}` })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('clicking a result navigates to it', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace();
    const roomName = await chatPage.createRoom();

    const dialog = await openSwitcher(page);

    // Wait for results
    await expect(switcherResults(dialog).first()).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Click the room result
    await switcherResults(dialog).filter({ hasText: roomName }).click();

    // Dialog should close
    await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });

    // Should have navigated to the room
    await expect(page.getByRole('heading', { name: `# ${roomName}` })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('does not surface users without an existing DM', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
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
      // destination label. With no DM open with userB, the only thing that
      // could surface them is the (now-removed) user-search code path.
      await input.fill(userB.login);

      await expect(dialog.getByText('No results')).toBeVisible({
        timeout: TIMEOUTS.UI_FAST
      });
    } finally {
      await context2.close();
    }
  });

  test('navigating to a space works', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    // Create a room so we're inside the space, then open switcher
    await chatPage.createRoom();

    const dialog = await openSwitcher(page);
    const input = switcherInput(dialog);

    // Wait for results
    await expect(switcherResults(dialog).first()).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Search for the space and click it
    await input.fill(spaceName);
    await switcherResults(dialog).filter({ hasText: spaceName }).first().click();

    // Dialog should close
    await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  });
});
