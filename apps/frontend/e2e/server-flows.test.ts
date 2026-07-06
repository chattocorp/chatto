import type { Browser, BrowserContext, BrowserContextOptions, Page } from '@playwright/test';
import { test, expect } from './setup';
import { createAndLoginTestUser } from './fixtures/testUser';
import { withServerUser } from './fixtures/serverUser';
import { startSecondServer, stopSecondServer, createUserOnRemote } from './fixtures/multiServer';
import { connectPost } from './fixtures/connectHelpers';
import type { ServerInfo } from './fixtures/server';
import { DMPage } from './pages/DMPage';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

interface FreshPageSession {
  context: BrowserContext;
  page: Page;
}

interface ViewerResponse {
  user?: { profile?: { user?: { id?: string } } };
}

async function withFreshPage<T>(
  browser: Browser,
  run: (session: FreshPageSession) => Promise<T>,
  contextOptions: BrowserContextOptions = {}
): Promise<T> {
  const context = await browser.newContext(contextOptions);
  const page = await context.newPage();

  try {
    return await run({ context, page });
  } finally {
    await context.close();
  }
}

test.describe('Landing Page', () => {
  test('unauthenticated user is redirected to /login', async ({ browser }) => {
    await withFreshPage(browser, async ({ page }) => {
      await page.goto('/');
      await page.waitForURL(routes.login);
    });
  });

  test('unauthenticated user does not see sidebar nav icons', async ({ browser }) => {
    await withFreshPage(browser, async ({ page }) => {
      await page.goto(routes.login);

      // Sidebar nav icons for DMs, Browse Spaces, and Create Space should not be present
      await expect(page.getByTestId('dm-icon')).not.toBeVisible();
      await expect(page.getByRole('link', { name: 'Explore Spaces' })).not.toBeVisible();
      await expect(page.getByRole('link', { name: 'Create Space' })).not.toBeVisible();
    });
  });

  test('fresh browser context accepts a valid session cookie without a CSRF cookie', async ({
    browser,
    page,
    serverURL
  }) => {
    await createAndLoginTestUser(page);
    const sessionCookie = (await page.context().cookies()).find(
      (cookie) => cookie.name === 'chatto_session'
    );
    expect(sessionCookie).toBeDefined();

    await withFreshPage(
      browser,
      async ({ context, page: freshPage }) => {
        await context.addCookies([sessionCookie!]);

        const rejectedResponse = await freshPage.request.post('/auth/logout');
        expect(rejectedResponse.status()).toBe(403);

        const viewer = await connectPost<ViewerResponse>(
          freshPage,
          'chatto.api.v1.ViewerService/GetViewer'
        );
        expect(viewer.user?.profile?.id).toBeTruthy();

        await freshPage.goto(routes.settings);
        await expect(freshPage.getByRole('heading', { name: 'Profile' })).toBeVisible();
        await expect(freshPage).not.toHaveURL(routes.login);
      },
      { baseURL: serverURL }
    );
  });

  test('authenticated user with instances is redirected into the server', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto('/');

    // After the auto-join retirement, signed-in users land on the
    // server's Overview page (`/chat/-`); only `lastRoom` storage
    // promotes them into a specific room. Accept either.
    await page.waitForURL(routes.patterns.chatRootOrRoom);
  });
});

