import { expect, test } from './setup';
import { completeSignup } from './fixtures/signup';

test('keeps same-document navigation and history free of page transitions', async ({ page }) => {
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
  await expect(page.locator('html')).toHaveAttribute('data-transitions', '0');
  await page.goBack();
  await expect(page.getByRole('textbox', { name: 'Password', exact: true })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('data-transitions', '0');
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.getByRole('link', { name: 'Create account', exact: true }).click();
  await expect(page).toHaveURL(/\/signup$/);
  await expect(page.locator('html')).toHaveAttribute('data-transitions', '0');
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

test('settling a login response cannot steal focus from the password field', async ({ page }) => {
  await page.goto('/login');
  await page.evaluate(() => {
    // Focus immediately after the swap returns, before a delayed settle task.
    document.addEventListener(
      'htmx:afterSwap',
      () => {
        queueMicrotask(() => document.querySelector<HTMLInputElement>('#password')?.focus());
      },
      { once: true }
    );
    document.addEventListener(
      'htmx:afterSettle',
      () => {
        setTimeout(() => {
          document.documentElement.dataset.settledFocus = document.activeElement?.id;
        }, 0);
      },
      { once: true }
    );
  });
  await page.getByLabel('Email address').fill('missing@example.invalid');
  await page.getByLabel('Password', { exact: true }).fill('incorrect-password');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.locator('html')).toHaveAttribute('data-settled-focus', 'password');
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
  await expect(page.getByRole('alert')).toContainText(
    "We couldn't confirm whether your submission reached the server."
  );
  expect(submissions).toBe(1);
});

test('profile validation and saving keep the document and expose pending state', async ({
  page,
  request,
  stack
}) => {
  await completeSignup(
    page,
    request,
    stack,
    'profile-navigation@example.invalid',
    'correct horse battery staple'
  );
  await page.getByRole('link', { name: 'Edit profile', exact: true }).click();
  await page.evaluate(() => {
    document.documentElement.dataset.navigationMarker = 'original';
  });
  const username = page.getByRole('textbox', { name: 'Preferred username', exact: true });
  await username.fill('  ');
  await page.getByRole('button', { name: 'Save profile' }).click();
  await expect(page.getByRole('alert')).toContainText('Enter a preferred username');
  await expect(username).toHaveValue('  ');
  await expect(page.locator('html')).toHaveAttribute('data-navigation-marker', 'original');

  let releaseRequest!: () => void;
  const pending = new Promise<void>((resolve) => {
    releaseRequest = resolve;
  });
  await page.route('**/account/profile', async (route) => {
    if (route.request().method() === 'POST') await pending;
    await route.continue();
  });
  await username.fill('new-profile-name');
  await page.getByRole('button', { name: 'Save profile' }).click();
  try {
    await expect(page.getByRole('button', { name: 'Save profile' })).toBeDisabled();
    await expect(page.locator('form[action="/account/profile"]')).toHaveAttribute(
      'aria-busy',
      'true'
    );
  } finally {
    releaseRequest();
  }
  await expect(page).toHaveURL(/\/account$/);
  await expect(page.getByRole('status')).toContainText('Your profile was updated.');
  await expect(page.locator('html')).toHaveAttribute('data-navigation-marker', 'original');
  await page.getByRole('link', { name: 'Edit profile', exact: true }).click();
  await expect(username).toHaveValue('new-profile-name');
});

test('profile network failure preserves input and does not retry the submission', async ({
  page,
  request,
  stack
}) => {
  await completeSignup(
    page,
    request,
    stack,
    'profile-network@example.invalid',
    'correct horse battery staple'
  );
  await page.getByRole('link', { name: 'Edit profile', exact: true }).click();
  let submissions = 0;
  await page.route('**/account/profile', async (route) => {
    if (route.request().method() !== 'POST') return route.continue();
    submissions++;
    await route.abort();
  });
  await page.getByRole('textbox', { name: 'Full name', exact: true }).fill('Unsaved example');
  await page.getByRole('button', { name: 'Save profile' }).click();
  await expect(page.getByRole('alert')).toContainText(
    "We couldn't confirm whether your submission reached the server."
  );
  await expect(page.getByRole('textbox', { name: 'Full name', exact: true })).toHaveValue(
    'Unsaved example'
  );
  await expect(page.getByRole('button', { name: 'Save profile' })).toBeEnabled();
  await expect(page.locator('form[action="/account/profile"]')).not.toHaveAttribute('aria-busy');
  expect(submissions).toBe(1);
});

test.describe('without JavaScript', () => {
  test.use({ javaScriptEnabled: false });
  test('profile editing works with native form submission', async ({ page, request, stack }) => {
    await completeSignup(
      page,
      request,
      stack,
      'profile-native@example.invalid',
      'correct horse battery staple'
    );
    await page.getByRole('link', { name: 'Edit profile', exact: true }).click();
    await page.getByRole('textbox', { name: 'Full name', exact: true }).fill('Native example');
    await page.getByRole('button', { name: 'Save profile' }).click();
    await expect(page.getByRole('status')).toContainText('Your profile was updated.');
    await page.getByRole('link', { name: 'Edit profile', exact: true }).click();
    await expect(page.getByRole('textbox', { name: 'Full name', exact: true })).toHaveValue(
      'Native example'
    );
  });
  test('ordinary links and forms remain available', async ({ page }) => {
    await page.goto('/');
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeVisible();
    await page.getByRole('link', { name: 'Create account', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Email me a code' })).toBeVisible();
  });
});
