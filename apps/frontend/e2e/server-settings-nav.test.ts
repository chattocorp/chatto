import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  grantPermission,
  logoutCurrentUser,
  loginAsAdminAndUsePrimaryServer,
  type TestUser
} from './fixtures/testUser';
import * as routes from './routes';

interface TestServer {
  id: string;
  name: string;
}

/** Log in as the bootstrap admin and return the primary server metadata. */
async function usePrimaryServerViaAPI(page: Page, _name?: string): Promise<TestServer> {
  return loginAsAdminAndUsePrimaryServer(page);
}

/**
 * Creates a second test user with verified email.
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

  // Verify email to satisfy account-creation requirements
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
  await logoutCurrentUser(page);
}

test.describe('Server Admin Navigation Permissions', () => {
  test.describe('Server Admin button visibility', () => {
    test('server admin sees Server Admin button', async ({ serverAdminPage }) => {
      const { page } = serverAdminPage;

      // Create user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Navigate to server
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

      // Admin should see Server Admin link
      await serverAdminPage.expectAdminLinkVisible();
    });

    test('regular member without admin permissions does not see Server Admin button', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to server
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

      // Regular member without admin permissions should NOT see Server Admin link
      await serverAdminPage.expectAdminLinkNotVisible();
    });

    test('member with only role.assign permission sees Server Admin button', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Grant role.assign to everyone role
      await grantPermission(page, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to server
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

      // Member with role.assign should see Server Admin link
      await serverAdminPage.expectAdminLinkVisible();
    });

    test('member with only user.delete-any permission sees Server Admin button', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Grant user.delete-any to everyone role. Like the other tests in
      // this block, this picks a single admin-tier permission that is part
      // of the HasAnyAdminPermission set and verifies that holding just
      // that one perm is enough to surface the Server Admin link.
      await grantPermission(page, 'everyone', 'user.delete-any');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to server
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

      // Member with user.delete-any should see Server Admin link
      await serverAdminPage.expectAdminLinkVisible();
    });

    test('member with only role.manage permission sees Server Admin button', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Grant role.manage to everyone role
      await grantPermission(page, 'everyone', 'role.manage');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to server
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

      // Member with role.manage should see Server Admin link
      await serverAdminPage.expectAdminLinkVisible();
    });
  });

  test.describe('Settings nav item filtering', () => {
    test('server admin sees all settings nav items', async ({ serverAdminPage }) => {
      const { page } = serverAdminPage;

      // Create user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Navigate to settings
      await serverAdminPage.goto(server.id);

      // Admin should see all nav items
      await serverAdminPage.expectGeneralNavVisible();
      await serverAdminPage.expectMembersNavVisible();
      await serverAdminPage.expectRolesNavVisible();
    });

    test('member with only role.assign permission sees Members nav item', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Grant role.assign to everyone role (enables Members page access)
      await grantPermission(page, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to settings
      await serverAdminPage.gotoMembersDirectly(server.id);

      // Wait for page to load
      await expect(page.getByRole('heading', { name: 'Members', exact: true })).toBeVisible();

      // Should see Members (has role.assign)
      await serverAdminPage.expectMembersNavVisible();

      // Should NOT see other permission-gated nav items
      await serverAdminPage.expectGeneralNavNotVisible();
      await serverAdminPage.expectRolesNavNotVisible();
    });

    test('member with only role.manage permission sees Roles nav item', async ({
      serverAdminPage,
      serverRolesPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Grant role.manage to everyone role (enables Roles page access)
      await grantPermission(page, 'everyone', 'role.manage');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate directly to roles page using the roles page object
      await serverRolesPage.gotoRolesList(server.id);

      // Should see Roles (has role.manage)
      await serverAdminPage.expectRolesNavVisible();

      // Should NOT see other permission-gated nav items
      await serverAdminPage.expectGeneralNavNotVisible();
      await serverAdminPage.expectMembersNavNotVisible();
    });
  });

  test.describe('Route authorization', () => {
    test('member without any admin permissions sees Access Denied on General settings', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Create and login as non-admin user (no admin permissions)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate directly to a concrete admin URL
      await page.goto(routes.serverAdminGeneral);

      // Should see Access Denied (has no admin permissions at all)
      await serverAdminPage.expectAccessDenied();
    });

    test('member with partial admin permissions can access their concrete section', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Grant only role.assign to everyone role (no server.manage)
      await grantPermission(page, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to the concrete section unlocked by role.assign
      await page.goto(routes.serverAdminMembers);

      // Should see Members, NOT Access Denied and NOT General settings
      await serverAdminPage.expectAccessNotDenied();
      await expect(page.getByRole('heading', { name: 'Members', exact: true })).toBeVisible();
      await serverAdminPage.expectGeneralSettingsNotVisible();
    });

    test('admin uses General as the first concrete admin page', async ({ serverAdminPage }) => {
      const { page } = serverAdminPage;

      // Create user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      await serverAdminPage.goto(server.id);

      await serverAdminPage.expectGeneralSettingsVisible();
      await serverAdminPage.expectAccessNotDenied();
    });

    test('admin sees General settings on /admin/general page', async ({ serverAdminPage }) => {
      const { page } = serverAdminPage;

      // Create user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Navigate to General settings page directly
      await serverAdminPage.gotoGeneralDirectly(server.id);

      // Admin should see General settings content
      await serverAdminPage.expectGeneralSettingsVisible();
      await serverAdminPage.expectAccessNotDenied();
    });

    test('member without server.manage permission sees Access Denied on General settings', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Grant only role.assign to member role (NOT server.manage)
      await grantPermission(page, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate directly to General settings
      await page.goto(routes.serverAdminGeneral);

      // Should see Access Denied (no server.manage permission)
      await serverAdminPage.expectAccessDenied();
    });

    test('member without role.assign permission sees Access Denied on Members settings', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Create and login as non-admin user (no admin permissions)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate directly to Members settings
      await serverAdminPage.gotoMembersDirectly(server.id);

      // Should see Access Denied
      await serverAdminPage.expectAccessDenied();
    });

    test('member without role.manage permission sees Access Denied on Roles settings', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Create and login as non-admin user (no admin permissions)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate directly to Roles settings
      await serverAdminPage.gotoRolesDirectly(server.id);

      // Should see Access Denied
      await serverAdminPage.expectAccessDenied();
    });
  });
});
