import { expect } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { waitForRoomReady } from './fixtures/realtimeSync';

test('shortcut help preserves editor text, selection, and focus', async ({
  page,
  chatPage,
  roomPage
}) => {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  await createAndLoginTestUser(page);
  await chatPage.goto();
  await chatPage.enterRoom('general');
  await waitForRoomReady(page, 'general');
  await roomPage.waitForInputEditable();
  await roomPage.messageInput.fill('Keep this selected draft');
  await roomPage.messageInput.press('ControlOrMeta+A');
  const selected = await page.evaluate(() => window.getSelection()?.toString());

  await page.keyboard.press('ControlOrMeta+Shift+K');
  const dialog = page.getByRole('dialog', { name: 'Keyboard shortcuts' });
  await expect(dialog).toBeVisible();
  await expect(
    dialog.getByRole('button', { name: 'Esc', exact: true })
  ).toHaveAccessibleDescription('Close');
  await expect(dialog.getByText('Open quick switcher', { exact: true })).toBeVisible();
  await expect(dialog.getByText('Send message', { exact: true })).toBeVisible();
  await expect(dialog.getByText('Bold', { exact: true })).toBeVisible();
  await expect(dialog.getByText('Undo', { exact: true })).toBeVisible();
  await expect(dialog.getByRole('textbox', { name: 'Search', exact: true })).toBeFocused();
  await page.keyboard.press('PageDown');
  await expect
    .poll(() => dialog.locator('.dialog-body').evaluate((node) => node.scrollTop))
    .toBeGreaterThan(0);
  await dialog.getByRole('textbox', { name: 'Search', exact: true }).fill('Bold');
  await expect(dialog.getByText('Bold', { exact: true })).toBeVisible();
  await expect(dialog.getByText('Undo', { exact: true })).not.toBeVisible();
  await page.keyboard.press('Escape');
  await expect(dialog).not.toBeVisible();
  await expect(roomPage.messageInput).toBeFocused();
  await expect(roomPage.messageInput).toHaveText('Keep this selected draft');
  expect(await page.evaluate(() => window.getSelection()?.toString())).toBe(selected);

  await page.keyboard.press('ControlOrMeta+k');
  const paletteStyle = await page.locator('dialog[open] .command-palette').evaluate((node) => {
    const style = getComputedStyle(node);
    return { width: style.width, font: style.font, padding: style.padding };
  });
  await expect(page.getByRole('button', { name: 'Keyboard shortcuts', exact: true })).toHaveCount(
    0
  );
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: 'Quick switcher', exact: true })).not.toBeVisible();
  await page.keyboard.press('ControlOrMeta+Shift+K');
  await expect(dialog).toBeVisible();
  expect(
    await dialog.locator('.command-palette').evaluate((node) => {
      const style = getComputedStyle(node);
      return {
        width: style.width,
        font: style.font,
        padding: style.padding
      };
    })
  ).toEqual(paletteStyle);
  await expect(dialog.getByText('Go to next unread source', { exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(dialog).not.toBeVisible();
  await expect(roomPage.messageInput).toBeFocused();
  expect(await page.evaluate(() => window.getSelection()?.toString())).toBe(selected);

  await page.keyboard.press('ControlOrMeta+b');
  await expect(roomPage.messageInput.locator('strong')).toHaveText('Keep this selected draft');
  expect(errors).toEqual([]);
});
