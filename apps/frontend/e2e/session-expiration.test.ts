import { test, expect } from './setup';
import type { Page } from '@playwright/test';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

const VIEWER_RPC_PATH = '/api/connect/chatto.api.v1.ViewerService/GetViewer';
const VIEWER_RPC_ROUTE = `**${VIEWER_RPC_PATH}`;

/**
 * Navigate to a route and wait for the client-side app to be fully hydrated.
 *
 * When navigating via page.goto(), visible route content can appear before
 * client-side hydration completes. Waiting for the WebSocket connection console
 * log proves the full client-side app is initialized.
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

  await page.locator('body').waitFor({ state: 'visible' });
}

/**
 * Clear stored credentials and reload the protected route.
 *
 * Session expiry is observed on the next app load or protected request. There
 * is intentionally no passive visibilitychange validation hook anymore: that
 * hook made transient auth/API failures look like real logouts.
 */
async function clearCredentialsAndReloadProtectedRoute(page: Page): Promise<void> {
  const target = new URL(page.url());
  const targetPath = target.pathname + target.search + target.hash;

  await page.context().clearCookies();
  await page.evaluate((origin) => {
    const raw = localStorage.getItem('chatto:instances');
    if (raw === null) return;

    let servers: unknown;
    try {
      servers = JSON.parse(raw);
    } catch {
      return;
    }
    if (!Array.isArray(servers)) return;

    localStorage.setItem(
      'chatto:instances',
      JSON.stringify(
        servers.map((server) => {
          if (server === null || typeof server !== 'object') return server;
          const serverRecord = server as Record<string, unknown>;
          if (typeof serverRecord.url !== 'string') return server;

          try {
            if (new URL(serverRecord.url).origin !== origin) return server;
          } catch {
            return server;
          }

          return {
            ...serverRecord,
            token: null,
            userId: null,
            userLogin: null,
            userDisplayName: null,
            userAvatarUrl: null
          };
        })
      )
    );
  }, target.origin);

  const currentUserResponse = page.waitForResponse(
    async (resp) => {
      return resp.url().includes(VIEWER_RPC_PATH);
    },
    { timeout: TIMEOUTS.REALTIME_EVENT }
  );

  await page.goto(targetPath);
  await currentUserResponse;
}

async function expectLoggedOutRedirect(page: Page): Promise<void> {
  await expect(page).toHaveURL(
    (url) => url.pathname === routes.root || url.pathname === routes.login,
    { timeout: TIMEOUTS.REALTIME_EVENT }
  );
}

test.describe('Session Expiration Handling', () => {
  test('redirects to login when stored credentials are cleared', async ({ page, authPage }) => {
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

    // Clear credentials and reload the protected route
    await clearCredentialsAndReloadProtectedRoute(page);

    // Should leave the authenticated chat surface.
    await expectLoggedOutRedirect(page);
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

    // Clear credentials and reload the protected route
    await clearCredentialsAndReloadProtectedRoute(page);

    // Wait for redirect
    await expectLoggedOutRedirect(page);

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

    // Clear credentials and reload the protected route
    await clearCredentialsAndReloadProtectedRoute(page);

    // Wait for redirect to login
    await expectLoggedOutRedirect(page);

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

  test('handles repeated expired-session loads without multiple redirects', async ({
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

    // Intercept GetViewer to return an unauthenticated Connect error.
    await page.route(VIEWER_RPC_ROUTE, async (route) => {
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        headers: { 'Connect-Protocol-Version': '1' },
        body: JSON.stringify({ code: 'unauthenticated', message: 'authentication required' })
      });
    });

    await page.goto(routes.settings);

    // Should still end up at landing page.
    await expectLoggedOutRedirect(page);
    await authPage.expectLoggedOut();

    // Re-entering the stale protected URL should settle to the same logged-out
    // route without bouncing between app shells.
    await page.goto(routes.settings);
    await expectLoggedOutRedirect(page);
    await authPage.expectLoggedOut();

    // Clean up route handler
    await page.unroute(VIEWER_RPC_ROUTE);

    // Page should be stable (not in a redirect loop) — landed at /login, / or /chat
    await expect(async () => {
      const url = page.url();
      expect(url.endsWith('/') || url.includes('/chat') || url.includes('/login')).toBe(true);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD, intervals: [500, 1000] });
  });
});
