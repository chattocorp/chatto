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
 * Clear all stored credentials and reload the protected route.
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

          if (typeof serverRecord.id === 'string') {
            localStorage.setItem(
              `chatto:i:${serverRecord.id}:authentication`,
              JSON.stringify({
                version: 1,
                token: null,
                refreshToken: null,
                accessTokenExpiresAt: null,
                refreshTokenExpiresAt: null,
                oauthClientId: null,
                refreshRequestId: null,
                reauthRequiredAt: null
              })
            );
          }

          return {
            ...serverRecord,
            token: null,
            refreshToken: null,
            accessTokenExpiresAt: null,
            refreshTokenExpiresAt: null,
            oauthClientId: null,
            refreshRequestId: null,
            reauthRequiredAt: null,
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
    await expect(page.getByRole('heading', { name: 'Profile', level: 1 })).toBeVisible();

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
    await expect(page.getByRole('heading', { name: 'Profile', level: 1 })).toBeVisible();

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
    await expect(page.getByRole('heading', { name: 'Profile', level: 1 })).toBeVisible();

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

  test('same-origin login keeps a renewable cookie without persisted bearer credentials', async ({
    page,
    authPage
  }) => {
    const timestamp = Date.now();
    const testLogin = `sessionrefresh${timestamp}`;
    const testPassword = 'testpassword123';

    // Create and login
    await authPage.createUserViaApi(testLogin, testPassword);
    await authPage.login(testLogin, testPassword);
    await authPage.expectLoggedIn();

    // Get initial cookie
    const initialCookies = await page.context().cookies();
    const initialSessionCookie = initialCookies.find((c) => c.name.startsWith('chatto_auth_'));
    expect(initialSessionCookie).toBeDefined();

    // Ordinary navigation validates the cookie without re-signing it or
    // updating the server-side record.
    await page.goto(routes.settings);
    await page.waitForURL(routes.settings);
    await expect(page.getByRole('heading', { name: 'Profile', level: 1 })).toBeVisible();

    // Get updated cookie
    const updatedCookies = await page.context().cookies();
    const updatedSessionCookie = updatedCookies.find((c) => c.name.startsWith('chatto_auth_'));
    expect(updatedSessionCookie).toBeDefined();

    // Cookie expiration should be ~90 days from login.
    const now = Date.now() / 1000;
    const ninetyDaysInSeconds = 90 * 24 * 60 * 60;
    const expectedMinExpires = now + ninetyDaysInSeconds - 60; // Allow 1 minute tolerance

    // Verify the cookie has a reasonable fixed expiration.
    expect(updatedSessionCookie!.expires).toBeGreaterThan(expectedMinExpires);

    // Ordinary navigation does not replace the opaque handle or its expiry.
    expect(Math.abs(updatedSessionCookie!.expires - initialSessionCookie!.expires)).toBeLessThan(2);
    expect(updatedSessionCookie!.value).toBe(initialSessionCookie!.value);

    const originAuthentication = await page.evaluate(() => {
      const registrations = JSON.parse(localStorage.getItem('chatto:instances') ?? '[]') as Array<{
        id?: string;
        url?: string;
      }>;
      const origin = registrations.find(
        (registration) =>
          typeof registration.url === 'string' &&
          new URL(registration.url).origin === window.location.origin
      );
      if (!origin?.id) return null;
      return JSON.parse(
        localStorage.getItem(`chatto:i:${origin.id}:authentication`) ?? 'null'
      ) as Record<string, unknown> | null;
    });
    expect(originAuthentication).toMatchObject({
      token: null,
      refreshToken: null,
      accessTokenExpiresAt: null,
      refreshTokenExpiresAt: null
    });
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

test.describe('Cookie session renewal', () => {
  test.use({
    serverOptions: {
      env: {
        CHATTO_AUTH_TOKEN_TTL: '12s'
      }
    }
  });

  test('automatically renews the opaque cookie in the final quarter of its lifetime', async ({
    page,
    authPage
  }) => {
    const timestamp = Date.now();
    const testLogin = `sessionrenew${timestamp}`;
    const testPassword = 'testpassword123';

    await authPage.createUserViaApi(testLogin, testPassword);
    await authPage.login(testLogin, testPassword);
    await authPage.expectLoggedIn();

    const initialSessionCookie = (await page.context().cookies()).find((cookie) =>
      cookie.name.startsWith('chatto_auth_')
    );
    expect(initialSessionCookie).toBeDefined();

    // The realtime connection asks the frontend to call the explicit renewal
    // endpoint when the session enters its final quarter.
    await expect
      .poll(
        async () =>
          (await page.context().cookies()).find((cookie) => cookie.name.startsWith('chatto_auth_'))
            ?.expires ?? 0,
        { timeout: 11_000, intervals: [250] }
      )
      .toBeGreaterThan(initialSessionCookie!.expires);

    const renewedSessionCookie = (await page.context().cookies()).find((cookie) =>
      cookie.name.startsWith('chatto_auth_')
    );
    expect(renewedSessionCookie).toBeDefined();
    expect(renewedSessionCookie!.value).toBe(initialSessionCookie!.value);
    expect(renewedSessionCookie!.expires).toBeGreaterThan(initialSessionCookie!.expires);
  });
});
