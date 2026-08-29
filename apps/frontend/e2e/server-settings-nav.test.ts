import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  grantPermission,
  logoutCurrentUser,
  loginAsAdminAndUsePrimaryServer,
  type TestUser
} from './fixtures/testUser';
import { browserAuthenticationHeaders } from './fixtures/csrf';
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
  const loginResponse = await page.request.post('/auth/browser/login', {
    headers: await browserAuthenticationHeaders(page),
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
  test.describe('Settings entry visibility', () => {
    test('server admin sees Settings', async ({ serverAdminPage }) => {
      const { page } = serverAdminPage;

      // Create user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Navigate to server
      await page.goto(routes.chat);
      await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

      // Every member sees Settings; permissions filter its Server Configuration group.
      await serverAdminPage.expectSettingsLinkVisible();
    });

    test('regular member enters Settings through Appearance', async ({ serverAdminPage }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to server
      await page.goto(routes.chat);
      await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

      // Fresh servers grant bot.create to everyone, so Bots is this member's
      // only Server Configuration page. Settings still starts at Appearance.
      await serverAdminPage.expectSettingsLinkVisible();
      await serverAdminPage.settingsLink.click();
      await page.waitForURL(routes.settingsAppearance);
      await expect(page.getByRole('heading', { name: 'Appearance' })).toBeVisible();
      await serverAdminPage.expectBotsNavVisible();
      await serverAdminPage.expectGeneralNavNotVisible();
    });

    test('member with only role.assign permission sees Settings', async ({ serverAdminPage }) => {
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
      await page.goto(routes.chat);
      await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

      // Settings also contains the member's User Preferences.
      await serverAdminPage.expectSettingsLinkVisible();
    });

    test('member with only user.delete-any permission sees Settings', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Grant user.delete-any to everyone role. Like the other tests in
      // this block, this picks a single admin-tier permission that is part
      // of the HasAnyAdminPermission set and verifies that holding just
      // that one perm is enough to surface its matching Server Configuration item.
      await grantPermission(page, 'everyone', 'user.delete-any');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to server
      await page.goto(routes.chat);
      await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

      // Settings also contains the member's User Preferences.
      await serverAdminPage.expectSettingsLinkVisible();
    });

    test('member with only role.manage permission sees Settings', async ({ serverAdminPage }) => {
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
      await page.goto(routes.chat);
      await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

      // Settings also contains the member's User Preferences.
      await serverAdminPage.expectSettingsLinkVisible();
    });
  });

  test.describe('Server Admin nav item filtering', () => {
    test('server admin sees all management nav items', async ({ serverAdminPage }) => {
      const { page } = serverAdminPage;

      // Create user and load the primary server
      await createAndLoginTestUser(page);
      const server = await usePrimaryServerViaAPI(page);

      // Navigate to settings
      await serverAdminPage.goto(server.id);

      // Admin should see all nav items
      await serverAdminPage.expectGeneralNavVisible();
      await serverAdminPage.expectMembersNavVisible();
      await serverAdminPage.expectBotsNavVisible();
      await serverAdminPage.expectRolesNavVisible();
    });

    test('member with only role.assign permission does not see Permissions or Members nav items', async ({
      serverAdminPage
    }) => {
      const { page } = serverAdminPage;

      // Create admin user and load the primary server
      await createAndLoginTestUser(page);
      await usePrimaryServerViaAPI(page);

      // Grant role.assign to everyone role. Role assignment is a targeted
      // mutation permission; the Permissions matrix still requires role.manage,
      // and Members still requires admin.view-users because it lists user records.
      await grantPermission(page, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to an admin shell route that is still available to a viewer
      // with an admin-scoped action permission.
      await page.goto(routes.serverAdmin('moderation'));

      // Should not see Permissions or Members.
      await serverAdminPage.expectRolesNavNotVisible();
      await serverAdminPage.expectMembersNavNotVisible();

      // Should NOT see unrelated permission-gated nav items.
      await serverAdminPage.expectGeneralNavNotVisible();

      // Direct Permissions access is also denied because the page loads the
      // server role-permission matrix, which requires role.manage.
      await page.goto(routes.serverAdminPermissions);
      await serverAdminPage.expectAccessDenied();
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

      // Grant only role.manage to everyone role (no server.manage)
      await grantPermission(page, 'everyone', 'role.manage');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      // Navigate to the concrete section unlocked by role.manage
      await page.goto(routes.serverAdminPermissions);

      // Should see Permissions, NOT Access Denied and NOT General settings
      await serverAdminPage.expectAccessNotDenied();
      await expect(page.getByRole('heading', { name: 'Permissions', level: 1 })).toBeVisible();
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

    test('member without admin.view-users permission sees Access Denied on Members settings', async ({
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
