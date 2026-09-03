import { expect } from '@playwright/test';
import { test } from './setup';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import * as routes from './routes';

test('owner explicitly activates and deactivates privileged mode', async ({ page }) => {
  await page.goto(routes.root);
  await loginAsAdminAndUsePrimaryServer(page, { activatePrivilegedMode: false });
  await page.goto(routes.chat);

  const enable = page.getByRole('button', { name: 'Enable privileged mode' });
  await expect(enable).toBeVisible();
  await expect(page.getByRole('button', { name: 'New Group' })).not.toBeVisible();

  await enable.click();
  const dialog = page.getByRole('dialog', { name: 'Enable privileged mode' });
  await expect(dialog).toContainText(
    'For 15 minutes, this enables the additional permissions assigned to you on this server.'
  );
  await dialog.getByRole('button', { name: 'Enable privileged mode' }).click();

  const disable = page.getByRole('button', { name: 'Disable privileged mode' });
  await expect(disable).toBeVisible();
  await expect(page.getByRole('button', { name: 'New Group' })).toBeVisible();

  await disable.click();
  await expect(enable).toBeVisible();
  await expect(page.getByRole('button', { name: 'New Group' })).not.toBeVisible();
});
