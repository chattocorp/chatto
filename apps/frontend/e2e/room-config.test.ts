import { expect } from '@playwright/test';
import { loginAsAdminAndUsePrimaryServer } from './fixtures/testUser';
import * as routes from './routes';
import { test } from './setup';

test.describe('Room configuration', () => {
  test('the server layer can be changed and returned to the product default', async ({ page }) => {
    await loginAsAdminAndUsePrimaryServer(page);
    await page.goto(routes.serverAdminRoomConfig);

    const editWindow = page.getByRole('combobox', { name: 'Author edit window' });
    await expect(editWindow).toHaveValue('inherit');
    await expect(page.getByText('3 hours · Product default')).toBeVisible();

    await editWindow.selectOption('1800');
    await expect(page.getByText('Saved')).toBeVisible();
    await expect(page.getByText('30 minutes · Server')).toBeVisible();

    await page.reload();
    await expect(editWindow).toHaveValue('1800');
    await expect(page.getByText('30 minutes · Server')).toBeVisible();

    await editWindow.selectOption('inherit');
    await expect(page.getByText('Saved')).toBeVisible();
    await expect(page.getByText('3 hours · Product default')).toBeVisible();
  });
});
