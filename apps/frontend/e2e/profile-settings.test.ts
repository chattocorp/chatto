import { test } from './setup';
import { SettingsPage } from './pages/SettingsPage';
import { createAndLoginTestUser } from './fixtures/testUser';
import { expect, type Page } from '@playwright/test';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

async function setCustomStatus(page: Page, text: string): Promise<void> {
  await page.getByTestId('current-user-presence-menu').click();
  await page.getByTestId('current-user-custom-status-action').click();
  await page.getByRole('textbox', { name: 'Status text', exact: true }).fill(text);
  await page.getByRole('button', { name: 'Save status', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Set a status' })).not.toBeVisible();
}

test.describe('Profile Settings', () => {
  test('cancels and confirms a username change with the current profile draft', async ({
    page
  }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settings);
    const username = page.getByTestId('settings-username');
    const renamed = `renamed${Date.now()}`;
    const settings = new SettingsPage(page);
    await username.fill(`${renamed}x`);
    await settings.displayNameInput.fill('Updated profile');
    await settings.saveDisplayNameButton.click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toContainText(`@${renamed}x`);
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await expect(username).toHaveValue(`${renamed}x`);
    await expect(settings.displayNameInput).toHaveValue('Updated profile');

    await username.fill(renamed);
    await settings.saveDisplayNameButton.click();
    await expect(dialog).toContainText(`@${renamed}?`);
    await dialog.getByRole('button', { name: 'Change username', exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(page.getByText('Profile updated')).toBeVisible();
    await expect(settings.saveDisplayNameButton).toBeDisabled();
    await page.reload();
    await expect(username).toHaveValue(renamed);
    await expect(settings.displayNameInput).toHaveValue('Updated profile');
  });

  test('opens settings from own profile menus and returns with updated identity', async ({
    page,
    chatPage
  }) => {
    const user = await createAndLoginTestUser(page);
    await chatPage.goto();
    const previousURL = page.url();
    await page.getByTestId('current-user-identity-card').click({ button: 'right' });
    const menu = page.getByRole('dialog', { name: 'User profile', exact: true });
    const edit = menu.getByRole('link', { name: 'Edit profile', exact: true });
    await expect(edit).toHaveAttribute('href', routes.settings);
    await edit.focus();
    await page.keyboard.press('Enter');
    await expect(page).toHaveURL(routes.settings);
    await expect(menu).not.toBeVisible();

    const settings = new SettingsPage(page);
    await settings.updateDisplayName('Updated profile shortcut');
    await page.goBack();
    await expect(page).toHaveURL(previousURL);
    await expect(page.getByTestId('current-user-identity-card')).toContainText(
      'Updated profile shortcut'
    );

    await page.goto(routes.user(user.id!));
    await page.getByTestId('profile-user-menu-button').click();
    await expect(edit).toBeVisible();
    await edit.click();
    await expect(page).toHaveURL(routes.settings);
    await expect(menu).not.toBeVisible();
    await settings.expectDisplayNameValue('Updated profile shortcut');
  });

  test('syncs custom status changes and clearing across clients of the same user', async ({
    page,
    chatPage,
    browser,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    const device = await browser.newContext({
      baseURL: serverURL,
      storageState: await page.context().storageState()
    });
    const other = await device.newPage();
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    other.on('pageerror', (error) => errors.push(error.message));
    try {
      await other.goto(routes.chat);
      await expect(other.getByTestId('current-user-presence-menu')).toBeVisible();
      await setCustomStatus(page, 'First device status');
      const otherCard = other.getByTestId('current-user-identity-card');
      await expect(otherCard.getByRole('img', { name: /First device status/ })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      await other.getByTestId('current-user-presence-menu').click();
      await expect(other.getByTestId('current-user-clear-status-action')).toBeVisible();
      await other.getByTestId('current-user-custom-status-action').click();
      await expect(other.getByRole('textbox', { name: 'Status text', exact: true })).toHaveValue(
        'First device status'
      );
      await other
        .getByRole('textbox', { name: 'Status text', exact: true })
        .fill('Second device status');
      await other.getByRole('button', { name: 'Save status', exact: true }).click();
      const firstCard = page.getByTestId('current-user-identity-card');
      await expect(firstCard.getByRole('img', { name: /Second device status/ })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      // Keep the receiver's menu open to exercise its live conditional action.
      await page.getByTestId('current-user-presence-menu').click();
      await expect(page.getByTestId('current-user-clear-status-action')).toBeVisible();
      await other.getByTestId('current-user-presence-menu').click();
      await other.getByTestId('current-user-clear-status-action').click();
      await expect(firstCard.getByRole('img', { name: /Second device status/ })).not.toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      await expect(page.getByTestId('current-user-clear-status-action')).not.toBeVisible();
      await expect(page.getByTestId('current-user-custom-status-action')).toBeVisible();
      await page.getByTestId('current-user-custom-status-action').click();
      await expect(page.getByRole('textbox', { name: 'Status text', exact: true })).toHaveValue('');
      expect(errors).toEqual([]);
    } finally {
      await device.close();
    }
  });

  test('display name update persists across page reload', async ({ page }) => {
    await createAndLoginTestUser(page);
    const settingsPage = new SettingsPage(page);
    await settingsPage.goto();

    const newName = `Updated Name ${Date.now()}`;
    await settingsPage.updateDisplayName(newName);

    await page.reload();
    await settingsPage.expectDisplayNameValue(newName);
  });
});
