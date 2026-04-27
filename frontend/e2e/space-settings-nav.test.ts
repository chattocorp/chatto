import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { createAndLoginTestUser, type TestUser } from './fixtures/testUser';
import * as routes from './routes';

interface TestSpace {
  id: string;
  name: string;
}

/**
 * Creates a space via GraphQL API (requires authenticated user).
 * The creator becomes the space admin.
 */
async function createSpaceViaAPI(page: Page, name?: string): Promise<TestSpace> {
  const timestamp = Date.now();
  const spaceName = name ?? `Admin Nav Test Space ${timestamp}`;

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
					}
				}
			`,
      variables: {
        input: {
          name: spaceName,
          description: 'A space for testing admin navigation'
        }
      }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.createSpace).toBeTruthy();

  return {
    id: data.data.createSpace.id,
    name: data.data.createSpace.name
  };
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

/**
 * Joins a space via GraphQL API.
 */
async function joinSpaceViaAPI(page: Page, spaceId: string): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation JoinSpace($input: JoinSpaceInput!) { joinSpace(input: $input)
				}
			`,
      variables: { input: { spaceId } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.joinSpace).toBeTruthy();
}

/**
 * Grants a space permission to a role via GraphQL API.
 */
async function grantSpacePermission(
  page: Page,
  spaceId: string,
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
				mutation GrantSpacePermission($input: GrantSpacePermissionInput!) {
					grantSpacePermission(input: $input)
				}
			`,
      variables: { input: { spaceId, role, permission } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.grantSpacePermission).toBe(true);
}

test.describe('Space Admin Navigation Permissions', () => {
  test.describe('Space Admin button visibility', () => {
    test('space admin sees Space Admin button', async ({ spaceAdminPage }) => {
      const { page } = spaceAdminPage;

      // Create user and space (creator is admin)
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to space
      await page.goto(routes.space(space.id));
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
      await page.goto(routes.space(space.id));
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
      await grantSpacePermission(page, space.id, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to space
      await page.goto(routes.space(space.id));
      await expect(page.getByRole('heading', { name: space.name })).toBeVisible();

      // Member with role.assign should see Space Admin link
      await spaceAdminPage.expectAdminLinkVisible();
    });

    test('member with only member.invite permission sees Space Admin button', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant member.invite to everyone role
      await grantSpacePermission(page, space.id, 'everyone', 'member.invite');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to space
      await page.goto(routes.space(space.id));
      await expect(page.getByRole('heading', { name: space.name })).toBeVisible();

      // Member with member.invite should see Space Admin link
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
      await grantSpacePermission(page, space.id, 'everyone', 'role.manage');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to space
      await page.goto(routes.space(space.id));
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
      await spaceAdminPage.expectHomeNavVisible();
      await spaceAdminPage.expectGeneralNavVisible();
      await spaceAdminPage.expectMembersNavVisible();
      await spaceAdminPage.expectInvitesNavVisible();
      await spaceAdminPage.expectRolesNavVisible();
    });

    test('member with only role.assign permission sees Home and Members nav items', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant role.assign to everyone role (enables Members page access)
      await grantSpacePermission(page, space.id, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to settings
      await spaceAdminPage.gotoMembersDirectly(space.id);

      // Wait for page to load
      await expect(page.getByRole('heading', { name: 'Members' })).toBeVisible();

      // Should see Home (always visible) and Members (has role.assign)
      await spaceAdminPage.expectHomeNavVisible();
      await spaceAdminPage.expectMembersNavVisible();

      // Should NOT see other permission-gated nav items
      await spaceAdminPage.expectGeneralNavNotVisible();
      await spaceAdminPage.expectInvitesNavNotVisible();
      await spaceAdminPage.expectRolesNavNotVisible();
    });

    test('member with only member.invite permission sees Home and Invites nav items', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant member.invite to everyone role (enables Invites page access)
      await grantSpacePermission(page, space.id, 'everyone', 'member.invite');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to invites page
      await spaceAdminPage.gotoInvitesDirectly(space.id);

      // Should see Home (always visible) and Invites (has member.invite)
      await spaceAdminPage.expectHomeNavVisible();
      await spaceAdminPage.expectInvitesNavVisible();

      // Should NOT see other permission-gated nav items
      await spaceAdminPage.expectGeneralNavNotVisible();
      await spaceAdminPage.expectMembersNavNotVisible();
      await spaceAdminPage.expectRolesNavNotVisible();
    });

    test('member with only role.manage permission sees Home and Roles nav items', async ({
      spaceAdminPage,
      spaceRolesPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant role.manage to everyone role (enables Roles page access)
      await grantSpacePermission(page, space.id, 'everyone', 'role.manage');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to roles page using the roles page object
      await spaceRolesPage.gotoRolesList(space.id);

      // Should see Home (always visible) and Roles (has role.manage)
      await spaceAdminPage.expectHomeNavVisible();
      await spaceAdminPage.expectRolesNavVisible();

      // Should NOT see other permission-gated nav items
      await spaceAdminPage.expectGeneralNavNotVisible();
      await spaceAdminPage.expectMembersNavNotVisible();
      await spaceAdminPage.expectInvitesNavNotVisible();
    });
  });

  test.describe('Route authorization', () => {
    test('member without any admin permissions sees Access Denied on settings home', async ({
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

      // Navigate directly to settings URL
      await page.goto(routes.spaceAdmin(space.id));

      // Should see Access Denied (has no admin permissions at all)
      await spaceAdminPage.expectAccessDenied();
    });

    test('member with partial admin permissions sees placeholder on settings home', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant only role.assign to everyone role (no space.manage)
      await grantSpacePermission(page, space.id, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to settings home
      await page.goto(routes.spaceAdmin(space.id));

      // Should see placeholder, NOT Access Denied and NOT General settings
      await spaceAdminPage.expectAccessNotDenied();
      await spaceAdminPage.expectGeneralSettingsNotVisible();
      await spaceAdminPage.expectAdminPlaceholderVisible();
    });

    test('admin sees placeholder on settings home (not General settings)', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create user and space (creator is admin)
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to settings home
      await spaceAdminPage.goto(space.id);

      // Admin should see placeholder on home, NOT General settings
      await spaceAdminPage.expectAdminPlaceholderVisible();
      await spaceAdminPage.expectGeneralSettingsNotVisible();
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
      await grantSpacePermission(page, space.id, 'everyone', 'role.assign');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to General settings
      await page.goto(routes.spaceAdminGeneral(space.id));

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

    test('member without member.invite permission sees Access Denied on Invites settings', async ({
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

      // Navigate directly to Invites settings
      await page.goto(routes.spaceAdminInvites(space.id));

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

    test('member with correct permission can access authorized route', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant member.invite to everyone role
      await grantSpacePermission(page, space.id, 'everyone', 'member.invite');

      // Create and login as non-admin user
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to Invites settings
      await spaceAdminPage.gotoInvitesDirectly(space.id);

      // Should see Invites content, NOT access denied
      await spaceAdminPage.expectAccessNotDenied();
      await spaceAdminPage.expectInvitesPageVisible();
    });
  });
});