test.describe('Last-Room Memory', () => {
  test('navigating to /chat/- redirects to the last visited room', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create and enter a room. `createRoom` waits for the room header to
    // render, which means Room.svelte has mounted and the `setLastRoom`
    // effect has fired.
    const roomName = await chatPage.createRoom();
    const roomUrl = page.url();

    // Navigate to the server root — should redirect back to the room.
    await page.goto(routes.chat);
    await expect(page).toHaveURL(roomUrl);
    await expect(chatPage.getRoomHeader(roomName)).toBeVisible();
  });

  test('navigating to /chat/- redirects to the last visited DM', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    await withServerUser(browser, serverURL, async ({ page: page2 }) => {
      const room = await new DMPage(page2).startConversation(userA.login);
      await room.sendMessage('seed for remembered DM');
      const dmUrl = page2.url();

      await page2.goto(routes.chat);
      await expect(page2).toHaveURL(dmUrl);
      await expect(page2.getByTestId('message-input')).toBeVisible();
    });
  });

  test('navigating to / redirects to the home server last visited DM', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    await withServerUser(browser, serverURL, async ({ page: page2 }) => {
      const room = await new DMPage(page2).startConversation(userA.login);
      await room.sendMessage('seed for root remembered DM');
      const dmUrl = page2.url();

      await page2.goto(routes.root);
      await expect(page2).toHaveURL(dmUrl);
      await expect(page2.getByTestId('message-input')).toBeVisible();
    });
  });

  test('/chat/- falls through to Overview when no last room is stored', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Fresh login, no room visited yet — `/chat/-` should land on Overview.
    await page.goto(routes.chat);
    await page.waitForURL(/\/chat\/-\/overview$/);
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible();
  });

  test('Overview page is reachable directly even when a last DM is stored', async ({
    page,
    browser,
    serverURL
  }) => {
    const userA = await createAndLoginTestUser(page);

    await withServerUser(browser, serverURL, async ({ page: page2 }) => {
      const room = await new DMPage(page2).startConversation(userA.login);
      await room.sendMessage('seed for overview reachability');

      // Navigating directly to the Overview URL should stay on Overview,
      // not bounce to the last room.
      await page2.goto(routes.browseRooms);
      await expect(page2).toHaveURL(/\/chat\/-\/overview$/);
      await expect(page2.getByRole('heading', { name: 'Overview' })).toBeVisible();
    });
  });
});

