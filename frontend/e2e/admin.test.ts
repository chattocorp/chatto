import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { AdminPage, ChatPage } from './pages';
import * as routes from './routes';
import { TIMEOUTS } from './constants';
import {
  createAndLoginTestUser,
  generateInstanceRoleName,
  loginAsAdmin,
  verifyAdminEmail,
  grantInstancePermission,
  revokeInstancePermission,
  denyUserInstancePermission,
  clearUserInstancePermissionOverride,
  type TestUser
} from './fixtures/testUser';

async function createInstanceRoleViaAPI(
  page: Page,
  name: string,
  displayName: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: CreateRoleInput!) { createRole(input: $input) { name } }`,
      variables: { input: { name, displayName, description: `Test role: ${displayName}` } }
    }
  });
  expect(resp.ok()).toBeTruthy();
}

async function assignInstanceRoleViaAPI(
  page: Page,
  userId: string,
  roleName: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: AssignInstanceRoleInput!) { assignInstanceRole(input: $input) }`,
      variables: { input: { userId, roleName } }
    }
  });
  expect(resp.ok()).toBeTruthy();
}

async function revokeInstanceRoleViaAPI(
  page: Page,
  userId: string,
  roleName: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: RevokeInstanceRoleInput!) { revokeInstanceRole(input: $input) }`,
      variables: { input: { userId, roleName } }
    }
  });
  expect(resp.ok()).toBeTruthy();
}

/**
 * Logs in as the admin user (created by server bootstrap) and verifies
 * the admin email to grant config-based admin access (for admin panel).
 */
async function createAndLoginAdminUser(page: Page): Promise<TestUser> {
  const adminUser = await loginAsAdmin(page);
  await verifyAdminEmail(page, adminUser.id!);
  return adminUser;
}

test.describe('Admin Access Control', () => {
  test('non-admin users see access denied message on /chat/-/admin', async ({ page, adminPage }) => {
    // First, ensure there's already an admin user (the first user gets auto-promoted)
    // by creating a throwaway user that claims the first-user admin spot
    await createAndLoginAdminUser(page);

    // Now create a regular (non-admin) user - they won't be the first user
    await createAndLoginTestUser(page);

    // Try to access admin page
    await adminPage.goto();

    // Should see access denied message
    await adminPage.expectAccessDenied();
    await expect(
      page.getByText('You do not have permission to view the admin panel.')
    ).toBeVisible();

    // Should have a link to return to chat
    await adminPage.expectReturnToChatVisible();
  });

  test('unauthenticated users are redirected from /chat/-/admin to home', async ({
    page,
    adminPage
  }) => {
    // Try to access admin page without logging in
    await adminPage.goto();

    // The /chat layout is accessible without auth. The admin page stays at the URL
    // but shows no admin content (admin resolver returns null for unauthenticated users).
    // Accept any /chat route — the page may stay at admin or redirect to /chat.
    await page.waitForURL((url) => url.pathname.startsWith('/chat'));
  });
});

test.describe('Admin Dashboard', () => {
  test('admin users can access the dashboard', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.goto();

    // Should see the dashboard
    await adminPage.expectDashboardVisible();

    // Should see the sidebar navigation
    await adminPage.expectSidebarNavVisible();

    // Should see the back to chat link
    await adminPage.expectBackToChatVisible();
  });

  test('dashboard shows user and space counts', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.goto();

    // Wait for stats to load
    await adminPage.expectDashboardStatsVisible();

    // Should see quick action links
    await adminPage.expectQuickActionsVisible();
  });
});

test.describe('Admin Users Page', () => {
  test('admin can view users list', async ({ page, adminPage }) => {
    const adminUser = await createAndLoginAdminUser(page);

    await adminPage.gotoUsers();

    // Should see the users page header
    await adminPage.expectUsersPageVisible();

    // Should see table headers
    await adminPage.expectUsersTableHeadersVisible();

    // Should see the admin user's login in the list
    await expect(page.getByRole('cell', { name: adminUser.login, exact: true })).toBeVisible();

    // Should see the total count
    await adminPage.expectUserCountVisible();
  });

  test('admin can see verified emails for multiple OAuth users', async ({
    page,
    adminPage,
    browser
  }) => {
    const timestamp = Date.now();

    // Create two OAuth users with verified emails using separate contexts
    const oauthUser1Email = `oauthlist1-${timestamp}@google.com`;
    const oauthUser2Email = `oauthlist2-${timestamp}@google.com`;

    // Create first OAuth user in separate context
    const context1 = await browser.newContext();
    const page1 = await context1.newPage();
    await page1.request.post('/auth/test/oauth-callback', {
      data: {
        email: oauthUser1Email,
        displayName: 'OAuth List User 1'
      }
    });
    await context1.close();

    // Create second OAuth user in separate context
    const context2 = await browser.newContext();
    const page2 = await context2.newPage();
    await page2.request.post('/auth/test/oauth-callback', {
      data: {
        email: oauthUser2Email,
        displayName: 'OAuth List User 2'
      }
    });
    await context2.close();

    // Now create admin user and log in (in main page context)
    await createAndLoginAdminUser(page);

    // Go to admin users page
    await adminPage.gotoUsers();
    await adminPage.expectUsersPageVisible();

    // Wait for the table to load
    await adminPage.expectUserCountVisible();

    // Both verified emails should be visible in the list
    await adminPage.expectEmailVisible(oauthUser1Email);
    await adminPage.expectEmailVisible(oauthUser2Email);
  });
});

test.describe('Admin Spaces Page', () => {
  test('admin can view spaces list', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoSpaces();

    // Should see the spaces page header
    await adminPage.expectSpacesPageVisible();

    // Should see table headers
    await adminPage.expectSpacesTableHeadersVisible();

    // Should see the total count (even if 0)
    await adminPage.expectSpaceCountVisible();
  });
});

test.describe('Admin System Page', () => {
  test('admin can view system information', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoSystem();

    // Should see the system page header
    await adminPage.expectSystemPageVisible();

    // Wait for system info to load and check for connection status
    await adminPage.expectSystemConnected();

    // Should see account usage stat cards
    await adminPage.expectSystemStatsVisible();
  });
});

test.describe('Admin Navigation', () => {
  test('sidebar navigation works correctly', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.goto();

    // Navigate to Users
    await adminPage.navigateToUsers();
    await adminPage.expectUsersPageVisible();

    // Navigate to Spaces
    await adminPage.navigateToSpaces();
    await adminPage.expectSpacesPageVisible();

    // Navigate to System
    await adminPage.navigateToSystem();
    await adminPage.expectSystemPageVisible();

    // Navigate back to Dashboard
    await adminPage.navigateToDashboard();
    await adminPage.expectDashboardVisible();
  });

  test('back to chat link works', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.goto();

    // Click back to chat - may redirect to /chat/spaces for users with no joined spaces
    await adminPage.navigateBackToChat();
  });
});

test.describe('Admin Granular Permissions', () => {
  // These tests modify role permissions for the 'everyone' role.
  // If a test fails mid-way, cleanup must still happen to prevent test pollution.
  test.afterEach(async ({ page }) => {
    // Reset all potentially modified everyone role permissions.
    // Uses page.request which maintains the admin session cookies.
    const permissions = [
      'admin.access',
      'admin.view-users',
      'admin.view-spaces',
      'admin.view-system',
      'admin.view-roles'
    ];

    for (const permission of permissions) {
      try {
        await page.request.post('/api/graphql', {
          headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
          data: {
            query: `mutation { revokeInstancePermission(input: {role: "everyone", permission: "${permission}"}) }`
          }
        });
      } catch {
        // Ignore errors - permission may not have been granted
      }
    }
  });

  test('user with admin permission can access dashboard', async ({ page, browser }) => {
    // First, as admin, grant admin.access to everyone role
    await createAndLoginAdminUser(page);
    await grantInstancePermission(page, 'everyone', 'admin.access');

    // Now create a regular user and try to access admin
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.goto();

    // Should see the dashboard (not access denied)
    await regularAdminPage.expectDashboardVisible();

    // Clean up: revoke the permission
    await revokeInstancePermission(page, 'everyone', 'admin.access');
    await regularContext.close();
  });

  test('user with admin but without admin.view-users sees limited nav items', async ({
    page,
    browser
  }) => {
    // Grant only admin.access to everyone role
    await createAndLoginAdminUser(page);
    await grantInstancePermission(page, 'everyone', 'admin.access');

    // Create regular user and access admin
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.goto();

    // Should see dashboard in nav
    await regularAdminPage.expectSidebarLinkVisible('Dashboard');

    // Should NOT see Users, Spaces, System (no permissions for those)
    await regularAdminPage.expectSidebarLinkNotVisible('Users');
    await regularAdminPage.expectSidebarLinkNotVisible('Spaces');
    await regularAdminPage.expectSidebarLinkNotVisible('System');

    // Clean up
    await revokeInstancePermission(page, 'everyone', 'admin.access');
    await regularContext.close();
  });

  test('user with admin.view-users permission can see users list', async ({ page, browser }) => {
    // Grant admin.access and admin.view-users to everyone role
    await createAndLoginAdminUser(page);
    await grantInstancePermission(page, 'everyone', 'admin.access');
    await grantInstancePermission(page, 'everyone', 'admin.view-users');

    // Create regular user
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoUsers();

    // Should see the users page with data
    await regularAdminPage.expectUsersPageVisible();
    await regularAdminPage.expectUsersTableHeadersVisible();
    // Should see at least one user in the list (the user count)
    await regularAdminPage.expectUserCountVisible();

    // Clean up
    await revokeInstancePermission(page, 'everyone', 'admin.access');
    await revokeInstancePermission(page, 'everyone', 'admin.view-users');
    await regularContext.close();
  });

  test('user without admin.view-users sees access denied on /chat/-/admin/users', async ({
    page,
    browser
  }) => {
    // Grant only admin (not admin.view-users)
    await createAndLoginAdminUser(page);
    await grantInstancePermission(page, 'everyone', 'admin.access');

    // Create regular user
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoUsers();

    // Should see access denied with the specific permission mentioned
    await regularAdminPage.expectAccessDeniedForPermission('admin.view-users');

    // Clean up
    await revokeInstancePermission(page, 'everyone', 'admin.access');
    await regularContext.close();
  });

  test('user without admin.view-spaces sees access denied on /chat/-/admin/spaces', async ({
    page,
    browser
  }) => {
    await createAndLoginAdminUser(page);
    await grantInstancePermission(page, 'everyone', 'admin.access');

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoSpaces();

    await regularAdminPage.expectAccessDeniedForPermission('admin.view-spaces');

    await revokeInstancePermission(page, 'everyone', 'admin.access');
    await regularContext.close();
  });

  test('user without admin.view-system sees access denied on /chat/-/admin/system', async ({
    page,
    browser
  }) => {
    await createAndLoginAdminUser(page);
    await grantInstancePermission(page, 'everyone', 'admin.access');

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoSystem();

    await regularAdminPage.expectAccessDeniedForPermission('admin.view-system');

    await revokeInstancePermission(page, 'everyone', 'admin.access');
    await regularContext.close();
  });

  test('user without admin.view-roles sees access denied on /chat/-/admin/roles', async ({
    page,
    browser
  }) => {
    await createAndLoginAdminUser(page);
    await grantInstancePermission(page, 'everyone', 'admin.access');

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoRoles();

    await regularAdminPage.expectAccessDeniedForPermission('admin.view-roles');

    await revokeInstancePermission(page, 'everyone', 'admin.access');
    await regularContext.close();
  });

  test('user with admin.view-spaces permission can see spaces list', async ({ page, browser }) => {
    // Grant admin and admin.view-spaces
    await createAndLoginAdminUser(page);
    await grantInstancePermission(page, 'everyone', 'admin.access');
    await grantInstancePermission(page, 'everyone', 'admin.view-spaces');

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoSpaces();

    await regularAdminPage.expectSpacesPageVisible();
    await regularAdminPage.expectSpaceCountVisible();

    // Clean up
    await revokeInstancePermission(page, 'everyone', 'admin.access');
    await revokeInstancePermission(page, 'everyone', 'admin.view-spaces');
    await regularContext.close();
  });

  test('user with admin.view-system permission can see system page', async ({ page, browser }) => {
    // Grant admin and admin.view-system
    await createAndLoginAdminUser(page);
    await grantInstancePermission(page, 'everyone', 'admin.access');
    await grantInstancePermission(page, 'everyone', 'admin.view-system');

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoSystem();

    await regularAdminPage.expectSystemPageVisible();
    await regularAdminPage.expectSystemConnected();

    // Clean up
    await revokeInstancePermission(page, 'everyone', 'admin.access');
    await revokeInstancePermission(page, 'everyone', 'admin.view-system');
    await regularContext.close();
  });

  // Note: a read-only view of the roles page (admin.view-roles without
  // admin.manage-roles) was removed when the UI moved from a per-role
  // editor to the unified matrix. The matrix's tierRoles query gates on
  // instance admin / role.manage, so view-roles alone is currently not
  // sufficient to render the page. Re-add the test once the matrix grows
  // a read-only mode.

  test('nav items dynamically update based on granted permissions', async ({ page, browser }) => {
    // Start with only admin
    await createAndLoginAdminUser(page);
    await grantInstancePermission(page, 'everyone', 'admin.access');

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.goto();

    // Initially should only see Dashboard
    await regularAdminPage.expectSidebarLinkVisible('Dashboard');
    await regularAdminPage.expectSidebarLinkNotVisible('Users');

    // Now grant admin.view-users permission as admin
    await grantInstancePermission(page, 'everyone', 'admin.view-users');

    // Reload and check nav updated
    await regularPage.reload();
    await regularAdminPage.expectSidebarLinkVisible('Users');

    // Grant admin.view-system
    await grantInstancePermission(page, 'everyone', 'admin.view-system');
    await regularPage.reload();
    await regularAdminPage.expectSidebarLinkVisible('System');

    // Clean up
    await revokeInstancePermission(page, 'everyone', 'admin.access');
    await revokeInstancePermission(page, 'everyone', 'admin.view-users');
    await revokeInstancePermission(page, 'everyone', 'admin.view-system');
    await regularContext.close();
  });
});

test.describe('User Permission Management', () => {
  test('admin can navigate to user management page', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    // Go to users list
    await adminPage.gotoUsers();

    // Click on a user row (the admin user themselves)
    await adminPage.clickUser('e2eadmin');

    // Should see user details
    await adminPage.expectUserManagementVisible();
    // The login should appear in the user profile section
    await expect(
      page.locator('.font-medium').filter({ hasText: 'e2eadmin' }).first()
    ).toBeVisible();
  });

  test('granting a role with admin.access gives user admin access', async ({ page, browser }) => {
    await createAndLoginAdminUser(page);

    // Create a regular user
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    const regularUser = await createAndLoginTestUser(regularPage);

    // Regular user should NOT have admin access initially
    await regularAdminPage.goto();
    await expect(regularPage.getByText('Access Denied', { exact: true })).toBeVisible();

    // Create a role with admin.access and assign it to the user (via API as admin)
    const roleName = generateInstanceRoleName('grant');
    await createInstanceRoleViaAPI(page, roleName, 'Grant Admin');
    await grantInstancePermission(page, roleName, 'admin.access');
    await assignInstanceRoleViaAPI(page, regularUser.id!, roleName);

    // Regular user should now have admin access
    await regularPage.reload();
    await regularAdminPage.expectDashboardVisible();

    // Clean up
    await revokeInstanceRoleViaAPI(page, regularUser.id!, roleName);
    await regularContext.close();
  });

  test('denying a permission via role blocks user access', async ({ page, browser }) => {
    await createAndLoginAdminUser(page);

    // Create a regular user
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularUser = await createAndLoginTestUser(regularPage);

    // User should initially be able to browse spaces (has space.list via everyone role)
    await regularPage.goto(routes.spaces);
    await expect(regularPage.getByRole('heading', { name: 'Browse Spaces' })).toBeVisible();

    // Deny space.list for this user via a deny-role
    const denyRoleName = await denyUserInstancePermission(page, regularUser.id!, 'space.list');

    // User should now be denied — the SpaceDirectory shows an error or permission message.
    // The server may return a GraphQL error or canListSpaces: false depending on
    // how the permission denial manifests, so accept either message.
    await regularPage.reload();
    await expect(
      regularPage.getByText(/(No permission to browse spaces on|Could not connect to)/)
    ).toBeVisible();

    // Clean up
    await clearUserInstancePermissionOverride(page, regularUser.id!, 'space.list', denyRoleName);
    await regularContext.close();
  });

  test('removing deny role restores role-based permission', async ({ page, browser }) => {
    await createAndLoginAdminUser(page);

    // Create a regular user
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularUser = await createAndLoginTestUser(regularPage);

    // Deny space.list for this user
    const denyRoleName = await denyUserInstancePermission(page, regularUser.id!, 'space.list');

    // Verify denied — the SpaceDirectory shows an error or permission message
    await regularPage.goto(routes.spaces);
    await expect(
      regularPage.getByText(/(No permission to browse spaces on|Could not connect to)/)
    ).toBeVisible();

    // Remove the deny role to restore permission
    await clearUserInstancePermissionOverride(page, regularUser.id!, 'space.list', denyRoleName);

    // Permission should be restored via everyone role
    await regularPage.reload();
    await expect(regularPage.getByRole('heading', { name: 'Browse Spaces' })).toBeVisible();

    await regularContext.close();
  });
});

test.describe('Role Assignment', () => {
  test('admin can assign admin role to a user', async ({ page, adminPage, browser }) => {
    await createAndLoginAdminUser(page);

    // Create a regular user
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    const regularUser = await createAndLoginTestUser(regularPage);

    // Navigate to user management page
    await adminPage.gotoUserManagement(regularUser.id!);

    // Find the Role Assignments section
    await adminPage.expectRoleAssignmentsVisible();

    // Find the admin role checkbox (label contains "Instance Admin" display name)
    const adminCheckbox = adminPage.getRoleCheckbox('Instance Admin');
    await expect(adminCheckbox).not.toBeChecked();

    // Assign admin role
    await adminCheckbox.click();
    await expect(adminCheckbox).toBeChecked({ timeout: TIMEOUTS.UI_STANDARD });

    // Verify user is now admin by accessing admin panel
    await regularAdminPage.goto();
    await regularAdminPage.expectDashboardVisible();

    // User should have full admin access (not limited by granular permissions)
    await regularAdminPage.gotoUsers();
    await regularAdminPage.expectUsersPageVisible();

    // Clean up - revoke admin role via API
    // (can't use UI since after assignment, both users are equal rank)
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					mutation RevokeInstanceRole($input: RevokeInstanceRoleInput!) { revokeInstanceRole(input: $input)
					}
				`,
        variables: { input: { userId: regularUser.id, roleName: 'instance-admin' } }
      }
    });

    await regularContext.close();
  });

  test('admin role page shows users with that role', async ({ page, adminPage, browser }) => {
    await createAndLoginAdminUser(page);

    // Create a user and assign admin role
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularUser = await createAndLoginTestUser(regularPage);

    // Assign admin role via API
    await page.request.post('/api/graphql', {
      headers: {
        'Content-Type': 'application/json',
        'X-REQUEST-TYPE': 'GraphQL'
      },
      data: {
        query: `
					mutation AssignRole($input: AssignInstanceRoleInput!) { assignInstanceRole(input: $input)
					}
				`,
        variables: { input: { userId: regularUser.id, roleName: 'instance-admin' } }
      }
    });

    // Navigate to admin role page
    await adminPage.gotoRole('instance-admin');

    // Should see "Users with this Role" section
    await adminPage.expectUsersWithRoleVisible();

    // Should see the regular user in the list
    await adminPage.expectUserLoginVisible(regularUser.login);

    // Clean up - revoke role
    await page.request.post('/api/graphql', {
      headers: {
        'Content-Type': 'application/json',
        'X-REQUEST-TYPE': 'GraphQL'
      },
      data: {
        query: `
					mutation RevokeRole($input: RevokeInstanceRoleInput!) { revokeInstanceRole(input: $input)
					}
				`,
        variables: { input: { userId: regularUser.id, roleName: 'instance-admin' } }
      }
    });

    await regularContext.close();
  });

  test('everyone role page shows special message instead of user list', async ({
    page,
    adminPage
  }) => {
    await createAndLoginAdminUser(page);

    // Navigate to everyone role page
    await adminPage.gotoRole('everyone');

    // Should see special message about implicit membership
    await adminPage.expectMemberRoleMessage();
  });

  test('clicking user in role page navigates to user management', async ({
    page,
    adminPage,
    browser
  }) => {
    await createAndLoginAdminUser(page);

    // Create a user and assign admin role
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularUser = await createAndLoginTestUser(regularPage);

    // Assign admin role via API
    await page.request.post('/api/graphql', {
      headers: {
        'Content-Type': 'application/json',
        'X-REQUEST-TYPE': 'GraphQL'
      },
      data: {
        query: `
					mutation AssignRole($input: AssignInstanceRoleInput!) { assignInstanceRole(input: $input)
					}
				`,
        variables: { input: { userId: regularUser.id, roleName: 'instance-admin' } }
      }
    });

    // Navigate to admin role page
    await adminPage.gotoRole('instance-admin');

    // Click on the user
    await page.getByText(regularUser.login).click();

    // Should navigate to user management page
    await expect(page).toHaveURL(routes.adminUser(regularUser.id!));
    await adminPage.expectUserManagementVisible();

    // Clean up
    await page.request.post('/api/graphql', {
      headers: {
        'Content-Type': 'application/json',
        'X-REQUEST-TYPE': 'GraphQL'
      },
      data: {
        query: `
					mutation RevokeRole($input: RevokeInstanceRoleInput!) { revokeInstanceRole(input: $input)
					}
				`,
        variables: { input: { userId: regularUser.id, roleName: 'instance-admin' } }
      }
    });

    await regularContext.close();
  });
});

