import { completeSignup } from './fixtures/signup';
import { expect, test } from './setup';

test('uses the site identity and keeps account settings usable on small screens', async ({ page, request, stack }) => {
  await page.goto('/');
  await expect(page).toHaveTitle('Test Accounts');
  await expect(page.getByRole('heading', { name: 'Welcome to Test Accounts' })).toBeVisible();
  await expect(page.locator('meta[name="description"]')).toHaveAttribute('content', 'Your test account.');
  await page.keyboard.press('Tab');
  await expect(page.getByRole('link', { name: 'Skip to content' })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('main')).toBeFocused();

  await completeSignup(page, request, stack, 'site-test@example.invalid', 'correct horse battery staple');
  await expect(page).toHaveTitle('Your account · Test Accounts');
  for (const name of ['Profile', 'Security', 'Connected apps', 'Sessions', 'Delete account']) {
    await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
  }
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme, reducedMotion: 'reduce' });
    await page.setViewportSize({ width: 320, height: 740 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await expect(page.getByRole('link', { name: 'Signed in as site-test@example.invalid. View your account.' })).toBeVisible();
  }
  await page.getByRole('link', { name: 'Delete account', exact: true }).click();
  await expect(page.getByRole('checkbox')).toHaveAccessibleName('I understand that this permanently deletes my account on Test Accounts.');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
