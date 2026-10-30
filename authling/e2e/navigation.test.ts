import { expect, test } from './setup';

test('animates same-document navigation and history unless reduced motion is enabled', async ({
  page
}) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' });
  await page.goto('/login');
  await expect(page.locator('form')).toHaveAttribute('hx-boost', 'true');
  await page.evaluate(() => {
    document.documentElement.dataset.transitions = '0';
    document.addEventListener('htmx:beforeTransition', () => {
      const root = document.documentElement;
      root.dataset.transitions = String(Number(root.dataset.transitions) + 1);
    });
  });
  await page.getByRole('link', { name: 'Create account', exact: true }).click();
  await expect(page).toHaveURL(/\/signup$/);
  await expect(page.locator('html')).toHaveAttribute('data-transitions', '1');
  await page.goBack();
  await expect(page.getByRole('textbox', { name: 'Password', exact: true })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-transitions', '2');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.getByRole('link', { name: 'Create account', exact: true }).click();
  await expect(page).toHaveURL(/\/signup$/);
  await expect(page.locator('html')).toHaveAttribute('data-transitions', '2');
});

test('asset changes fall back to a fresh document', async ({ page }) => {
  await page.goto('/login');
  await expect(page.locator('form')).toHaveAttribute('hx-boost', 'true');
  await page.evaluate(() => {
    document.documentElement.dataset.navigationMarker = 'original';
  });
  await page.route('**/signup', async (route) => {
    if (route.request().headers()['hx-boosted'] !== 'true') return route.continue();
    const response = await route.fetch();
    await route.fulfill({
      response,
      body: (await response.text()).replace(
        /\/assets\/[^/]+\/app.css/,
        '/assets/new-version/app.css'
      )
    });
  });
  await page.getByRole('link', { name: 'Create account', exact: true }).click();
  await expect(page).toHaveURL(/\/signup$/);
  await expect(page.getByRole('heading', { name: 'Create your account' })).toBeVisible();
  await expect(page.locator('html')).not.toHaveAttribute('data-navigation-marker');
});

test('ordinary navigation and history keep the document without caching page contents', async ({
  page
}) => {
  await page.goto('/');
  await expect(page.locator('script[src$="/app.js"]')).toHaveCount(1);
  await page.evaluate(() => {
    document.documentElement.dataset.navigationMarker = 'original';
  });
  await page.getByRole('banner').getByRole('link', { name: 'Test Accounts', exact: true }).click();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole('textbox', { name: 'Email address' })).toBeFocused();
  await expect(page.locator('html')).toHaveAttribute('data-navigation-marker', 'original');
  await page.getByRole('link', { name: 'Create account', exact: true }).click();
  await expect(page).toHaveURL(/\/signup$/);
  await expect(page).toHaveTitle('Create your account · Test Accounts');
  await expect(page.locator('html')).toHaveAttribute('data-navigation-marker', 'original');
  await page.goBack();
  await expect(page).toHaveURL(/\/login$/);
  await expect(page.getByRole('textbox', { name: 'Email address' })).toBeFocused();
  await expect(page.locator('html')).toHaveAttribute('data-navigation-marker', 'original');
  await page.goForward();
  await expect(page).toHaveURL(/\/signup$/);
  await expect(page.getByRole('heading', { name: 'Create your account' })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-navigation-marker', 'original');
  expect(
    await page.evaluate(() => [
      localStorage.getItem('htmx-history-cache'),
      sessionStorage.getItem('htmx-history-cache')
    ])
  ).toEqual([null, null]);
});

test('failed login keeps the document and displays the server error', async ({ page }) => {
  await page.goto('/login');
  await expect(page.locator('form')).toHaveAttribute('hx-boost', 'true');
  await page.evaluate(() => {
    document.documentElement.dataset.navigationMarker = 'original';
  });
  await page.getByRole('textbox', { name: 'Email address' }).fill('missing@example.invalid');
  await page.getByRole('textbox', { name: 'Password', exact: true }).fill('incorrect-password');
  const request = page.waitForRequest(
    (request) => request.method() === 'POST' && request.url().endsWith('/login')
  );
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  expect((await request).isNavigationRequest()).toBe(false);
  await expect(page.getByRole('alert')).toContainText(
    'The email address or password is incorrect.'
  );
  await expect(page.locator('html')).toHaveAttribute('data-navigation-marker', 'original');
  await expect(page.getByRole('textbox', { name: 'Password', exact: true })).toHaveValue('');
});

test('a login network failure shows feedback without retrying credentials', async ({ page }) => {
  await page.goto('/login');
  let submissions = 0;
  await page.route('**/login', async (route) => {
    if (route.request().method() !== 'POST') return route.continue();
    submissions++;
    await route.abort();
  });
  await page.getByRole('textbox', { name: 'Email address' }).fill('missing@example.invalid');
  await page.getByRole('textbox', { name: 'Password', exact: true }).fill('incorrect-password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText("We couldn't reach the server.");
  expect(submissions).toBe(1);
});

test.describe('without JavaScript', () => {
  test.use({ javaScriptEnabled: false });
  test('ordinary links and forms remain available', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeVisible();
    await page.getByRole('link', { name: 'Create account', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Email me a code' })).toBeVisible();
  });
});
