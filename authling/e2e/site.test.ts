import { completeSignup } from './fixtures/signup';
import { expect, test } from './setup';

test('centers focused cards and keeps tall forms reachable on short screens', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 900 });
  for (const path of ['/login', '/signup']) {
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

for (const javaScriptEnabled of [true, false]) {
  test.describe(`account sections (JavaScript ${javaScriptEnabled ? 'on' : 'off'})`, () => {
    test.use({ javaScriptEnabled });
    test('keeps sections separate and account tasks full width', async ({
      page,
      request,
      stack
    }) => {
      await completeSignup(
        page,
        request,
        stack,
        'account-sections@example.invalid',
        'correct horse battery staple'
      );
      const sections = page.getByRole('navigation', { name: 'Account sections' });
      for (const [name, path] of [
        ['Profile', '/account'],
        ['Security', '/account/security'],
        ['Connected apps', '/account/apps'],
        ['Sessions', '/account/sessions']
      ]) {
        await sections.getByRole('link', { name, exact: true }).click();
        await expect(page).toHaveURL(`${stack.baseURL}${path}`);
        await expect(sections.locator('[aria-current="page"]')).toHaveAccessibleName(name);
        await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
        await expect(page.locator('.focus-card')).toHaveCount(0);
        if (name !== 'Sessions')
          await expect(page.getByText('Browser session', { exact: true })).toHaveCount(0);
        if (name !== 'Connected apps')
          await expect(page.getByText('You haven’t authorized any apps yet.')).toHaveCount(0);
      }
      await page.goBack();
      await expect(sections.locator('[aria-current="page"]')).toHaveAccessibleName(
        'Connected apps'
      );
      for (const path of [
        '/account/profile',
        '/account/email',
        '/account/password',
        '/account/delete'
      ]) {
        await page.goto(path);
        await expect(page.locator('.focus-card')).toHaveCount(0);
        await expect(sections).toBeVisible();
        await expect(page.getByRole('heading', { level: 1 })).toHaveCount(1);
        const main = await page.locator('.account-section').boundingBox();
        expect((await page.locator('.panel').boundingBox())!.width).toBeCloseTo(main!.width, 0);
      }
      await page.goto('/password-reset');
      await expect(page.locator('.focus-card')).toHaveCount(0);
      await expect(page.locator('.panel')).toBeVisible();
    });
  });
}

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
  const container = await page.locator('.account-section').boundingBox();
  for (const panel of await page.locator('.panel').all()) {
    expect((await panel.boundingBox())!.width).toBeCloseTo(container!.width, 0);
  }
  await expect(page.getByRole('heading', { name: 'Profile', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Security', exact: true })).toHaveCount(0);
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
  await page.getByRole('link', { name: 'Security', exact: true }).click();
  await page.getByRole('link', { name: 'Delete account', exact: true }).click();
  await expect(page.getByRole('checkbox')).toHaveAccessibleName(
    'I understand that this permanently deletes my account on Test Accounts.'
  );
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