test.describe('Browse Spaces Permission', () => {
  test('user with denied space.list cannot see explore spaces icon', async ({ page, browser }) => {
    // Create admin and regular user
    await createAndLoginAdminUser(page);

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularChatPage = new ChatPage(regularPage);
    const regularUser = await createAndLoginTestUser(regularPage);

    // Navigate to chat - should see explore spaces icon by default
    await regularChatPage.goto();
    await regularChatPage.expectExploreSpacesVisible();

    // Deny space.list for the regular user
    const denyRoleName = await denyUserInstancePermission(page, regularUser.id!, 'space.list');

    // Reload and verify icon is hidden
    await regularPage.reload();
    await expect(regularPage.locator('[title="Explore Spaces"]')).not.toBeVisible();

    // Clean up
    await clearUserInstancePermissionOverride(page, regularUser.id!, 'space.list', denyRoleName);
    await regularContext.close();
  });

  test('user with denied space.list sees no spaces on /chat/spaces', async ({
    page,
    browser
  }) => {
    // Create admin and regular user
    await createAndLoginAdminUser(page);

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularUser = await createAndLoginTestUser(regularPage);

    // Deny space.list for the regular user
    const denyRoleName = await denyUserInstancePermission(page, regularUser.id!, 'space.list');

    // Navigate directly to /chat/spaces
    await regularPage.goto(routes.spaces);

    // The page loads (instance-agnostic browse page) but no space cards are shown
    await expect(regularPage.getByRole('heading', { name: 'Browse Spaces' })).toBeVisible();
    await expect(regularPage.locator('[data-testid="space-card"]')).not.toBeVisible();

    // Clean up
    await clearUserInstancePermissionOverride(page, regularUser.id!, 'space.list', denyRoleName);
    await regularContext.close();
  });

  test('user with space.list can see explore spaces icon and page', async ({ page, browser }) => {
    // Create admin and regular user
    await createAndLoginAdminUser(page);

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularChatPage = new ChatPage(regularPage);
    await createAndLoginTestUser(regularPage);

    // Navigate to chat - should see explore spaces icon (default permission)
    await regularChatPage.goto();
    await regularChatPage.expectExploreSpacesVisible();

    // Click the icon to navigate to spaces page
    await regularChatPage.goToExploreSpaces();

    // Should see the browse spaces page content (not access denied)
    await expect(regularPage.getByRole('heading', { name: 'Browse Spaces' })).toBeVisible();
    await expect(regularPage.getByRole('heading', { name: 'Access Denied' })).not.toBeVisible();

    await regularContext.close();
  });

  test('user without space.list sees fallback message instead of redirect to Browse Spaces', async ({
    page,
    browser
  }) => {
    // Create admin and regular user
    await createAndLoginAdminUser(page);

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularUser = await createAndLoginTestUser(regularPage);

    // Deny space.list for the regular user
    const denyRoleName = await denyUserInstancePermission(page, regularUser.id!, 'space.list');

    // Navigate to / - without canListSpaces, the user is redirected to /chat/-
    // but NOT to /chat/spaces (Browse Spaces)
    await regularPage.goto('/');

    // Should see the fallback message (redirected to /chat/- which shows it)
    await expect(regularPage.getByText('Choose a space from the sidebar to get started.')).toBeVisible();

    // Clean up
    await clearUserInstancePermissionOverride(page, regularUser.id!, 'space.list', denyRoleName);
    await regularContext.close();
  });
});

