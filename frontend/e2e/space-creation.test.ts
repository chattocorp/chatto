import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  revokeInstancePermission,
  grantInstancePermission,
  type TestUser
} from './fixtures/testUser';
import { ChatPage } from './pages';
import * as routes from './routes';

// Admin config must match e2e/fixtures/chatto.toml
const ADMIN_EMAIL = 'admin@e2e-test.example.com';
const ADMIN_LOGIN = 'e2eadmin';
const ADMIN_PASSWORD = 'adminpassword123';

let sharedAdminUser: TestUser | null = null;

async function createAndLoginAdminUser(page: Page): Promise<TestUser> {
  if (sharedAdminUser) {
    const loginResponse = await page.request.post('/auth/login', {
      data: {
        login: sharedAdminUser.login,
        password: sharedAdminUser.password
      }
    });
    expect(loginResponse.ok()).toBeTruthy();
    return sharedAdminUser;
  }

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

  // Always login after creating (or if user already exists)
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

  sharedAdminUser = adminUser;
  return adminUser;
}

test.describe('Space Creation Permission', () => {
  // Ensure everyone role's space.create permission is restored after each test
  test.afterEach(async ({ page }) => {
    try {
      await page.request.post('/api/graphql', {
        headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
        data: {
          query: `mutation { grantInstancePermission(input: {role: "everyone", permission: "space.create"}) }`
        }
      });
    } catch {
      // Ignore errors - may not be logged in as admin
    }
  });

  test('create space button is visible by default (members have space.create)', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // The create space button should be visible in the sidebar
    await chatPage.expectCreateSpaceVisible();
  });

  test('create space button is hidden when space.create permission is revoked', async ({
    page,
    browser
  }) => {
    // First, as admin, revoke space.create from everyone role
    await createAndLoginAdminUser(page);
    await revokeInstancePermission(page, 'everyone', 'space.create');

    // Create a user in a separate context
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    await createAndLoginTestUser(regularPage);

    const chatPage = new ChatPage(regularPage);
    await chatPage.goto();

    // The create space button should NOT be visible
    await chatPage.expectCreateSpaceNotVisible();

    // But the explore spaces button should still be visible
    await chatPage.expectExploreSpacesVisible();

    // Clean up: restore the permission
    await grantInstancePermission(page, 'everyone', 'space.create');
    await regularContext.close();
  });

  test('create space succeeds when user has space.create permission', async ({
    page,
    chatPage
  }) => {
    await createAndLoginTestUser(page);
    await chatPage.goto();

    // Create a space via the modal
    const spaceName = 'My Test Space';
    await chatPage.createSpace(spaceName);

    // Should have navigated to the new space
    await expect(page.getByText(spaceName)).toBeVisible();
  });
});

test.describe('Space Name Validation', () => {
  const MAX_SPACE_NAME_LENGTH = 42;

  test('shows error when space name exceeds maximum length', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.newSpace);

    // Enter a name that's too long (43 characters)
    const tooLongName = 'a'.repeat(MAX_SPACE_NAME_LENGTH + 1);
    await page.getByLabel('Name').fill(tooLongName);

    // Error message should appear
    await expect(
      page.getByText(`Space name cannot exceed ${MAX_SPACE_NAME_LENGTH} characters`)
    ).toBeVisible();

    // Submit button should be disabled
    await expect(page.locator('button[type="submit"]')).toBeDisabled();
  });

  test('allows space name at exactly maximum length', async ({ page }) => {
    await createAndLoginTestUser(page);
    await page.goto(routes.newSpace);

    // Enter a name at exactly the max length (42 characters)
    const maxLengthName = 'a'.repeat(MAX_SPACE_NAME_LENGTH);
    await page.getByLabel('Name').fill(maxLengthName);

    // No error should appear
    await expect(
      page.getByText(`Space name cannot exceed ${MAX_SPACE_NAME_LENGTH} characters`)
    ).not.toBeVisible();

    // Submit button should be enabled
    await expect(page.locator('button[type="submit"]')).toBeEnabled();
  });

  test('backend rejects space name over maximum length', async ({ page }) => {
    await createAndLoginTestUser(page);

    // Try to create a space with a name that's too long via GraphQL
    const tooLongName = 'a'.repeat(MAX_SPACE_NAME_LENGTH + 1);
    const response = await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					mutation CreateSpace($input: CreateSpaceInput!) {
						createSpace(input: $input) { id }
					}
				`,
        variables: { input: { name: tooLongName } }
      }
    });

    const data = await response.json();

    // Should have an error about name being too long
    expect(data.errors).toBeDefined();
    expect(data.errors.length).toBeGreaterThan(0);
    expect(data.errors[0].message).toContain('space name exceeds maximum length');
  });
});
