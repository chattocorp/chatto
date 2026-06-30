import { test, expect } from './setup';
import { csrfHeaders } from './fixtures/csrf';
import { createAndLoginTestUser } from './fixtures/testUser';
import { connectPost } from './fixtures/connectHelpers';
import * as routes from './routes';

interface E2EViewerResponse {
  user?: {
    hasVerifiedEmail?: boolean;
  };
}

test.describe('Return URL after login', () => {
  test('redirects to original URL after email login', async ({ page, authPage }) => {
    const timestamp = Date.now();
    const testLogin = `returnurl${timestamp}`;
    const testPassword = 'testpassword123';

    // Create a user first
    await authPage.createUserViaApi(testLogin, testPassword);

    // Try to access a protected route directly (unauthenticated)
    await page.goto(routes.settings);

    // The [serverId] layout detects no auth and redirects to /login,
    // saving the returnUrl in sessionStorage
    await page.waitForURL('/login');

    // sessionStorage should have the return URL stored
    const returnUrl = await page.evaluate(() => sessionStorage.getItem('returnUrl'));
    expect(returnUrl).toBe(routes.settings);

    // Now login via the form
    await authPage.gotoLogin();
    await authPage.fillLoginForm(testLogin, testPassword);
    await authPage.signInButton.click();

    // Should be redirected to the original URL, not /chat
    await page.waitForURL(routes.settings);
  });

  test('redirects to original URL after registration', async ({ page, authPage }) => {
    const timestamp = Date.now();
    const testLogin = `returnurlreg${timestamp}`;
    const testEmail = `returnurlreg${timestamp}@example.com`;
    const testPassword = 'testpassword123';

    // Try to access a protected route directly (unauthenticated)
    await page.goto(routes.admin);

    // The [serverId] layout detects no auth and redirects to /login,
    // saving the returnUrl in sessionStorage
    await page.waitForURL('/login');

    // Register via the email → code → account-details flow
    await authPage.register(testLogin, testEmail, testPassword);

    // Should be redirected to the original URL, not /chat
    await page.waitForURL(routes.admin);
  });

  test('redirects to original URL after OAuth login', async ({ page, authPage }) => {
    const timestamp = Date.now();
    const oauthEmail = `returnoauth${timestamp}@google.com`;

    // Try to access a protected route directly (will be redirected)
    await page.goto(routes.settings);

    // Should be redirected to /login (unauthenticated)
    await page.waitForURL('/login');

    // Simulate OAuth callback - this creates a session
    const data = await authPage.simulateOAuthCallback(oauthEmail, 'OAuth Return User');
    expect(data.success).toBe(true);

    // Navigate to / which forwards authenticated users to /chat — ReturnUrlHandler fires
    await page.goto('/');

    // Should be redirected to the original URL
    await page.waitForURL(routes.settings);
  });

  test('sessionStorage is cleared after redirect to prevent loops', async ({ page, authPage }) => {
    const timestamp = Date.now();
    const testLogin = `noloop${timestamp}`;
    const testPassword = 'testpassword123';

    // Create a user first
    await authPage.createUserViaApi(testLogin, testPassword);

    // Try to access a protected route directly (will be redirected)
    await page.goto(routes.settings);

    // Should be redirected to login page
    await page.waitForURL('/login');

    // Login
    await authPage.gotoLogin();
    await authPage.fillLoginForm(testLogin, testPassword);
    await authPage.signInButton.click();

    // Should be redirected to the original URL
    await page.waitForURL(routes.settings);

    // sessionStorage should now be cleared
    const returnUrl = await page.evaluate(() => sessionStorage.getItem('returnUrl'));
    expect(returnUrl).toBeNull();

    // Refreshing should not redirect back to login (no redirect loop)
    await page.reload();
    await page.waitForURL(routes.patterns.chatRedirect);
    // Verify we're still in the authenticated area (not redirected to login)
    expect(page.url()).toContain(routes.chat);
  });

  test('stores full URL including query parameters', async ({ page, authPage }) => {
    const timestamp = Date.now();
    const testLogin = `queryparams${timestamp}`;
    const testPassword = 'testpassword123';

    // Create a user first
    await authPage.createUserViaApi(testLogin, testPassword);

    // Try to access a protected deep route with query params
    await page.goto(routes.settings + '?tab=profile');

    // Should be redirected to /login (unauthenticated)
    await page.waitForURL('/login');

    // sessionStorage should have the return URL with query params
    const returnUrl = await page.evaluate(() => sessionStorage.getItem('returnUrl'));
    expect(returnUrl).toBe(routes.settings + '?tab=profile');

    // Login
    await authPage.gotoLogin();
    await authPage.fillLoginForm(testLogin, testPassword);
    await authPage.signInButton.click();

    // Should be redirected to the original URL with query params
    await page.waitForURL(routes.settings + '?tab=profile');
  });
});

