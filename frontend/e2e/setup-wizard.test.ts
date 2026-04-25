import { test as base, expect } from '@playwright/test';
import { startServer, stopServer, type ServerInfo } from './fixtures/server';
import { ChatPage, AuthPage } from './pages';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

/**
 * Test fixture that uses a fresh (non-bootstrapped) server.
 * This allows testing the setup wizard flow.
 */
const test = base.extend<{
  freshServer: ServerInfo;
  serverURL: string;
  chatPage: ChatPage;
  authPage: AuthPage;
}>({
  freshServer: async ({}, use, testInfo) => {
    // Start server without bootstrapping
    const server = await startServer(testInfo, { skipBootstrap: true });
    await use(server);
    await stopServer(server, testInfo);
  },

  serverURL: async ({ freshServer }, use) => {
    await use(freshServer.baseURL);
  },

  chatPage: async ({ page }, use) => {
    await use(new ChatPage(page));
  },

  authPage: async ({ page }, use) => {
    await use(new AuthPage(page));
  }
});

test.describe('Setup Wizard', () => {
  test('fresh instance redirects root to /setup', async ({ page, serverURL }) => {
    await page.goto(serverURL);

    // Should be redirected to setup page
    await page.waitForURL(`${serverURL}/setup`);

    // Verify setup page content is visible
    await expect(page.getByRole('heading', { name: 'Set Up Chatto' })).toBeVisible();
    await expect(page.getByText("Welcome! Let's get your instance ready.")).toBeVisible();
  });

  test('setup page shows admin account form', async ({ page, serverURL }) => {
    await page.goto(`${serverURL}/setup`);

    // Wait for setup page to load
    await expect(page.getByRole('heading', { name: 'Set Up Chatto' })).toBeVisible();

    // Verify form elements
    await expect(page.getByLabel('Username')).toBeVisible();
    await expect(page.getByLabel('Email')).toBeVisible();
    await expect(page.getByLabel('Password')).toBeVisible();
    await expect(page.getByLabel('Create an initial space')).toBeVisible();

    // Space fields visible by default (checkbox is checked by default)
    await expect(page.getByLabel('Space Name')).toBeVisible();
  });

  test('can complete setup without creating a space', async ({ page, serverURL }) => {
    await page.goto(`${serverURL}/setup`);

    // Wait for page to load
    await expect(page.getByRole('heading', { name: 'Set Up Chatto' })).toBeVisible();

    // Uncheck "Create an initial space"
    await page.getByLabel('Create an initial space').uncheck();

    // Space name should be hidden now
    await expect(page.getByLabel('Space Name')).not.toBeVisible();

    // Fill in admin account
    await page.getByLabel('Username').fill('testadmin');
    await page.getByLabel('Email').fill('testadmin@example.com');
    await page.getByLabel('Password').fill('testpassword123');

    // Submit
    await page.getByRole('button', { name: 'Complete Setup' }).click();

    // Should redirect to chat, then to browse spaces (since no spaces joined)
    await page.waitForURL(`${serverURL}${routes.spaces}`);

    // Verify we're logged in and on the browse spaces page
    await expect(page.getByRole('heading', { name: 'Browse Spaces' })).toBeVisible();
  });

  test('can complete setup with initial space', async ({ page, serverURL }) => {
    await page.goto(`${serverURL}/setup`);

    // Wait for page to load
    await expect(page.getByRole('heading', { name: 'Set Up Chatto' })).toBeVisible();

    // Fill in admin account
    await page.getByLabel('Username').fill('testadmin');
    await page.getByLabel('Email').fill('testadmin@example.com');
    await page.getByLabel('Password').fill('testpassword123');

    // Fill in space details (checkbox is checked by default)
    await page.getByLabel('Space Name').fill('My Test Space');
    await page.getByLabel('Description (optional)').fill('A great place to chat');

    // Submit
    await page.getByRole('button', { name: 'Complete Setup' }).click();

    // Should redirect to chat
    await page.waitForURL((url) => url.pathname === '/' || url.pathname.startsWith('/chat'));

    // Verify the space was created by checking if we can fetch it via API
    const response = await page.request.post(`${serverURL}/api/graphql`, {
      data: {
        query: `query { spaces { id name } }`
      }
    });
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    const spaceNames = body.data.spaces.map((s: { name: string }) => s.name);
    expect(spaceNames).toContain('My Test Space');
  });

  test('created admin has admin role', async ({ page, serverURL }) => {
    await page.goto(`${serverURL}/setup`);

    // Wait for page to load
    await expect(page.getByRole('heading', { name: 'Set Up Chatto' })).toBeVisible();

    // Complete setup without creating a space
    await page.getByLabel('Create an initial space').uncheck();
    await page.getByLabel('Username').fill('testadmin');
    await page.getByLabel('Email').fill('testadmin@example.com');
    await page.getByLabel('Password').fill('testpassword123');
    await page.getByRole('button', { name: 'Complete Setup' }).click();

    // Wait for redirect to chat welcome page (user has no spaces)
    await page.waitForURL((url) => url.pathname === '/' || url.pathname.startsWith('/chat'));

    // Verify user has admin access by checking if Admin Panel link is visible
    // Only admins can see the Admin Panel link in the header
    // Give the page a moment to load the canViewAdmin query
    await expect(page.getByRole('link', { name: 'Admin Panel' })).toBeVisible({ timeout: TIMEOUTS.COMPLEX_OPERATION });
  });

  test('shows validation errors for invalid input', async ({ page, serverURL }) => {
    await page.goto(`${serverURL}/setup`);

    // Wait for page to load
    await expect(page.getByRole('heading', { name: 'Set Up Chatto' })).toBeVisible();

    // Fill in invalid username (special characters)
    await page.getByLabel('Username').fill('test@user!');
    await page.getByLabel('Email').focus(); // Trigger validation

    // Should show validation error
    await expect(page.getByText('Only letters, numbers, dots, dashes, underscores')).toBeVisible();

    // Clear and fill invalid email
    await page.getByLabel('Username').clear();
    await page.getByLabel('Username').fill('testuser');
    await page.getByLabel('Email').fill('notanemail');
    await page.getByLabel('Password').focus(); // Trigger validation

    await expect(page.getByText('Please enter a valid email address')).toBeVisible();

    // Clear and fill short password
    await page.getByLabel('Email').clear();
    await page.getByLabel('Email').fill('test@example.com');
    await page.getByLabel('Password').fill('short');
    await page.getByLabel('Username').focus(); // Trigger validation

    await expect(page.getByText('Must be at least 8 characters')).toBeVisible();
  });

  test('submit button disabled until form is valid', async ({ page, serverURL }) => {
    await page.goto(`${serverURL}/setup`);

    // Wait for page to load
    await expect(page.getByRole('heading', { name: 'Set Up Chatto' })).toBeVisible();

    const submitButton = page.getByRole('button', { name: 'Complete Setup' });

    // Initially disabled (form is empty)
    await expect(submitButton).toBeDisabled();

    // Fill partial form
    await page.getByLabel('Username').fill('testadmin');
    await expect(submitButton).toBeDisabled();

    await page.getByLabel('Email').fill('test@example.com');
    await expect(submitButton).toBeDisabled();

    await page.getByLabel('Password').fill('testpassword123');
    // Space name is required when checkbox is checked
    await expect(submitButton).toBeDisabled();

    await page.getByLabel('Space Name').fill('Test Space');
    // Now should be enabled
    await expect(submitButton).toBeEnabled();
  });
});

