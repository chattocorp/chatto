import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';

/**
 * Opens the quick switcher palette via Cmd/Ctrl+K.
 * Returns the dialog locator.
 */
async function openSwitcher(page: import('@playwright/test').Page) {
  const isMac = process.platform === 'darwin';
  await page.keyboard.press(isMac ? 'Meta+k' : 'Control+k');
  const dialog = page.locator('dialog.quick-switcher');
  await expect(dialog).toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  return dialog;
}

/** Returns the search input inside the quick switcher. */
function switcherInput(dialog: import('@playwright/test').Locator) {
  return dialog.getByPlaceholder('Go to space, room, conversation, or user...');
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

  test('closes when clicking outside the dialog', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await chatPage.createSpace('Switcher Test');

    const dialog = await openSwitcher(page);

    // Click backdrop (top-left corner, outside the dialog content)
    await page.mouse.click(5, 5);
    await expect(dialog).not.toBeVisible({ timeout: TIMEOUTS.UI_FAST });
  });

  test('shows joined spaces and rooms', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();
    const roomName = await chatPage.createRoom();

    const dialog = await openSwitcher(page);

    // Wait for loading to finish
    await expect(dialog.locator('.animate-spin')).not.toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Should show the space
    await expect(
      dialog.getByRole('button', { name: new RegExp(`${spaceName}.*Space`) })
    ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // Should show the room with # prefix
    await expect(
      dialog.getByRole('button', { name: new RegExp(`#${roomName}.*Room`) })
    ).toBeVisible();
  });

  // FIXME: creates two separate spaces on the same server. Doesn't apply
  // post-collapse. Re-enable / remove in next phase-2 PR.
  test.skip('does not show spaces the user has not joined', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    // User 1 creates a space
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const spaceName = await chatPage.createSpace();

    // User 2 logs in separately (does NOT join the space)
    const context2 = await browser!.newContext({ baseURL: serverURL });
    const page2 = await context2.newPage();

    try {
      await createAndLoginTestUser(page2);
      const { ChatPage } = await import('./pages');
      const cp2 = new ChatPage(page2);
      await cp2.goto();
      // Create user 2's own space so the app loads properly
      await cp2.createSpace();

      const dialog = await openSwitcher(page2);

      // Wait for loading to finish
      await expect(dialog.locator('.animate-spin')).not.toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });

      // The space created by user 1 should NOT appear (user 2 hasn't joined it)
      await expect(
        switcherResults(dialog).filter({ hasText: spaceName }).filter({ hasText: 'Space' })
      ).not.toBeVisible();
    } finally {
      await context2.close();
    }
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