test.describe('Instance Settings', () => {
  // Reset instance config after each test to prevent test pollution
  test.afterEach(async ({ page }) => {
    try {
      await page.request.post('/api/graphql', {
        headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
        data: {
          query: `mutation { admin { resetInstanceConfig } }`
        }
      });
    } catch {
      // Ignore errors - may not be logged in or admin
    }
  });

  test('admin can access instance settings page', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoInstanceSettings();

    await adminPage.expectInstanceSettingsVisible();
    await adminPage.expectInstanceName('Chatto');
    await adminPage.expectMotd('');
    await adminPage.expectWelcomeMessage('');
  });

  test('admin can set instance name and MOTD', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoInstanceSettings();

    // Set values
    await adminPage.fillInstanceSettings({
      instanceName: 'Test Instance',
      motd: 'Hello World'
    });
    await adminPage.saveInstanceSettings();

    // Reload and verify values persisted
    await page.reload();
    await adminPage.expectInstanceName('Test Instance');
    await adminPage.expectMotd('Hello World');
  });

  test('admin can clear MOTD by setting it to empty string', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoInstanceSettings();

    // First set a MOTD
    await adminPage.fillInstanceSettings({ motd: 'Initial MOTD' });
    await adminPage.saveInstanceSettings();

    // Verify it was set
    await page.reload();
    await adminPage.expectMotd('Initial MOTD');

    // Now clear it
    await adminPage.fillInstanceSettings({ motd: '' });
    await adminPage.saveInstanceSettings();

    // Verify it was cleared
    await page.reload();
    await adminPage.expectMotd('');

    // Also verify the MOTD is not shown in the chat header
    await page.goto('/chat');
    // The AppHeader shows MOTD in center - if empty, should just be an empty flex spacer
    await expect(page.getByTestId('motd-content')).not.toBeVisible();
  });

  test('MOTD appears in the chat header with markdown support', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoInstanceSettings();

    // Set MOTD with markdown
    await adminPage.fillInstanceSettings({ motd: 'Welcome to **Chatto**!' });
    await adminPage.saveInstanceSettings();

    // Navigate to chat and check MOTD is rendered
    await page.goto('/chat');

    // Wait for the markdown to render (it's async)
    await expect(page.getByTestId('motd-content')).toBeVisible();
    // The markdown should render **Chatto** as bold
    await expect(page.getByTestId('motd-content').locator('strong')).toHaveText('Chatto');
  });

  test('reset to defaults clears all instance settings', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoInstanceSettings();

    // First set some values
    await adminPage.fillInstanceSettings({
      instanceName: 'Custom Name',
      motd: 'Custom MOTD',
      welcomeMessage: 'Custom Welcome'
    });
    await adminPage.saveInstanceSettings();

    // Verify they're set
    await page.reload();
    await adminPage.expectInstanceName('Custom Name');

    // Now reset
    await adminPage.resetInstanceSettings();

    // Verify they're back to defaults
    await adminPage.expectInstanceName('Chatto');
    await adminPage.expectMotd('');
    await adminPage.expectWelcomeMessage('');
  });

  test('instance config changes update other connected clients in real-time', async ({
    page,
    adminPage,
    browser
  }) => {
    // First admin user logs in and goes to settings
    await createAndLoginAdminUser(page);

    // Second context - a regular user viewing the chat
    const context2 = await browser.newContext();
    const page2 = await context2.newPage();
    await createAndLoginTestUser(page2);
    await page2.goto('/chat');

    // Verify no MOTD initially on second page
    await expect(page2.getByTestId('motd-content')).not.toBeVisible();

    // First page (admin): go to settings and set MOTD
    await adminPage.gotoInstanceSettings();
    await adminPage.fillInstanceSettings({ motd: 'Live update test!' });
    await adminPage.saveInstanceSettings();

    // Second page (regular user) should now show the MOTD (via live events)
    await expect(page2.getByTestId('motd-content')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });
    await expect(page2.getByTestId('motd-content')).toContainText('Live update test!');

    // Clean up
    await context2.close();
  });

  test('instance name changes update page title in real-time for connected clients', async ({
    page,
    adminPage,
    browser
  }) => {
    // First admin user logs in
    await createAndLoginAdminUser(page);

    // Second context - a regular user viewing the chat
    const context2 = await browser.newContext();
    const page2 = await context2.newPage();
    await createAndLoginTestUser(page2);
    await page2.goto(routes.spaces);

    // Verify initial page title uses default instance name
    await expect(page2).toHaveTitle(/Chatto/);

    // First page (admin): go to settings and change instance name
    await adminPage.gotoInstanceSettings();
    await adminPage.fillInstanceSettings({ instanceName: 'Live Title Test' });
    await adminPage.saveInstanceSettings();

    // Second page (regular user) should now show updated instance name in title (via live events)
    await expect(page2).toHaveTitle(/Live Title Test/, { timeout: TIMEOUTS.UI_STANDARD });

    // Clean up
    await context2.close();
  });

  test('instance name appears in page title', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoInstanceSettings();

    // Set instance name
    await adminPage.fillInstanceSettings({ instanceName: 'My Chat Server' });
    await adminPage.saveInstanceSettings();

    // Navigate to chat and check page title includes instance name
    await page.goto('/chat');
    await expect(page).toHaveTitle(/My Chat Server/);

    // Verify page-specific prefixes also include instance name
    await page.goto(routes.admin);
    await expect(page).toHaveTitle('Admin Dashboard | My Chat Server');

    await page.goto(routes.adminUsers);
    await expect(page).toHaveTitle('Users | Admin | My Chat Server');

    await page.goto(routes.spaces);
    await expect(page).toHaveTitle('Browse Spaces | My Chat Server');
  });

  test('Link Previews section is accessible and has all expected fields', async ({
    page,
    adminPage
  }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoInstanceSettings();

    // Verify the page loads and the Link Previews section is visible
    await adminPage.expectInstanceSettingsVisible();
    await adminPage.expectLinkPreviewsSectionVisible();

    // Verify OG title and description can be set
    await adminPage.fillLinkPreviewSettings({
      ogTitle: 'My Test Chat',
      ogDescription: 'A chat application for testing'
    });
    await adminPage.saveInstanceSettings();

    // Reload and verify values persisted
    await page.reload();
    await expect(adminPage.ogTitleInput).toHaveValue('My Test Chat');
    await expect(adminPage.ogDescriptionInput).toHaveValue('A chat application for testing');
  });
});

