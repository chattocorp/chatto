import { completeSignup } from './fixtures/signup';
import { expect, test } from './setup';

test('centers focused cards and keeps tall forms reachable on short screens', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 900 });
  for (const path of ['/login', '/signup', '/password-reset']) {
    await page.goto(path);
    const card = await page.locator('.focus-card').boundingBox();
    const header = await page.locator('body > header').boundingBox();
    expect(card).not.toBeNull();
    expect(header).not.toBeNull();
    expect(Math.abs(card!.x + card!.width / 2 - 600)).toBeLessThan(1);
    expect(Math.abs(card!.y + card!.height / 2 - (900 + header!.height) / 2)).toBeLessThan(1);
  }
  await page.setViewportSize({ width: 320, height: 300 });
  await page.goto('/login');
  await page.evaluate(() => scrollTo(0, 0));
  const card = await page.locator('.focus-card').boundingBox();
  const header = await page.locator('body > header').boundingBox();
  expect(card!.y).toBeGreaterThanOrEqual(header!.height);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.getByRole('button', { name: 'Sign in', exact: true }).scrollIntoViewIfNeeded();
  await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeInViewport();
});

test('uses the site identity and keeps account settings usable on small screens', async ({
  page,
  request,
  stack
}) => {
  await page.goto('/');
  await expect(page).toHaveURL(/\/login$/);
  await expect(page).toHaveTitle('Sign in · Test Accounts');
  await expect(page.getByRole('heading', { name: 'Sign in', exact: true })).toBeVisible();
  await expect(page.locator('meta[name="description"]')).toHaveAttribute(
    'content',
    'Your test account.'
  );
  await page.getByRole('link', { name: 'Skip to content' }).focus();
  await expect(page.getByRole('link', { name: 'Skip to content' })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('main')).toBeFocused();

  await completeSignup(
    page,
    request,
    stack,
    'site-test@example.invalid',
    'correct horse battery staple'
  );
  await expect(page).toHaveTitle('Your account · Test Accounts');
  const container = await page.locator('.page-main > div').boundingBox();
  for (const panel of await page.locator('.panel').all()) {
    expect((await panel.boundingBox())!.width).toBeCloseTo(container!.width, 0);
  }
  for (const name of ['Profile', 'Security', 'Connected apps', 'Sessions', 'Delete account']) {
    await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
  }
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme, reducedMotion: 'reduce' });
    await page.setViewportSize({ width: 320, height: 740 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true
    );
    await expect(
      page.getByRole('link', { name: 'Signed in as site-test@example.invalid. View your account.' })
    ).toBeVisible();
  }
  await page.getByRole('link', { name: 'Edit profile', exact: true }).click();
  await expect(
    page.getByRole('textbox', { name: 'Preferred username', exact: true })
  ).toBeFocused();
  await expect(
    page.getByRole('textbox', { name: 'Preferred username', exact: true })
  ).toHaveAccessibleDescription('This does not have to be unique.');
  await page.getByRole('link', { name: 'Return to your account', exact: true }).click();
  await page.getByRole('link', { name: 'Delete account', exact: true }).click();
  await expect(page.getByRole('checkbox')).toHaveAccessibleName(
    'I understand that this permanently deletes my account on Test Accounts.'
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
