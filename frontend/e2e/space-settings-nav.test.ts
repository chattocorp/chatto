import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  loginAsAdminAndUsePrimarySpace,
  type TestUser
} from './fixtures/testUser';
import * as routes from './routes';

interface TestSpace {
  id: string;
  name: string;
}

/**
 * Issue #330 / ADR-027: createSpace mutation is gone. Re-login as e2eadmin
 * (bootstrap space owner) and return the primary space so admin-style
 * navigation tests still run with sufficient permissions.
 */
async function createSpaceViaAPI(page: Page, _name?: string): Promise<TestSpace> {
  return loginAsAdminAndUsePrimarySpace(page);
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
  await page.request.post('/auth/logout');
}

/**
 * Vestigial helper kept for source-compat: post-#330 PR(a) joinSpace is
 * gone — server membership is implicit on signup.
 */
async function joinSpaceViaAPI(_page: Page, _spaceId: string): Promise<void> {
  // no-op
}

/**
 * Grants a space permission to a role via GraphQL API.
 */
async function grantPermission(
  page: Page,
  role: string,
  permission: string
): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation GrantPerm($input: GrantPermissionInput!) {
					grantPermission(input: $input)
				}
			`,
      variables: { input: { roleName: role, permission } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.grantPermission).toBe(true);
}

test.describe('Space Admin Navigation Permissions', () => {
  test.describe('Space Admin button visibility', () => {
    test('space admin sees Space Admin button', async ({ spaceAdminPage }) => {
      const { page } = spaceAdminPage;

      // Create user and space (creator is admin)
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to space
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: space.name })).toBeVisible();

      // Admin should see Space Admin link
      await spaceAdminPage.expectAdminLinkVisible();
    });

    test('regular member without admin permissions does not see Space Admin button', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to space
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: space.name })).toBeVisible();

      // Regular member without admin permissions should NOT see Space Admin link
      await spaceAdminPage.expectAdminLinkNotVisible();
    });

    test('member with only role.assign permission sees Space Admin button', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant role.assign to everyone role
      await grantPermission(page, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to space
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: space.name })).toBeVisible();

      // Member with role.assign should see Space Admin link
      await spaceAdminPage.expectAdminLinkVisible();
    });

    test('member with only user.delete-any permission sees Space Admin button', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant user.delete-any to everyone role. Like the other tests in
      // this block, this picks a single admin-tier permission that is part
      // of the HasAnyAdminPermission set and verifies that holding just
      // that one perm is enough to surface the Space Admin link.
      await grantPermission(page, 'everyone', 'user.delete-any');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to space
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: space.name })).toBeVisible();

      // Member with user.delete-any should see Space Admin link
      await spaceAdminPage.expectAdminLinkVisible();
    });

    test('member with only role.manage permission sees Space Admin button', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant role.manage to everyone role
      await grantPermission(page, 'everyone', 'role.manage');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to space
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: space.name })).toBeVisible();

      // Member with role.manage should see Space Admin link
      await spaceAdminPage.expectAdminLinkVisible();
    });
  });

  test.describe('Settings nav item filtering', () => {
    test('space admin sees all settings nav items', async ({ spaceAdminPage }) => {
      const { page } = spaceAdminPage;

      // Create user and space (creator is admin)
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to settings
      await spaceAdminPage.goto(space.id);

      // Admin should see all nav items
      await spaceAdminPage.expectGeneralNavVisible();
      await spaceAdminPage.expectMembersNavVisible();
      await spaceAdminPage.expectRolesNavVisible();
    });

    test('member with only role.assign permission sees Members nav item', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant role.assign to everyone role (enables Members page access)
      await grantPermission(page, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to settings
      await spaceAdminPage.gotoMembersDirectly(space.id);

      // Wait for page to load
      await expect(page.getByRole('heading', { name: 'Members', exact: true })).toBeVisible();

      // Should see Members (has role.assign)
      await spaceAdminPage.expectMembersNavVisible();

      // Should NOT see other permission-gated nav items
      await spaceAdminPage.expectGeneralNavNotVisible();
      await spaceAdminPage.expectRolesNavNotVisible();
    });

    test('member with only role.manage permission sees Roles nav item', async ({
      spaceAdminPage,
      spaceRolesPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant role.manage to everyone role (enables Roles page access)
      await grantPermission(page, 'everyone', 'role.manage');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to roles page using the roles page object
      await spaceRolesPage.gotoRolesList(space.id);

      // Should see Roles (has role.manage)
      await spaceAdminPage.expectRolesNavVisible();

      // Should NOT see other permission-gated nav items
      await spaceAdminPage.expectGeneralNavNotVisible();
      await spaceAdminPage.expectMembersNavNotVisible();
    });
  });

  test.describe('Route authorization', () => {
    test('member without any admin permissions sees Access Denied on General settings', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create and login as non-admin user (no admin permissions)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to a concrete admin URL
      await page.goto(routes.serverAdminGeneral);

      // Should see Access Denied (has no admin permissions at all)
      await spaceAdminPage.expectAccessDenied();
    });

    test('member with partial admin permissions can access their concrete section', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant only role.assign to everyone role (no space.manage)
      await grantPermission(page, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to the concrete section unlocked by role.assign
      await page.goto(routes.serverAdminMembers);

      // Should see Members, NOT Access Denied and NOT General settings
      await spaceAdminPage.expectAccessNotDenied();
      await expect(page.getByRole('heading', { name: 'Members', exact: true })).toBeVisible();
      await spaceAdminPage.expectGeneralSettingsNotVisible();
    });

    test('admin uses General as the first concrete admin page', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create user and space (creator is admin)
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      await spaceAdminPage.goto(space.id);

      await spaceAdminPage.expectGeneralSettingsVisible();
      await spaceAdminPage.expectAccessNotDenied();
    });

    test('admin sees General settings on /admin/general page', async ({ spaceAdminPage }) => {
      const { page } = spaceAdminPage;

      // Create user and space (creator is admin)
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to General settings page directly
      await spaceAdminPage.gotoGeneralDirectly(space.id);

      // Admin should see General settings content
      await spaceAdminPage.expectGeneralSettingsVisible();
      await spaceAdminPage.expectAccessNotDenied();
    });

    test('member without space.manage permission sees Access Denied on General settings', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant only role.assign to member role (NOT space.manage)
      await grantPermission(page, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to General settings
      await page.goto(routes.serverAdminGeneral);

      // Should see Access Denied (no space.manage permission)
      await spaceAdminPage.expectAccessDenied();
    });

    test('member without role.assign permission sees Access Denied on Members settings', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create and login as non-admin user (no admin permissions)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to Members settings
      await spaceAdminPage.gotoMembersDirectly(space.id);

      // Should see Access Denied
      await spaceAdminPage.expectAccessDenied();
    });

    test('member without role.manage permission sees Access Denied on Roles settings', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create and login as non-admin user (no admin permissions)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to Roles settings
      await spaceAdminPage.gotoRolesDirectly(space.id);

      // Should see Access Denied
      await spaceAdminPage.expectAccessDenied();
    });

  });
});
