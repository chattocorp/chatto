import { type Page } from '@playwright/test';
import { test, expect } from './setup';
import {
  createAndLoginTestUser,
  denyUserInstancePermission,
  clearUserInstancePermissionOverride,
  type TestUser
} from './fixtures/testUser';
import * as routes from './routes';
import { TIMEOUTS } from './constants';

// Admin config must match e2e/fixtures/chatto.toml
const ADMIN_EMAIL = 'admin@e2e-test.example.com';
const ADMIN_LOGIN = 'e2eadmin';
const ADMIN_PASSWORD = 'adminpassword123';

interface TestSpace {
  id: string;
  name: string;
  description?: string;
}

/**
 * Creates a space via GraphQL API (requires authenticated user).
 * Returns the space ID for use in join page tests.
 */
async function createSpaceViaAPI(
  page: Page,
  options?: { name?: string; description?: string }
): Promise<TestSpace> {
  const timestamp = Date.now();
  const spaceName = options?.name ?? `Join Test Space ${timestamp}`;
  const spaceDescription = options?.description ?? 'A space for testing the join page';

  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation CreateSpace($input: CreateSpaceInput!) {
					createSpace(input: $input) {
						id
						name
						description
					}
				}
			`,
      variables: {
        input: {
          name: spaceName,
          description: spaceDescription
        }
      }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.createSpace).toBeTruthy();

  return {
    id: data.data.createSpace.id,
    name: data.data.createSpace.name,
    description: data.data.createSpace.description
  };
}

/**
 * Creates a second test user (different from the space creator).
 * The user has a verified email so they can join/create spaces.
 */
async function createSecondTestUser(page: Page): Promise<TestUser> {
  const timestamp = Date.now();
  const testUser: TestUser = {
    login: `seconduser${timestamp}`,
    displayName: `Second User ${timestamp}`,
    password: 'testpassword123'
  };

  const createUserResponse = await page.request.post('/auth/test/create-user', {
    headers: { 'Content-Type': 'application/json' },
    data: {
      login: testUser.login,
      displayName: testUser.displayName,
      password: testUser.password
    }
  });

  expect(createUserResponse.ok()).toBeTruthy();
  const createUserData = await createUserResponse.json();
  testUser.id = createUserData.id;

  // Verify email so user has space.join permission
  const verifyResponse = await page.request.post('/auth/test/verify-email', {
    headers: { 'Content-Type': 'application/json' },
    data: {
      userId: testUser.id,
      email: `${testUser.login}@example.com`
    }
  });
  expect(verifyResponse.ok()).toBeTruthy();

  return testUser;
}

/**
 * Logs in an existing user via HTTP endpoint.
 */
async function loginUser(page: Page, login: string, password: string): Promise<void> {
  const loginResponse = await page.request.post('/auth/login', {
    data: { login, password }
  });

  expect(loginResponse.ok()).toBeTruthy();
  const loginData = await loginResponse.json();
  expect(loginData.success).toBe(true);
}

/**
 * Logs out the current user.
 */
async function logoutUser(page: Page): Promise<void> {
  await page.request.post('/auth/logout');
}

test.describe('Space Join Page', () => {
  test('logged-out user sees space preview with sign-in options', async ({ page }) => {
    // Create a user and space first
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page, {
      name: 'Public Test Space',
      description: 'Welcome to our community!'
    });

    // Log out to test as unauthenticated user
    await logoutUser(page);

    // Visit join page
    await page.goto(routes.joinSpace(space.id));

    // Should see space name and description
    await expect(page.getByRole('heading', { name: space.name })).toBeVisible();
    await expect(page.getByText(space.description!)).toBeVisible();

    // Should see member count (creator is 1 member)
    await expect(page.getByText(/1 member/)).toBeVisible();

    // Should see sign-in prompt
    await expect(page.getByText('Sign in to join this space')).toBeVisible();

    // Should see sign-in options
    await expect(page.getByRole('link', { name: 'Sign in with Email' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Create Account' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Continue with Google' })).toBeVisible();

    // Should NOT see Join Space button
    await expect(page.getByRole('button', { name: 'Join Space' })).not.toBeVisible();
  });

  test('logged-in non-member sees Join Space button and can join', async ({ page }) => {
    // Create first user (space creator)
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page, { name: 'Joinable Space' });

    // Create second user (will join the space)
    const joiner = await createSecondTestUser(page);

    // Log out creator and log in as joiner
    await logoutUser(page);
    await loginUser(page, joiner.login, joiner.password);

    // Visit join page
    await page.goto(routes.joinSpace(space.id));

    // Should see space name
    await expect(page.getByRole('heading', { name: space.name })).toBeVisible();

    // Should NOT see sign-in prompt (user is logged in)
    await expect(page.getByText('Sign in to join this space')).not.toBeVisible();

    // Should see Join Space button
    const joinButton = page.getByRole('button', { name: 'Join Space' });
    await expect(joinButton).toBeVisible();

    // Click join button
    await joinButton.click();

    // Should redirect to the space chat
    await page.waitForURL(new RegExp(routes.space()));

    // Should be in the space now (check header, not announcer)
    await expect(page.getByRole('heading', { name: space.name })).toBeVisible();
  });

  test('logged-in member is redirected to space', async ({ page }) => {
    // Create user and space (creator is automatically a member)
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page);

    // Visit join page as member
    await page.goto(routes.joinSpace(space.id));

    // Should be redirected to the space chat
    await page.waitForURL(new RegExp(routes.space()));
  });

  test('non-existent space shows error', async ({ page }) => {
    // Visit join page for non-existent space
    await page.goto(routes.joinSpace('nonexistent123456'));

    // Should show error message
    await expect(page.getByText('Space not found')).toBeVisible();

    // Should show link to home
    await expect(page.getByRole('link', { name: 'Go to Home' })).toBeVisible();
  });

  test('sign-in links include redirect back to join page', async ({ page }) => {
    // Create a user and space first
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page);

    // Log out
    await logoutUser(page);

    // Visit join page
    await page.goto(routes.joinSpace(space.id));

    // Check that sign-in link has correct redirect
    const signInLink = page.getByRole('link', { name: 'Sign in with Email' });
    await expect(signInLink).toHaveAttribute('href', `${routes.login}?redirect=${routes.joinSpace(space.id)}`);

    // Check that register link has correct join param
    const registerLink = page.getByRole('link', { name: 'Create Account' });
    await expect(registerLink).toHaveAttribute('href', `${routes.register}?join=${space.id}`);

    // Check that Google auth link has correct redirect
    const googleLink = page.getByRole('link', { name: 'Continue with Google' });
    await expect(googleLink).toHaveAttribute('href', `/auth/google?redirect=${routes.joinSpace(space.id)}`);
  });
});

/**
 * Creates an admin user with verified email matching the owners.emails config.
 */
async function createAndLoginAdminUser(page: Page): Promise<TestUser> {
  const adminUser: TestUser = {
    login: ADMIN_LOGIN,
    displayName: 'Admin User',
    password: ADMIN_PASSWORD
  };

  // Try to create user via the test-only endpoint. May fail if the user
  // already exists from a previous run; the login flow below handles that.
  const createUserResponse = await page.request.post('/auth/test/create-user', {
    headers: { 'Content-Type': 'application/json' },
    data: {
      login: adminUser.login,
      displayName: adminUser.displayName,
      password: adminUser.password
    }
  });

  if (createUserResponse.ok()) {
    const createUserData = await createUserResponse.json();
    if (createUserData?.id) {
      adminUser.id = createUserData.id;
    }
  }

  // Login
  const loginResponse = await page.request.post('/auth/login', {
    data: { login: adminUser.login, password: adminUser.password }
  });
  expect(loginResponse.ok()).toBeTruthy();

  // If we don't have the user ID yet (existing user), fetch it via GraphQL
  if (!adminUser.id) {
    const meResponse = await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: { query: `query { me { id } }` }
    });
    const meData = await meResponse.json();
    adminUser.id = meData.data?.me?.id;
  }

  // Verify the admin email to grant admin access (idempotent - safe if already verified)
  if (adminUser.id) {
    await page.request.post('/auth/test/verify-email', {
      headers: { 'Content-Type': 'application/json' },
      data: { userId: adminUser.id, email: ADMIN_EMAIL }
    });
  }

  return adminUser;
}

test.describe('Join Space Permission', () => {
  test('user with denied space.join cannot join a space', async ({ page, browser, serverURL }) => {
    // Create admin and a space
    await createAndLoginAdminUser(page);
    const space = await createSpaceViaAPI(page, { name: 'Restricted Join Space' });

    // Create a regular user in a separate context
    const regularContext = await browser.newContext({ baseURL: serverURL });
    const regularPage = await regularContext.newPage();
    const regularUser = await createAndLoginTestUser(regularPage);

    // As admin, deny space.join for the regular user
    const denyRoleName = await denyUserInstancePermission(page, regularUser.id!, 'space.join');

    // As regular user, try to join the space via the join page
    await regularPage.goto(routes.joinSpace(space.id));

    // Should see the space info and join button
    await expect(regularPage.getByRole('heading', { name: space.name })).toBeVisible();
    const joinButton = regularPage.getByRole('button', { name: 'Join Space' });
    await expect(joinButton).toBeVisible();

    // Click join - should fail
    await joinButton.click();

    // Should see an error message (the UI shows a generic error)
    await expect(regularPage.getByText('Failed to join space')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Should NOT be redirected to the space
    await expect(regularPage).not.toHaveURL(new RegExp(routes.space()));

    // Clean up
    await clearUserInstancePermissionOverride(page, regularUser.id!, 'space.join', denyRoleName);
    await regularContext.close();
  });

  test('user with space.join permission can join a space', async ({ page, browser, serverURL }) => {
    // Create admin and a space
    await createAndLoginAdminUser(page);
    const space = await createSpaceViaAPI(page, { name: 'Open Join Space' });

    // Create a regular user in a separate context
    const regularContext = await browser.newContext({ baseURL: serverURL });
    const regularPage = await regularContext.newPage();
    await createAndLoginTestUser(regularPage);

    // Regular user has space.join by default (member role)
    await regularPage.goto(routes.joinSpace(space.id));

    // Click join button
    const joinButton = regularPage.getByRole('button', { name: 'Join Space' });
    await expect(joinButton).toBeVisible();
    await joinButton.click();

    // Should be redirected to the space
    await regularPage.waitForURL(new RegExp(routes.space()));
    await expect(regularPage.getByRole('heading', { name: space.name })).toBeVisible();

    await regularContext.close();
  });
});
