import { test, expect } from './setup';
import type { Page } from '@playwright/test';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

/**
 * Navigate to a route and wait for the client-side app to be fully hydrated.
 * The WebSocket connection console log proves the full client-side app is initialized.
 */
async function gotoAndWaitForHydration(page: Page, url: string): Promise<void> {
  const wsConnected = page.waitForEvent('console', {
    predicate: (msg) => /\[ws:.*] Connected/.test(msg.text()),
    timeout: TIMEOUTS.COMPLEX_OPERATION
  });

  await page.goto(url);

  // Wait for the WebSocket to connect, which proves the client-side app is running
  await wsConnected;

  await page.locator('body').waitFor({ state: 'visible' });
}

test.describe('Cross-Tab Sign-Out', () => {
  test('server-side: logout in one tab disconnects another tab via SessionTerminatedEvent', async ({
    browser,
    serverURL,
    authPage
  }) => {
    const timestamp = Date.now();
    const testLogin = `crosstab${timestamp}`;
    const testPassword = 'testpassword123';

    // Create user and login in tab 1
    await authPage.createUserViaApi(testLogin, testPassword);
    await authPage.login(testLogin, testPassword);
    await authPage.expectLoggedIn();

    // Get the session cookie from tab 1
    const cookies = await authPage.page.context().cookies();
    const sessionCookie = cookies.find((c) => c.name === 'chatto_session');
    expect(sessionCookie).toBeDefined();

    // Create a second browser context with the same session cookie
    const context2 = await browser!.newContext({
      baseURL: serverURL,
      viewport: { width: 1280, height: 720 }
    });
    await context2.addCookies([sessionCookie!]);
    const page2 = await context2.newPage();

    try {
      // Navigate page2 to chat/spaces and wait for full hydration
      await gotoAndWaitForHydration(page2, routes.spaces);

      // Verify page2 is authenticated and on the chat page
      await expect(page2).toHaveURL(routes.patterns.chatRedirect);

      // Set up a listener for the session terminated console log before triggering logout
      const sessionTerminatedLog = page2.waitForEvent('console', {
        predicate: (msg) => msg.text().includes('Session terminated by server'),
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Log out in tab 1 — this publishes SessionTerminatedEvent
      await authPage.page.evaluate(() => fetch('/auth/logout', { method: 'POST' }));

      // Wait for the session terminated event to be received
      await sessionTerminatedLog;

      // Tab 2 should redirect to /chat (unauthenticated view)
      await page2.waitForURL('/', { timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await context2.close();
    }
  });

  test('BroadcastChannel: logout in one tab notifies another tab in same browser', async ({
    page,
    authPage
  }) => {
    const timestamp = Date.now();
    const testLogin = `bcasttab${timestamp}`;
    const testPassword = 'testpassword123';

    // Create user and login in tab 1
    await authPage.createUserViaApi(testLogin, testPassword);
    await authPage.login(testLogin, testPassword);
    await authPage.expectLoggedIn();

    // Open a second page in the SAME browser context (shared cookies + BroadcastChannel)
    const page2 = await page.context().newPage();

    try {
      // Navigate page2 to chat and wait for full hydration
      await gotoAndWaitForHydration(page2, routes.spaces);
      await expect(page2).toHaveURL(routes.patterns.chatRedirect);

      // Log out in tab 1 via the logout endpoint + BroadcastChannel notification
      await page.evaluate(() => fetch('/auth/logout', { method: 'POST' }));
      await page.evaluate(() => {
        const ch = new BroadcastChannel('chatto-session');
        ch.postMessage({ type: 'logout' });
        ch.close();
      });

      // Tab 2 should receive the BroadcastChannel message and redirect to /chat
      await page2.waitForURL('/', { timeout: TIMEOUTS.REALTIME_EVENT });
    } finally {
      await page2.close();
    }
  });
});
