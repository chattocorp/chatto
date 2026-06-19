import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('User Settings - Display', () => {
  test('can navigate to display page', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsPreferences);
    await expect(page.getByRole('heading', { name: 'Display' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('can choose a local display theme', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' });
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsPreferences);
    await expect(page.getByRole('heading', { name: 'Display' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    const systemOption = page.getByRole('radio', { name: /System/ });
    const lightOption = page.getByRole('radio', { name: /Light/ });
    const darkOption = page.getByRole('radio', { name: /Dark/ });

    await expect(systemOption).toHaveAttribute('aria-checked', 'true');

    await darkOption.click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

    await page.reload();
    await expect(darkOption).toHaveAttribute('aria-checked', 'true');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

    await lightOption.click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');

    await systemOption.click();
    await expect(systemOption).toHaveAttribute('aria-checked', 'true');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');

    await page.emulateMedia({ colorScheme: 'dark' });
    await page.reload();
    await expect(systemOption).toHaveAttribute('aria-checked', 'true');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
  });

  test('can set timezone and save', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsPreferences);
    await expect(page.getByRole('heading', { name: 'Display' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Type a timezone
    const timezoneInput = page.getByTestId('timezone-input');
    await timezoneInput.fill('Europe/Berlin');

    // Save button should be enabled
    const saveButton = page.getByRole('button', { name: 'Save Display Settings' });
    await expect(saveButton).toBeEnabled({ timeout: TIMEOUTS.UI_STANDARD });
    await saveButton.click();

    // Should see success toast
    await expect(page.getByText('Display settings saved')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Reload and verify persistence
    await page.reload();
    await expect(timezoneInput).toHaveValue('Europe/Berlin', {
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('can set time format to 24-hour and save', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsPreferences);
    await expect(page.getByRole('heading', { name: 'Display' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Select 24-hour format
    await page.getByRole('button', { name: '24-hour' }).click();

    // Save
    const saveButton = page.getByRole('button', { name: 'Save Display Settings' });
    await expect(saveButton).toBeEnabled({ timeout: TIMEOUTS.UI_STANDARD });
    await saveButton.click();

    await expect(page.getByText('Display settings saved')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Reload and verify the 24-hour option is still selected
    await page.reload();
    // The selected option has a filled radio indicator
    const twentyFourHourButton = page.getByRole('button', { name: '24-hour' });
    await expect(twentyFourHourButton).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    // Verify it has the accent/selected styling
    await expect(twentyFourHourButton).toHaveClass(/border-accent/, {
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('can clear timezone back to browser default', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsPreferences);
    await expect(page.getByRole('heading', { name: 'Display' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // First set a timezone
    const timezoneInput = page.getByTestId('timezone-input');
    await timezoneInput.fill('America/New_York');

    const saveButton = page.getByRole('button', { name: 'Save Display Settings' });
    await saveButton.click();
    await expect(page.getByText('Display settings saved')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Dismiss the first toast before triggering a second save
    await page.getByRole('button', { name: 'Dismiss notification' }).click();

    // Now clear it using the X button
    await page.getByTitle('Clear timezone (use browser default)').click();
    await expect(timezoneInput).toHaveValue('');

    // Save again
    await expect(saveButton).toBeEnabled({ timeout: TIMEOUTS.UI_STANDARD });
    await saveButton.click();
    await expect(page.getByText('Display settings saved')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Reload and verify it's cleared
    await page.reload();
    await expect(timezoneInput).toHaveValue('', {
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('shows validation error for invalid timezone', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsPreferences);
    await expect(page.getByRole('heading', { name: 'Display' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Type an invalid timezone
    const timezoneInput = page.getByTestId('timezone-input');
    await timezoneInput.fill('Not/A/Timezone');

    // Should show validation error
    await expect(page.getByText('Please select a valid timezone')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Save button should be disabled
    const saveButton = page.getByRole('button', { name: 'Save Display Settings' });
    await expect(saveButton).toBeDisabled();
  });

  test('display nav item is visible in settings sidebar', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settings);

    // Check that Display is in the nav
    await expect(page.getByRole('link', { name: 'Display' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
  });
});
