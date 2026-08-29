import { test, expect } from './setup';
import { createAndLoginTestUser, reloadWithProductComposerDefaults } from './fixtures/testUser';
import {
  connectRemoteInstance,
  createUserOnRemote,
  startSecondServer,
  stopSecondServer
} from './fixtures/multiServer';
import { TIMEOUTS } from './constants';
import * as routes from './routes';

test.describe('App and User Preferences', () => {
  test('opens unified Appearance settings from the Application Header', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.chat);
    await page.getByRole('link', { name: 'App Preferences' }).click();
    await page.waitForURL(routes.settingsAppearance);
    await expect(page.getByTestId('server-sidebar')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
    await expect(page.getByText('App preferences', { exact: true })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Appearance' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Language' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Composer' })).toBeVisible();
  });

  test('can choose a local display theme', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'light' });
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsAppearance);
    await expect(page.getByRole('heading', { name: 'Appearance' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    const systemOption = page.getByRole('radio', { name: /System/ });
    const lightOption = page.getByRole('radio', { name: /Light/ });
    const darkOption = page.getByRole('radio', { name: /Dark/ });

    await expect(systemOption).toHaveAttribute('aria-checked', 'true');

    await darkOption.click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('html')).toHaveCSS('color-scheme', 'dark');

    await page.reload();
    await expect(darkOption).toHaveAttribute('aria-checked', 'true');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('html')).toHaveCSS('color-scheme', 'dark');

    await lightOption.click();
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    await expect(page.locator('html')).toHaveCSS('color-scheme', 'light');

    await systemOption.click();
    await expect(systemOption).toHaveAttribute('aria-checked', 'true');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');
    await expect(page.locator('html')).toHaveCSS('color-scheme', 'light');

    await page.emulateMedia({ colorScheme: 'dark' });
    await page.reload();
    await expect(systemOption).toHaveAttribute('aria-checked', 'true');
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
    await expect(page.locator('html')).toHaveCSS('color-scheme', 'dark');
  });

  test('can choose and persist browser-wide composer preferences', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsComposer);
    await expect(page.getByRole('heading', { name: 'Composer' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    const markdown = page.getByRole('radio', { name: /^Markdown/ });
    const returnToSend = page.getByRole('radio', { name: /^Return/ });
    await markdown.click();
    await returnToSend.click();
    await expect(markdown).toHaveAttribute('aria-checked', 'true');
    await expect(returnToSend).toHaveAttribute('aria-checked', 'true');
    await expect
      .poll(() =>
        page.evaluate(() => JSON.parse(localStorage.getItem('chatto:preferences') ?? '{}'))
      )
      .toMatchObject({ composerEditor: 'markdown', composerSendMode: 'enter' });

    await page.reload();
    await expect(markdown).toHaveAttribute('aria-checked', 'true');
    await expect(returnToSend).toHaveAttribute('aria-checked', 'true');
  });

  test('defaults to Markdown and Return-to-send when composer preferences are absent', async ({
    page
  }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsComposer);
    await expect(page.getByRole('heading', { name: 'Composer' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    await reloadWithProductComposerDefaults(page);

    await expect(page.getByRole('radio', { name: /^Markdown/ })).toHaveAttribute(
      'aria-checked',
      'true'
    );
    await expect(page.getByRole('radio', { name: /^Return/ })).toHaveAttribute(
      'aria-checked',
      'true'
    );
  });

  test('uses the same composer preferences on a connected server', async ({
    page,
    chatPage
  }, testInfo) => {
    const remoteServer = await startSecondServer(testInfo);
    try {
      await createAndLoginTestUser(page);
      await page.goto(routes.settingsComposer);
      await page.getByRole('radio', { name: /^Markdown/ }).click();
      await page.getByRole('radio', { name: /^Return/ }).click();
      await chatPage.goto();

      const baseURL = remoteServer.baseURL.replace('localhost', '127.0.0.1');
      const remoteUser = await createUserOnRemote(
        baseURL,
        'remote-editor-preference',
        'password123'
      );
      await connectRemoteInstance(page, { ...remoteServer, baseURL }, remoteUser.userId);

      await page.getByRole('link', { name: 'App Preferences' }).click();
      await page.waitForURL(/\/chat\/[^/]+\/settings\/appearance$/);
      await page.getByRole('link', { name: 'Composer' }).click();
      await page.waitForURL(/\/chat\/[^/]+\/settings\/composer$/);
      await expect(page.getByRole('radio', { name: /^Markdown/ })).toHaveAttribute(
        'aria-checked',
        'true'
      );
      await expect(page.getByRole('radio', { name: /^Return/ })).toHaveAttribute(
        'aria-checked',
        'true'
      );
    } finally {
      await stopSecondServer(remoteServer, testInfo);
    }
  });

  test('can choose and persist a regional locale', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsLanguage);
    await expect(page.getByRole('heading', { name: 'Language', level: 1 })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    const pageMarker = await page.evaluate(() => {
      const marker = `locale-switch-${Date.now()}`;
      (
        window as typeof window & { __chattoLocaleSwitchMarker?: string }
      ).__chattoLocaleSwitchMarker = marker;
      return marker;
    });

    await page.getByRole('radio', { name: 'German (Germany)' }).click();
    await expect(page.getByRole('heading', { name: 'Einstellungen' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(page.getByRole('heading', { name: 'Sprache', level: 1 })).toBeVisible();
    await expect(page.locator('html')).toHaveAttribute('lang', 'de-DE');
    await expect
      .poll(() =>
        page.evaluate(
          () =>
            (window as typeof window & { __chattoLocaleSwitchMarker?: string })
              .__chattoLocaleSwitchMarker
        )
      )
      .toBe(pageMarker);

    await page.reload();
    await expect(page.getByRole('heading', { name: 'Einstellungen' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(page.getByRole('radio', { name: 'Deutsch (Deutschland)' })).toHaveAttribute(
      'aria-checked',
      'true'
    );

    await page.getByRole('radio', { name: 'Englisch (Vereinigte Staaten)' }).click();
    await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(page.getByRole('heading', { name: 'Language', level: 1 })).toBeVisible();
    await expect(page.locator('html')).toHaveAttribute('lang', 'en-US');

    await page.reload();
    await expect(page.getByRole('radio', { name: 'American English' })).toHaveAttribute(
      'aria-checked',
      'true'
    );
  });

  test('can set timezone and save', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsTime);
    await expect(page.getByRole('heading', { name: 'Time & region', level: 1 })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Type a timezone
    const timezoneInput = page.getByTestId('timezone-input');
    await timezoneInput.fill('Europe/Berlin');

    // Save button should be enabled
    const saveButton = page.getByRole('button', { name: 'Save Time Settings' });
    await expect(saveButton).toBeEnabled({ timeout: TIMEOUTS.UI_STANDARD });
    await saveButton.click();

    // Should see success toast
    await expect(page.getByText('Time settings saved')).toBeVisible({
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
    await page.goto(routes.settingsTime);
    await expect(page.getByRole('heading', { name: 'Time & region', level: 1 })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    const timezoneInput = page.getByTestId('timezone-input');
    await timezoneInput.fill('Europe/Berlin');

    const currentTime = page.getByText(/^Current time there:/);
    await page.getByRole('radio', { name: '12-hour' }).click();
    await expect(currentTime).toContainText(/[AP]M$/);

    // Select 24-hour format and verify the unsaved preview updates.
    await page.getByRole('radio', { name: '24-hour' }).click();
    await expect(currentTime).not.toContainText(/[AP]M$/);

    // Save
    const saveButton = page.getByRole('button', { name: 'Save Time Settings' });
    await expect(saveButton).toBeEnabled({ timeout: TIMEOUTS.UI_STANDARD });
    await saveButton.click();

    await expect(page.getByText('Time settings saved')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Reload and verify the 24-hour option is still selected
    await page.reload();
    // The selected option has a filled radio indicator
    const twentyFourHourOption = page.getByRole('radio', { name: '24-hour' });
    await expect(twentyFourHourOption).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    // Verify it has the shared selected-row styling.
    await expect(twentyFourHourOption).toHaveClass(/choice-row-selected/, {
      timeout: TIMEOUTS.UI_STANDARD
    });
  });

  test('can clear timezone back to browser default', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsTime);
    await expect(page.getByRole('heading', { name: 'Time & region', level: 1 })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // First set a timezone
    const timezoneInput = page.getByTestId('timezone-input');
    await timezoneInput.fill('America/New_York');

    const saveButton = page.getByRole('button', { name: 'Save Time Settings' });
    await saveButton.click();
    await expect(page.getByText('Time settings saved')).toBeVisible({
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
    await expect(page.getByText('Time settings saved')).toBeVisible({
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
    await page.goto(routes.settingsTime);
    await expect(page.getByRole('heading', { name: 'Time & region', level: 1 })).toBeVisible({
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
    const saveButton = page.getByRole('button', { name: 'Save Time Settings' });
    await expect(saveButton).toBeDisabled();
  });

  test('unified Settings sidebar exposes the three ordered scope groups', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.settingsProfile);

    const groups = page.getByTestId('room-group-section');
    await expect(groups).toHaveCount(3);
    await expect(groups.nth(0)).toContainText('App preferences');
    await expect(groups.nth(1)).toContainText('Your account');
    await expect(groups.nth(2)).toContainText('Server');
    await expect(page.getByText('Your account', { exact: true })).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });
    await expect(page.getByText('Server', { exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Appearance' })).toHaveAttribute(
      'href',
      routes.settingsAppearance
    );
    await expect(page.getByRole('link', { name: 'Time & region' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Bots', exact: true })).toBeVisible();
  });
});
