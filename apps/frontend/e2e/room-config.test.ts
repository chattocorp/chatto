import { expect } from '@playwright/test';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import * as routes from './routes';
import { test } from './setup';

test.describe('Room configuration', () => {
  test('the server layer can be changed and returned to the product default', async ({ page }) => {
    await loginAsAdminAndUsePrimaryServer(page);
    await page.goto(routes.serverAdminRoomConfig);

    const editWindow = page.getByRole('combobox', { name: 'Author edit window' });
    const effectiveValue = page.getByText('Effective value').locator('..');
    await expect(editWindow).toHaveValue('inherit');
    await expect(effectiveValue).toContainText('3 hours');
    await expect(effectiveValue).toContainText('Inherit');

    await editWindow.selectOption('1800');
    await expect(page.getByText('Saved')).toBeVisible();
    await expect(effectiveValue).toContainText('30 minutes');
    await expect(effectiveValue).not.toContainText('Inherit');

    await page.reload();
    await expect(editWindow).toHaveValue('1800');
    await expect(effectiveValue).toContainText('30 minutes');
    await expect(effectiveValue).not.toContainText('Inherit');

    await editWindow.selectOption('inherit');
    await expect(page.getByText('Saved')).toBeVisible();
    await expect(effectiveValue).toContainText('3 hours');
    await expect(effectiveValue).toContainText('Inherit');
  });
});