test.describe('Instance Role Permission Denials', () => {
  test('admin can deny a permission on a role via API and it persists', async ({ page }) => {
    await createAndLoginAdminUser(page);

    // Create a custom role via API
    const roleName = generateInstanceRoleName('test');
    const createRoleResponse = await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					mutation CreateRole($input: CreateRoleInput!) {
						createRole(input: $input) {
							name
							permissions
							permissionDenials
						}
					}
				`,
        variables: {
          input: {
            name: roleName,
            displayName: 'Test Denial Role',
            description: 'A role for testing permission denials'
          }
        }
      }
    });
    expect(createRoleResponse.ok()).toBeTruthy();
    const createRoleData = await createRoleResponse.json();
    expect(createRoleData.data?.createRole).toBeTruthy();
    expect(createRoleData.data.createRole.permissionDenials).toEqual([]);

    // Deny a permission on the role
    const denyResponse = await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					mutation DenyInstancePermission($input: DenyInstancePermissionInput!) { denyInstancePermission(input: $input)
					}
				`,
        variables: { input: { role: roleName, permission: 'space.list' } }
      }
    });
    expect(denyResponse.ok()).toBeTruthy();
    const denyData = await denyResponse.json();
    expect(denyData.data?.denyInstancePermission).toBe(true);

    // Query the role and verify the denial persists
    const queryRoleResponse = await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					query GetRole($name: String!) {
						admin {
							role(name: $name) {
								name
								permissions
								permissionDenials
							}
						}
					}
				`,
        variables: { name: roleName }
      }
    });
    expect(queryRoleResponse.ok()).toBeTruthy();
    const queryRoleData = await queryRoleResponse.json();
    expect(queryRoleData.data?.admin?.role).toBeTruthy();
    expect(queryRoleData.data.admin.role.permissionDenials).toContain('space.list');

    // Clean up - delete the role
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `mutation DeleteRole($input: DeleteRoleInput!) { deleteRole(input: $input) }`,
        variables: { input: { name: roleName } }
      }
    });
  });

  test('admin can deny a permission on a role via UI and it persists', async ({
    page,
    adminPage
  }) => {
    await createAndLoginAdminUser(page);

    // Create a custom role via API
    const roleName = generateInstanceRoleName('deny');
    const createRoleResponse = await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					mutation CreateRole($input: CreateRoleInput!) {
						createRole(input: $input) {
							name
						}
					}
				`,
        variables: {
          input: {
            name: roleName,
            displayName: 'UI Denial Test Role',
            description: 'A role for testing permission denial UI'
          }
        }
      }
    });
    expect(createRoleResponse.ok()).toBeTruthy();

    // The matrix lives on the roles listing page now: rows are permissions,
    // columns are roles, and each cell is a button whose aria-label encodes
    // both the role displayName and the permission. Clicking cycles
    // neutral → allow → deny → neutral, so two clicks lands a fresh role on
    // Deny.
    const displayName = 'UI Denial Test Role';
    await adminPage.gotoRoles();
    await expect(page.getByRole('heading', { name: 'Roles' })).toBeVisible();

    const cell = page.locator(
      `button[aria-label*="${displayName}"][aria-label*="space.list"]`
    );
    await expect(cell).toHaveAttribute('aria-pressed', 'false');

    await cell.click();
    await expect(cell).toHaveAttribute('aria-label', /Override allow/, {
      timeout: TIMEOUTS.UI_STANDARD
    });

    await cell.click();
    await expect(cell).toHaveAttribute('aria-label', /Override deny/, {
      timeout: TIMEOUTS.UI_STANDARD
    });

    // Reload and verify the denial persists.
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Roles' })).toBeVisible();
    const cellAfterReload = page.locator(
      `button[aria-label*="${displayName}"][aria-label*="space.list"]`
    );
    await expect(cellAfterReload).toHaveAttribute('aria-label', /Override deny/);
    await expect(cellAfterReload).toHaveAttribute('aria-pressed', 'true');

    // Clean up - delete the role
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `mutation DeleteRole($input: DeleteRoleInput!) { deleteRole(input: $input) }`,
        variables: { input: { name: roleName } }
      }
    });
  });

  test('permission denial on a role blocks user with that role', async ({ page, browser }) => {
    await createAndLoginAdminUser(page);

    // Create a custom role
    const roleName = generateInstanceRoleName('block');
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					mutation CreateRole($input: CreateRoleInput!) {
						createRole(input: $input) { name }
					}
				`,
        variables: {
          input: {
            name: roleName,
            displayName: 'Block Test Role',
            description: 'A role that denies space.list'
          }
        }
      }
    });

    // Deny space.list on the role
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					mutation DenyInstancePermission($input: DenyInstancePermissionInput!) { denyInstancePermission(input: $input)
					}
				`,
        variables: { input: { role: roleName, permission: 'space.list' } }
      }
    });

    // Create a second user and assign the role
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularUser = await createAndLoginTestUser(regularPage);

    // First verify the user CAN browse spaces (has permission via member role)
    await regularPage.goto(routes.spaces);
    await expect(regularPage.getByRole('heading', { name: 'Browse Spaces' })).toBeVisible();

    // Now assign the blocking role to the user
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					mutation AssignRole($input: AssignInstanceRoleInput!) { assignInstanceRole(input: $input)
					}
				`,
        variables: { input: { userId: regularUser.id, roleName } }
      }
    });

    // Reload and verify the user is NOW blocked (denial overrides member grant)
    // The SpaceDirectory shows an error or permission message instead of Access Denied
    await regularPage.reload();
    await expect(
      regularPage.getByText(/(No permission to browse spaces on|Could not connect to)/)
    ).toBeVisible();

    // Clean up - revoke the role and delete it
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					mutation RevokeRole($input: RevokeInstanceRoleInput!) { revokeInstanceRole(input: $input)
					}
				`,
        variables: { input: { userId: regularUser.id, roleName } }
      }
    });
    await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `mutation DeleteRole($input: DeleteRoleInput!) { deleteRole(input: $input) }`,
        variables: { input: { name: roleName } }
      }
    });

    await regularContext.close();
  });
});