test.describe('Authentication', () => {
  test('unauthenticated users are redirected to /login at /', async ({ page }) => {
    // Access / directly without logging in
    await page.goto('/');

    // Should redirect to the login page
    await page.waitForURL('/login');
    await expect(page.getByRole('heading', { name: 'Sign In' })).toBeVisible();
  });

  test('authenticated users are redirected away from /login', async ({ page }) => {
    await createAndLoginTestUser(page);

    await page.goto('/login');

    await page.waitForURL((url) => url.pathname.startsWith('/chat'));
    await expect(page.getByRole('heading', { name: 'Sign In' })).not.toBeVisible();
  });

  test('same-origin cookie session works when the stored bearer token is absent', async ({
    page,
    authPage
  }) => {
    const timestamp = Date.now();
    const testLogin = `cookiefallback${timestamp}`;
    const testPassword = 'testpassword123';

    await authPage.createUserViaApi(testLogin, testPassword);
    await authPage.login(testLogin, testPassword);
    await page.waitForURL((url) => url.pathname.startsWith('/chat'));
    await expect(page.getByTitle('Sign out')).toBeVisible();

    const strippedOriginTokens = await page.evaluate(() => {
      const instances = JSON.parse(localStorage.getItem('chatto:instances') || '[]');
      let stripped = 0;
      localStorage.setItem(
        'chatto:instances',
        JSON.stringify(
          instances.map((instance: { url?: string; token?: string | null }) => {
            if (instance.url) {
              try {
                if (new URL(instance.url).origin === window.location.origin) {
                  if (instance.token) stripped += 1;
                  return { ...instance, token: null };
                }
              } catch {
                // Ignore malformed test state; the app will handle it on reload.
              }
            }
            return instance;
          })
        )
      );
      return stripped;
    });
    expect(strippedOriginTokens).toBeGreaterThan(0);

    await page.reload();
    await page.waitForURL((url) => url.pathname.startsWith('/chat'));
    await expect(page.getByRole('heading', { name: 'Sign In' })).not.toBeVisible();
    await expect(page.getByTitle('Sign out')).toBeVisible();
  });

  test('authenticated users are redirected away from /register', async ({ page }) => {
    await createAndLoginTestUser(page);

    await page.goto('/register');

    await page.waitForURL((url) => url.pathname.startsWith('/chat'));
    await expect(page.getByRole('heading', { name: 'Create Account' })).not.toBeVisible();
  });

  test('authenticated OAuth login redirects continue to backend target', async ({ page }) => {
    await createAndLoginTestUser(page);

    await page.goto(`/login?redirect=${encodeURIComponent('/oauth/authorize?client_id=e2e')}`);

    await page.waitForURL((url) => url.pathname === '/oauth/authorize');
  });

  test('authenticated root redirect preserves welcome confirmation', async ({ page }) => {
    await createAndLoginTestUser(page);

    await page.goto('/?welcome=true');

    await expect(
      page.getByText('Your email has been verified and your account is ready.')
    ).toBeVisible();
  });

  test.describe('Registration Form (Step 1 — Email)', () => {
    test('shows email form on /register', async ({ authPage }) => {
      await authPage.gotoRegister();
      await expect(authPage.emailInput).toBeVisible();
      await expect(authPage.continueButton).toBeVisible();
    });

    test('shows validation error for invalid email', async ({ authPage }) => {
      await authPage.gotoRegister();

      // Try an invalid email
      await authPage.emailInput.fill('notanemail');

      // Tab away to trigger validation
      await authPage.continueButton.focus();

      // Should show validation error
      await authPage.expectError(/valid email/i);
    });
  });

  test.describe('Registration Form (Step 2 — Complete)', () => {
    test('can register a new account via the two-step flow', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `regtest${timestamp}`;
      const testEmail = `regtest${timestamp}@example.com`;
      const testPassword = 'testpassword123';

      // Register via the POM using the email → code → account-details flow
      await authPage.register(testLogin, testEmail, testPassword);

      // Verify we're logged in
      await authPage.expectLoggedIn();
    });

    test('registration API creates user with verified email', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `apiregtest${timestamp}`;
      const testEmail = `apiregtest${timestamp}@example.com`;
      const testPassword = 'testpassword123';

      // Call registration API via POM
      const data = await authPage.registerViaApi(testLogin, testEmail, testPassword);

      // Verify response structure
      expect(data.success).toBe(true);
      expect(data.user).toBeDefined();
      expect(data.user.id).toBeDefined();
      expect(data.user.login).toBe(testLogin);
    });

    test('shows validation error for invalid username', async ({ authPage }) => {
      const timestamp = Date.now();
      const testEmail = `invaliduser${timestamp}@example.com`;
      const token = await authPage.createRegistrationTokenViaTestEndpoint(testEmail);
      await authPage.gotoRegisterComplete(token);

      // Try an invalid username with special characters
      await authPage.completeUsernameInput.fill('invalid@user!');

      // Tab away to trigger validation
      await authPage.passwordInput.focus();

      // Should show validation error
      await authPage.expectError(/only letters, numbers, dots, dashes, underscores/i);
    });

    test('shows validation error for short password', async ({ authPage }) => {
      const timestamp = Date.now();
      const testEmail = `shortpw${timestamp}@example.com`;
      const token = await authPage.createRegistrationTokenViaTestEndpoint(testEmail);
      await authPage.gotoRegisterComplete(token);

      // Try a short password
      await authPage.passwordInput.fill('short');

      // Tab away to trigger validation
      await authPage.completeUsernameInput.focus();

      // Should show validation error
      await authPage.expectError(/at least 8 characters/i);
    });

    test('shows validation error for password mismatch', async ({ authPage }) => {
      const timestamp = Date.now();
      const testEmail = `mismatch${timestamp}@example.com`;
      const token = await authPage.createRegistrationTokenViaTestEndpoint(testEmail);
      await authPage.gotoRegisterComplete(token);

      await authPage.passwordInput.fill('testpassword123');
      await authPage.completeConfirmPasswordInput.fill('differentpassword');

      // Tab away to trigger validation
      await authPage.completeUsernameInput.focus();

      // Should show validation error
      await authPage.expectError(/passwords do not match/i);
    });

    test('shows error for duplicate username', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `dupuser${timestamp}`;
      const testPassword = 'testpassword123';

      // First, create a user via API
      await authPage.createUserViaApi(testLogin, testPassword);

      // Create a registration completion token and try to use the same username
      const testEmail = `dupuser${timestamp}@example.com`;
      const token = await authPage.createRegistrationTokenViaTestEndpoint(testEmail);
      await authPage.gotoRegisterComplete(token);
      await authPage.fillRegistrationCompleteForm(testLogin, testPassword, testPassword);
      await authPage.createAccountButton.click();

      // Should show error about username being taken
      await authPage.expectError(/already taken/i);
    });

    test('shows invalid token message when token is missing', async ({ page }) => {
      await page.goto('/register/complete');

      // Should show invalid registration code message
      await expect(page.getByText('Invalid registration code')).toBeVisible();
    });

    test('shows error for invalid token on submit', async ({ authPage }) => {
      await authPage.gotoRegisterComplete('invalid-token');
      await authPage.fillRegistrationCompleteForm('someuser', 'testpassword123', 'testpassword123');
      await authPage.createAccountButton.click();

      // Should show error about invalid token
      await authPage.expectError(/invalid|expired|not found/i);
    });
  });

  test.describe('Login Form', () => {
    test('can login via the login form', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `logintest${timestamp}`;
      const testPassword = 'testpassword123';

      // First create a user via API
      await authPage.createUserViaApi(testLogin, testPassword);

      // Login via POM
      await authPage.login(testLogin, testPassword);

      // Verify we're logged in
      await authPage.expectLoggedIn();
    });

    test('can login with email address instead of username', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `emaillogin${timestamp}`;
      const testEmail = `emaillogin${timestamp}@example.com`;
      const testPassword = 'testpassword123';

      // Create a user via API
      const user = await authPage.createUserViaApi(testLogin, testPassword);

      // Add verified email to user
      await authPage.verifyEmailViaTestEndpoint(user.id, testEmail);

      // Login with email instead of username
      await authPage.login(testEmail, testPassword);

      // Verify we're logged in
      await authPage.expectLoggedIn();
    });

    test('shows error for invalid credentials', async ({ authPage }) => {
      await authPage.gotoLogin();

      // Try to login with non-existent user
      await authPage.fillLoginForm('nonexistentuser', 'wrongpassword');
      await authPage.signInButton.click();

      // Should show error
      await authPage.expectError(/invalid credentials/i);
    });

    test('shows error for wrong password', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `wrongpw${timestamp}`;
      const testPassword = 'testpassword123';

      // Create a user first
      await authPage.createUserViaApi(testLogin, testPassword);

      // Try to login with wrong password
      await authPage.gotoLogin();
      await authPage.fillLoginForm(testLogin, 'wrongpassword');
      await authPage.signInButton.click();

      // Should show error
      await authPage.expectError(/invalid credentials/i);
    });
  });

  test.describe('Registration then Login Flow', () => {
    test('can register, logout, and login again', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `fullflow${timestamp}`;
      const testEmail = `fullflow${timestamp}@example.com`;
      const testPassword = 'testpassword123';

      // Step 1: Register via form
      await authPage.register(testLogin, testEmail, testPassword);
      await authPage.expectLoggedIn();

      // Step 2: Logout
      await authPage.logout();

      // Step 3: Login via form
      await authPage.login(testLogin, testPassword);
      await authPage.expectLoggedIn();
    });
  });

  test.describe('Logout Confirmation', () => {
    test('shows confirmation modal when clicking logout', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `logoutconfirm${timestamp}`;
      const testPassword = 'testpassword123';

      // Create and login
      await authPage.createUserViaApi(testLogin, testPassword);
      await authPage.login(testLogin, testPassword);
      await authPage.expectLoggedIn();

      // Click sign out button - should show confirmation modal
      await authPage.logoutButton.click();
      await expect(authPage.logoutDialog).toBeVisible();
      await expect(authPage.logoutDialog.getByText('disconnect every server')).toBeVisible();
      await expect(
        authPage.logoutDialog.getByRole('button', { name: 'Current Server' })
      ).toBeVisible();
      await expect(authPage.confirmLogoutButton).toBeVisible();
    });

    test('can cancel logout and stay logged in', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `logoutcancel${timestamp}`;
      const testPassword = 'testpassword123';

      // Create and login
      await authPage.createUserViaApi(testLogin, testPassword);
      await authPage.login(testLogin, testPassword);
      await authPage.expectLoggedIn();

      // Cancel logout
      await authPage.cancelLogoutViaUI();

      // Should still be logged in
      await authPage.expectLoggedIn();
    });

    test('can confirm logout and be redirected to home', async ({ authPage }) => {
      const timestamp = Date.now();
      const testLogin = `logoutconfirmyes${timestamp}`;
      const testPassword = 'testpassword123';

      // Create and login
      await authPage.createUserViaApi(testLogin, testPassword);
      await authPage.login(testLogin, testPassword);
      await authPage.expectLoggedIn();

      // Confirm logout
      await authPage.logoutViaUI();

      // Should be redirected to home page
      await authPage.expectLoggedOut();
    });
  });

  test.describe('Navigation Links', () => {
    test('registration page has link to login', async ({ authPage }) => {
      await authPage.gotoRegister();

      await expect(authPage.signInLink).toBeVisible();
      await expect(authPage.signInLink).toHaveAttribute('href', routes.login);
    });

    test('login page has link to registration', async ({ authPage }) => {
      await authPage.gotoLogin();

      await expect(authPage.createAccountLink).toBeVisible();
      await expect(authPage.createAccountLink).toHaveAttribute('href', routes.register);
    });
  });

  test.describe('Email Verification', () => {
    test('user registered via email flow has verified email', async ({ page, authPage }) => {
      const timestamp = Date.now();
      const testEmail = `verified${timestamp}@example.com`;

      // Register via the two-step email flow (token proves email ownership).
      // registerViaApi uses page.request which shares cookies, so the user is
      // automatically logged in after registration — no separate login needed.
      const data = await authPage.registerViaApi(
        `verified${timestamp}`,
        testEmail,
        'testpassword123'
      );
      expect(data.success).toBe(true);

      // Navigate to chat to ensure the authenticated context is loaded
      await page.goto('/chat');
      await page.waitForURL(routes.patterns.chatRedirect);

      const viewer = await connectPost<E2EViewerResponse>(
        page,
        'chatto.api.v1.ViewerService/GetViewer'
      );
      expect(viewer.user?.hasVerifiedEmail).toBe(true);
    });
  });

  test.describe('OAuth Flow (simulated)', () => {
    test('new user via OAuth has auto-verified email', async ({ page, authPage }) => {
      const timestamp = Date.now();
      const oauthEmail = `oauth${timestamp}@google.com`;

      // Simulate OAuth callback via POM
      const data = await authPage.simulateOAuthCallback(oauthEmail, 'OAuth User');

      expect(data.success).toBe(true);
      expect(data.isNewUser).toBe(true);
      expect(data.user.id).toBeTruthy();

      const viewer = await connectPost<E2EViewerResponse>(
        page,
        'chatto.api.v1.ViewerService/GetViewer'
      );
      expect(viewer.user?.hasVerifiedEmail).toBe(true);
    });

    test('existing OAuth user logs in by verified email', async ({ page, authPage }) => {
      const timestamp = Date.now();
      const oauthEmail = `existingoauth${timestamp}@google.com`;

      // First OAuth login - creates user
      const firstData = await authPage.simulateOAuthCallback(oauthEmail, 'Existing OAuth User');
      expect(firstData.isNewUser).toBe(true);
      const userId = firstData.user.id;

      // Clear session by logging out
      await page.request.post('/auth/logout', { headers: await csrfHeaders(page) });

      // Second OAuth login - should find existing user by verified email
      const secondData = await authPage.simulateOAuthCallback(oauthEmail, 'Existing OAuth User');
      expect(secondData.isNewUser).toBe(false);
      expect(secondData.user.id).toBe(userId); // Same user ID
    });
  });
});

test('complete user journey: signup -> open server -> post message', async ({
  page,
  chatPage,
  roomPage
}) => {
  // Step 1: Create and login test user
  const testUser = await createAndLoginTestUser(page);

  // Step 2: Navigate to chat and open the primary server
  await chatPage.goto();

  // Step 3: Navigate to #general room (should be auto-created)
  await chatPage.enterRoom('general');

  // Step 4: Post a message to the #general room
  const testMessage = `Hello from e2e test! ${Date.now()}`;
  await roomPage.sendMessage(testMessage);

  // Step 5: Verify the message appears with the user's display name
  await expect(
    page.locator('[role="article"]').getByRole('button', { name: testUser.displayName })
  ).toBeVisible();
});
