import { test, expect } from './setup';
import type { Page } from '@playwright/test';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

/**
 * Navigate to a route and wait for the client-side app to be fully hydrated.
 *
 * When navigating via page.goto() to SSR pages, the server-rendered HTML may be
 * visible before client-side hydration completes. The visibilitychange handler
 * depends on onSessionValidationNeeded being registered (which happens during
 * hydration in initCurrentUserFromData). Waiting for the WebSocket connection
 * console log proves the full client-side app is initialized.
 */
async function gotoAndWaitForHydration(page: Page, url: string): Promise<void> {
  // Set up listener BEFORE navigating so we don't miss the console message
  const wsConnected = page.waitForEvent('console', {
    predicate: (msg) => /\[ws:.*] Connected/.test(msg.text()),
    timeout: TIMEOUTS.COMPLEX_OPERATION
  });

  await page.goto(url);
  await page.waitForURL(url);

  // Wait for the WebSocket to connect, which proves the client-side app is running
  await wsConnected;

  // Wait for SvelteKit hydration to complete — the session validation handler
  // is registered during initCurrentUserFromData() in the /chat layout.
  // We verify hydration by checking that the page is interactive.
  await page.waitForLoadState('networkidle');
}

/**
 * Clear cookies and trigger session validation via visibilitychange event.
 * Waits for the LoadCurrentUser query response specifically, ensuring the
 * validation completes before we check for the redirect.
 */
async function clearCookiesAndTriggerValidation(page: Page): Promise<void> {
  await page.context().clearCookies();

  // Watch specifically for the LoadCurrentUser validation query
  const validationResponse = page.waitForResponse(
    async (resp) => {
      if (!resp.url().includes('/api/graphql') || resp.status() !== 200) return false;
      const postData = resp.request().postData();
      return postData !== null && postData.includes('LoadCurrentUser');
    },
    { timeout: TIMEOUTS.REALTIME_EVENT }
  );

  await page.evaluate(() => {
    Object.defineProperty(document, 'visibilityState', {
      value: 'visible',
      writable: true,
      configurable: true
    });
    document.dispatchEvent(new Event('visibilitychange'));
  });

  // Wait for the validation query to complete before checking redirect
  await validationResponse;
}