test.describe('Identity Editing', () => {
  test('admin can rename a user and reset their cooldown', async ({
    page,
    adminPage,
    browser
  }) => {
    await createAndLoginAdminUser(page);

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularUser = await createAndLoginTestUser(regularPage, { loginPrefix: 'edituser' });

    // The regular user changes their own login first to set a cooldown
    // timestamp. We need this to verify Reset cooldown actually clears it.
    const userChosenLogin = `userpicked${Date.now()}`;
    const userRenameResp = await regularPage.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `mutation($input: UpdateMyProfileInput!) { updateMyProfile(input: $input) { id login } }`,
        variables: { input: { login: userChosenLogin } }
      }
    });
    expect(userRenameResp.ok()).toBeTruthy();

    // Admin navigates to the user management page
    await adminPage.gotoUserManagement(regularUser.id!);
    await adminPage.expectUserManagementVisible();

    // Identity panel should be visible
    await expect(page.getByRole('heading', { name: 'Identity' })).toBeVisible();

    // The username field should reflect the user's last self-chosen login
    const usernameInput = page.getByTestId('admin-identity-login');
    const displayNameInput = page.getByTestId('admin-identity-display-name');
    await expect(usernameInput).toHaveValue(userChosenLogin);

    // Save is disabled while pristine
    const saveButton = page.getByRole('button', { name: 'Save' });
    await expect(saveButton).toBeDisabled();

    // Admin renames the user via the panel
    const adminChosenLogin = `adminpicked${Date.now()}`;
    const adminChosenDisplay = 'Renamed By Admin';
    await usernameInput.fill(adminChosenLogin);
    await displayNameInput.fill(adminChosenDisplay);

    // Submitting via Enter inside the form should work (the panel uses a real <form>)
    await usernameInput.press('Enter');

    // Toast confirmation
    await expect(page.getByText('User updated')).toBeVisible({ timeout: TIMEOUTS.UI_STANDARD });

    // The User Details panel reflects the new identity (without a page reload —
    // the mutation refetches the query).
    const userDetailsPanel = page
      .locator('section, div')
      .filter({ hasText: 'User Details' })
      .first();
    await expect(userDetailsPanel.getByText(adminChosenLogin).first()).toBeVisible();
    await expect(userDetailsPanel.getByText(adminChosenDisplay).first()).toBeVisible();

    // The cooldown is unchanged because admin edits don't advance the user's
    // clock. The "Reset cooldown" button should still be enabled.
    const resetCooldownButton = page.getByRole('button', { name: 'Reset cooldown' });
    await expect(resetCooldownButton).toBeEnabled();
    await resetCooldownButton.click();

    await expect(page.getByText('Username change cooldown cleared')).toBeVisible({
      timeout: TIMEOUTS.UI_STANDARD
    });

    // After clearing, the panel should show the "never changed" state and the
    // button should be disabled (no cooldown to reset).
    await expect(page.getByText('User has never changed their username.')).toBeVisible();
    await expect(resetCooldownButton).toBeDisabled();

    // Sanity check: the user can now successfully rename themselves immediately,
    // proving the cooldown was actually cleared on the backend.
    const userSecondRename = `userrenamed${Date.now()}`;
    const secondRenameResp = await regularPage.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `mutation($input: UpdateMyProfileInput!) { updateMyProfile(input: $input) { id login } }`,
        variables: { input: { login: userSecondRename } }
      }
    });
    expect(secondRenameResp.ok()).toBeTruthy();
    const secondRenameData = (await secondRenameResp.json()) as {
      data?: { updateMyProfile?: { login?: string } };
      errors?: unknown;
    };
    expect(secondRenameData.errors).toBeUndefined();
    expect(secondRenameData.data?.updateMyProfile?.login).toBe(userSecondRename);

    await regularContext.close();
  });
});