test.describe('Origin Auto-Registration', () => {
  test('origin instance is registered in localStorage after probe', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    await expect(async () => {
      const stored = await page.evaluate(() =>
        JSON.parse(localStorage.getItem('chatto:instances') ?? '[]')
      );
      expect(stored.length).toBeGreaterThanOrEqual(1);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('origin re-registers after reload if user is authenticated', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Clear localStorage to remove all instances
    await page.evaluate(() => localStorage.removeItem('chatto:instances'));
    await page.reload();
    await page.waitForLoadState('networkidle');

    // Origin should be re-registered via probeOrigin — give it time
    await expect(async () => {
      const stored = await page.evaluate(() =>
        JSON.parse(localStorage.getItem('chatto:instances') ?? '[]')
      );
      expect(stored.length).toBeGreaterThanOrEqual(1);
    }).toPass({ timeout: TIMEOUTS.UI_STANDARD });
  });
});

test.describe('Add Server Dialog', () => {
  test('shows URL input for connecting to remote servers', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await page.getByTitle('Add Server').click();

    // URL input should be visible
    await expect(page.getByLabel('Server URL')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Connect' })).toBeVisible();
  });

  test('shows error for invalid server URL', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();
    await page.getByTitle('Add Server').click();

    // Enter an unreachable URL
    await page.getByLabel('Server URL').fill('https://nonexistent.invalid');
    await page.getByRole('button', { name: 'Connect' }).click();

    // Should show a connection error
    await expect(page.getByText('Could not connect')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  });
});

test.describe('Add Server - Remote Auth Flow', () => {
  let remoteServer: ServerInfo;

  test.beforeEach(async ({}, testInfo) => {
    remoteServer = await startSecondServer(testInfo);
  });

  test.afterEach(async ({}, testInfo) => {
    if (remoteServer) {
      await stopSecondServer(remoteServer, testInfo);
    }
  });

  function remoteBaseURL(server: ServerInfo): string {
    return server.baseURL.replace('localhost', '127.0.0.1');
  }

  /**
   * Drive the dialog up to (but not through) the OAuth redirect: open it
   * from the sidebar `+` button, fill the URL, click Connect to probe, and
   * click the static "Sign in" button on the preview.
   */
  async function driveAddServerToOAuth(page: Page, hostname: string): Promise<void> {
    await page.getByTitle('Add Server').click();
    await page.getByLabel('Server URL').fill(hostname);
    await page.getByRole('button', { name: 'Connect' }).click();
    await expect(page.getByRole('button', { name: 'Sign in', exact: true })).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  }

  test('previewing a valid remote server then continuing redirects to remote OAuth login', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const baseURL = remoteBaseURL(remoteServer);
    const hostname = new URL(baseURL).host;

    await driveAddServerToOAuth(page, hostname);

    // Should redirect to the remote's OAuth login page
    await expect(page).toHaveURL(/\/login\?redirect=/, {
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await expect(page.locator('input[autocomplete="username"]')).toBeVisible();
  });

  test('signing in to remote server via OAuth flow adds it to sidebar', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const baseURL = remoteBaseURL(remoteServer);
    const hostname = new URL(baseURL).host;
    const remoteHostname = new URL(baseURL).hostname;
    await createUserOnRemote(baseURL, 'remoteuser', 'password123');

    await driveAddServerToOAuth(page, hostname);
    await expect(page).toHaveURL(/\/login\?redirect=/, {
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    // Fill in credentials on the remote's login page
    await page.locator('input[autocomplete="username"]').fill('remoteuser');
    await page.locator('input[autocomplete="current-password"]').fill('password123');
    await page.getByRole('button', { name: 'Sign In' }).click();
    await expect(page).toHaveURL(/\/oauth\/consent/, {
      timeout: TIMEOUTS.REALTIME_EVENT
    });
    await page.getByRole('button', { name: 'Allow Access' }).click();

    // Post-PR(a) the OAuth callback drops the user directly into the
    // newly-added remote instance's chat tree (`/chat/<hostname>/...`).
    const remoteHostnameEsc = remoteHostname.replace(/\./g, '\\.');
    await page.waitForURL(new RegExp(`/chat/${remoteHostnameEsc}(/|$)`), {
      timeout: TIMEOUTS.COMPLEX_OPERATION
    });

    // The remote instance should now appear in the sidebar.
    await expect(
      page.locator(`[data-testid="server-icon"][href*="${remoteHostname}"]`).first()
    ).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
  });

  test('invalid credentials show error on remote OAuth login page', async ({ page, chatPage }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    const baseURL = remoteBaseURL(remoteServer);
    const hostname = new URL(baseURL).host;

    await driveAddServerToOAuth(page, hostname);
    await expect(page).toHaveURL(/\/login\?redirect=/, {
      timeout: TIMEOUTS.REALTIME_EVENT
    });

    await page.locator('input[autocomplete="username"]').fill('wronguser');
    await page.locator('input[autocomplete="current-password"]').fill('wrongpassword');
    await page.getByRole('button', { name: 'Sign In' }).click();

    // Should show an auth error on the remote's login page
    await expect(page.getByText(/failed|invalid|not found/i)).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Should stay on the remote's OAuth login page
    await expect(page).toHaveURL(/\/login\?redirect=/);
  });
});

test.describe('Sign Out', () => {
  test('sign out removes all instances and redirects to landing page', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Retry the click idempotently: the button can be visually hydrated
    // before Svelte attaches its onclick handler, so the first click
    // can be dropped. We only re-click if the dialog isn't open yet,
    // to avoid clicking through an already-open dialog.
    const dialog = page.getByRole('dialog');
    await expect(async () => {
      if (!(await dialog.isVisible())) {
        await page.getByTitle('Sign Out').click();
      }
      await expect(dialog).toBeVisible({ timeout: 1000 });
    }).toPass({ timeout: TIMEOUTS.REALTIME_EVENT, intervals: [200, 500, 1000] });

    // Confirm sign out
    await dialog.getByRole('button', { name: 'All Servers' }).click();

    // Should end up at the login page (hard reload clears state)
    await page.waitForURL('/login');

    // No instance should have an authenticated user
    const instances = await page.evaluate(() => localStorage.getItem('chatto:instances'));
    const parsed: { userId: string | null }[] = instances ? JSON.parse(instances) : [];
    expect(parsed.every((i) => !i.userId)).toBe(true);
  });
});

test.describe('/chat backward compatibility', () => {
  test('/chat redirects to / for unauthenticated users', async ({ browser }) => {
    await withFreshPage(browser, async ({ page }) => {
      const navigatedPaths: string[] = [];
      page.on('framenavigated', (frame) => {
        if (frame === page.mainFrame()) navigatedPaths.push(new URL(frame.url()).pathname);
      });

      await page.goto('/chat');
      await page.waitForURL((url) => url.pathname === '/' || url.pathname === '/login');

      expect(navigatedPaths).toContain('/');
    });
  });

  test('/chat redirects authenticated users to /', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto('/chat');

    // / then redirects to /chat/spaces for authenticated users
    await page.waitForURL((url) => url.pathname === '/' || url.pathname.startsWith('/chat/'));
  });
});