/**
 * Test that setup cannot be accessed on a non-fresh instance.
 * This uses the fresh server fixture but bootstraps it manually first.
 */
test.describe('Setup Wizard - Already Bootstrapped', () => {
  test('logged in user accessing /setup redirects to /chat', async ({ page, serverURL }) => {
    // First, complete the bootstrap
    await page.goto(`${serverURL}/setup`);
    await expect(page.getByRole('heading', { name: 'Set Up Chatto' })).toBeVisible();

    // Complete setup without creating a space
    await page.getByLabel('Create an initial space').uncheck();
    await page.getByLabel('Username').fill('testadmin');
    await page.getByLabel('Email').fill('testadmin@example.com');
    await page.getByLabel('Password').fill('testpassword123');
    await page.getByRole('button', { name: 'Complete Setup' }).click();
    // Redirects to /chat since user has no spaces
    await page.waitForURL((url) => url.pathname === '/' || url.pathname.startsWith('/chat'));

    // Now try to access /setup again while logged in - should redirect to /chat
    await page.goto(`${serverURL}/setup`);
    await page.waitForURL((url) => url.pathname === '/' || url.pathname.startsWith('/chat'));
  });

  test('unauthenticated user on non-fresh instance cannot access /setup', async ({
    page,
    serverURL,
    browser
  }) => {
    // First, complete the bootstrap
    await page.goto(`${serverURL}/setup`);
    await expect(page.getByRole('heading', { name: 'Set Up Chatto' })).toBeVisible();

    await page.getByLabel('Create an initial space').uncheck();
    await page.getByLabel('Username').fill('testadmin');
    await page.getByLabel('Email').fill('testadmin@example.com');
    await page.getByLabel('Password').fill('testpassword123');
    await page.getByRole('button', { name: 'Complete Setup' }).click();
    // Redirects to /chat since user has no spaces
    await page.waitForURL((url) => url.pathname === '/' || url.pathname.startsWith('/chat'));

    // Use a fresh browser context to simulate an unauthenticated user
    const newContext = await browser.newContext();
    const newPage = await newContext.newPage();

    // Try to access /setup as unauthenticated user - should redirect away
    await newPage.goto(`${serverURL}/setup`);
    await newPage.waitForURL((url) => url.pathname === '/' || url.pathname.startsWith('/chat'));

    // Verify we're redirected away from /setup
    expect(newPage.url()).not.toContain('/setup');

    await newContext.close();
  });

  test('attempting to bootstrap twice via API fails', async ({ page, serverURL }) => {
    // First bootstrap
    await page.goto(`${serverURL}/setup`);
    await expect(page.getByRole('heading', { name: 'Set Up Chatto' })).toBeVisible();

    await page.getByLabel('Create an initial space').uncheck();
    await page.getByLabel('Username').fill('admin1');
    await page.getByLabel('Email').fill('admin1@example.com');
    await page.getByLabel('Password').fill('password123');
    await page.getByRole('button', { name: 'Complete Setup' }).click();
    // Redirects to /chat since user has no spaces
    await page.waitForURL((url) => url.pathname === '/' || url.pathname.startsWith('/chat'));

    // Try to call the bootstrap API directly
    const response = await page.request.post(`${serverURL}/auth/bootstrap`, {
      data: {
        login: 'admin2',
        email: 'admin2@example.com',
        password: 'password456'
      }
    });

    // Should fail because already bootstrapped (409 Conflict)
    expect(response.status()).toBe(409);
    const body = await response.json();
    expect(body.error).toContain('already been set up');
  });
});