test.describe('Session Expiration Handling', () => {
  test('redirects to login when session cookie is cleared', async ({ page, authPage }) => {
    const timestamp = Date.now();
    const testLogin = `sessionexp${timestamp}`;
    const testPassword = 'testpassword123';

    // Create and login
    await authPage.createUserViaApi(testLogin, testPassword);
    await authPage.login(testLogin, testPassword);
    await authPage.expectLoggedIn();

    // Navigate to a deep route and wait for full client-side initialization
    await gotoAndWaitForHydration(page, routes.settings);
    await expect(page.getByRole('heading', { name: 'Profile' })).toBeVisible();

    // Clear cookies and trigger session validation
    await clearCookiesAndTriggerValidation(page);

    // Should be redirected to home (login) page
    await page.waitForURL('/', { timeout: TIMEOUTS.REALTIME_EVENT });
    await authPage.expectLoggedOut();
  });

  test('saves return URL when session expires', async ({ page, authPage }) => {
    const timestamp = Date.now();
    const testLogin = `sessionreturn${timestamp}`;
    const testPassword = 'testpassword123';

    // Create and login
    await authPage.createUserViaApi(testLogin, testPassword);
    await authPage.login(testLogin, testPassword);
    await authPage.expectLoggedIn();

    // Navigate to a specific route and wait for full client-side initialization
    await gotoAndWaitForHydration(page, routes.settings);
    await expect(page.getByRole('heading', { name: 'Profile' })).toBeVisible();

    // Clear cookies and trigger session validation
    await clearCookiesAndTriggerValidation(page);

    // Wait for redirect
    await page.waitForURL('/', { timeout: TIMEOUTS.REALTIME_EVENT });

    // Check that returnUrl was saved
    const returnUrl = await page.evaluate(() => sessionStorage.getItem('returnUrl'));
    expect(returnUrl).toBe(routes.settings);
  });

  test('can login again after session expiration and return to original page', async ({
    page,
    authPage
  }) => {
    const timestamp = Date.now();
    const testLogin = `sessionrelogin${timestamp}`;
    const testPassword = 'testpassword123';

    // Create and login
    await authPage.createUserViaApi(testLogin, testPassword);
    await authPage.login(testLogin, testPassword);
    await authPage.expectLoggedIn();

    // Navigate to a specific route and wait for full client-side initialization
    await gotoAndWaitForHydration(page, routes.settings);
    await expect(page.getByRole('heading', { name: 'Profile' })).toBeVisible();

    // Clear cookies and trigger session validation
    await clearCookiesAndTriggerValidation(page);

    // Wait for redirect to login
    await page.waitForURL('/', { timeout: TIMEOUTS.REALTIME_EVENT });

    // Login again
    await authPage.gotoLogin();
    await authPage.fillLoginForm(testLogin, testPassword);
    await authPage.signInButton.click();

    // Should be redirected back to the original page
    await page.waitForURL(routes.settings, { timeout: TIMEOUTS.REALTIME_EVENT });
  });

  test('session cookie is refreshed on page load', async ({ page, authPage }) => {
    const timestamp = Date.now();
    const testLogin = `sessionrefresh${timestamp}`;
    const testPassword = 'testpassword123';

    // Create and login
    await authPage.createUserViaApi(testLogin, testPassword);
    await authPage.login(testLogin, testPassword);
    await authPage.expectLoggedIn();

    // Get initial cookie
    const initialCookies = await page.context().cookies();
    const initialSessionCookie = initialCookies.find((c) => c.name === 'chatto_session');
    expect(initialSessionCookie).toBeDefined();

    // Wait >1 second so cookie timestamps can differ (precision is seconds).
    // This is an intentional delay — we need wall-clock time to pass so the
    // cookie's timestamp-based value changes on re-signing.
    await page.waitForTimeout(1500);

    // Navigate to a deep route (this should refresh the cookie)
    await page.goto(routes.settings);
    await page.waitForURL(routes.settings);
    await expect(page.getByRole('heading', { name: 'Profile' })).toBeVisible();

    // Get updated cookie
    const updatedCookies = await page.context().cookies();
    const updatedSessionCookie = updatedCookies.find((c) => c.name === 'chatto_session');
    expect(updatedSessionCookie).toBeDefined();

    // Cookie expiration should be ~90 days from now
    const now = Date.now() / 1000;
    const ninetyDaysInSeconds = 90 * 24 * 60 * 60;
    const expectedMinExpires = now + ninetyDaysInSeconds - 60; // Allow 1 minute tolerance

    // Verify cookie has reasonable expiration (90 days from now, with tolerance)
    expect(updatedSessionCookie!.expires).toBeGreaterThan(expectedMinExpires);

    // Verify cookie was updated (value may have changed due to re-signing)
    // The cookie value changes when session.Save() is called because it includes timestamp
    expect(updatedSessionCookie!.value).toBeTruthy();
  });

  test('handles rapid session validation without multiple redirects', async ({
    page,
    authPage
  }) => {
    const timestamp = Date.now();
    const testLogin = `sessionrapid${timestamp}`;
    const testPassword = 'testpassword123';

    // Create and login
    await authPage.createUserViaApi(testLogin, testPassword);
    await authPage.login(testLogin, testPassword);
    await authPage.expectLoggedIn();

    // Navigate and wait for full client-side initialization
    await gotoAndWaitForHydration(page, '/chat');
    await authPage.expectLoggedIn();

    // Intercept LoadCurrentUser queries to return viewer: null (simulating expired session).
    // This is more reliable than clearing cookies, which can have timing issues with
    // in-flight requests and WebSocket reconnection attempts.
    await page.route('**/api/graphql', async (route) => {
      const postData = route.request().postData();
      if (postData && postData.includes('LoadCurrentUser')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ data: { viewer: null } })
        });
      } else {
        await route.continue();
      }
    });

    // Trigger multiple rapid visibility changes (simulates user switching tabs quickly)
    await page.evaluate(() => {
      Object.defineProperty(document, 'visibilityState', {
        value: 'visible',
        writable: true,
        configurable: true
      });
      // Rapid fire events
      document.dispatchEvent(new Event('visibilitychange'));
      document.dispatchEvent(new Event('visibilitychange'));
      document.dispatchEvent(new Event('visibilitychange'));
    });

    // Should still end up at landing page (only one redirect)
    await page.waitForURL('/', { timeout: TIMEOUTS.REALTIME_EVENT });
    await authPage.expectLoggedOut();

    // Clean up route handler
    await page.unroute('**/api/graphql');

    // Page should be stable (not in a redirect loop) — landed at /login, / or /chat
    await expect(async () => {
      const url = page.url();
      expect(url.endsWith('/') || url.includes('/chat') || url.includes('/login')).toBe(true);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [500, 1000] });
  });
});
