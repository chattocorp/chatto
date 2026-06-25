import { test } from './setup';
import { SettingsPage } from './pages/SettingsPage';
import { createAndLoginTestUser } from './fixtures/testUser';

test.describe('Profile Settings', () => {
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
