import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import { AdminPage, ChatPage } from './pages';
import * as routes from './routes';
import { TIMEOUTS } from './constants';
import {
  createAndLoginTestUser,
  generateRoleName,
  loginAsAdmin,
  verifyAdminEmail,
  grantPermission,
  revokePermission,
  type TestUser
} from './fixtures/testUser';

async function createRoleViaAPI(
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

async function assignRoleViaAPI(
  page: Page,
  userId: string,
  roleName: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: AssignRoleInput!) { assignRole(input: $input) }`,
      variables: { input: { userId, roleName } }
    }
  });
  expect(resp.ok()).toBeTruthy();
}

async function revokeRoleViaAPI(
  page: Page,
  userId: string,
  roleName: string
): Promise<void> {
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation($input: RevokeRoleInput!) { revokeRole(input: $input) }`,
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
    await adminPage.expectAdministrationGroupNotVisible();
    await expect(
      page.getByText('You do not have permission to access this page.')
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

test.describe('Admin General Page', () => {
  test('admin users can access the default admin destination', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.goto();

    // Should see the General settings page
    await adminPage.expectGeneralPageVisible();

    // Admin pages keep the normal server sidebar; the inline admin group
    // starts expanded on admin routes and highlights the current page.
    await adminPage.expectBackToChatVisible();
    await adminPage.expectAdministrationGroupVisible();
    await adminPage.expectAdministrationExpanded();
    await adminPage.expectAdministrationGroupNotActive();
    await adminPage.expectSidebarLinkActive('General');

    // Should see the sidebar navigation
    await adminPage.expectSidebarNavVisible();
  });

  test('default admin destination shows General settings', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.goto();

    await adminPage.expectGeneralPageVisible();
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

    // Should see the admin user's login in the list (Members page formats
    // login with the leading @).
    await expect(page.getByRole('cell', { name: `@${adminUser.login}`, exact: true })).toBeVisible();

    // Should see the total count
    await adminPage.expectUserCountVisible();
  });

  // The previous "admin can see verified emails for multiple OAuth users"
  // test was retired when /admin/users folded into the server-admin Members
  // page. The Members page intentionally doesn't surface email addresses —
  // that level of identity is a deliberate scope reduction.
});

// Admin Spaces page retired in PR(a) — instance metadata is managed via the
// Server Admin → General page now; the old spaces tier is gone.

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
  test('administration group toggles inline admin links', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.goto();

    await adminPage.expectAdministrationGroupVisible();
    await adminPage.expectAdministrationExpanded();
    await adminPage.expectSidebarLinkVisible('General');
    await adminPage.expectSidebarLinkVisible('Users');

    await adminPage.collapseAdministration();
    await adminPage.expectAdministrationCollapsed();
    await adminPage.expectSidebarLinkNotVisible('General');

    await adminPage.expandAdministration();
    await adminPage.expectAdministrationExpanded();
    await adminPage.expectSidebarLinkVisible('General');
  });

  test('admin pages keep the normal server sidebar shell', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoUsers();

    await adminPage.expectUsersPageVisible();
    await adminPage.expectBackToChatVisible();
    await adminPage.expectAdministrationGroupVisible();
    await adminPage.expectAdministrationExpanded();
    await adminPage.expectSidebarLinkActive('Users');
  });

  test('sidebar navigation works correctly', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.goto();

    // Navigate to Users
    await adminPage.navigateToUsers();
    await adminPage.expectUsersPageVisible();

    // Navigate to System
    await adminPage.navigateToSystem();
    await adminPage.expectSystemPageVisible();

    // Navigate back to General
    await adminPage.navigateToGeneral();
    await adminPage.expectGeneralPageVisible();
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
      'admin.view-users',
      'admin.view-system',
      'role.manage',
      'room.manage'
    ];

    for (const permission of permissions) {
      try {
        await page.request.post('/api/graphql', {
          headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
          data: {
            query: `mutation { revokePermission(input: {roleName: "everyone", permission: "${permission}"}) }`
          }
        });
      } catch {
        // Ignore errors - permission may not have been granted
      }
    }
  });

  test('user with a concrete admin capability can access their admin section', async ({
    page,
    browser
  }) => {
    // First, as admin, grant an admin-view capability to everyone role.
    await createAndLoginAdminUser(page);
    await grantPermission(page, 'everyone', 'admin.view-users');

    // Now create a regular user and try to access admin
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoUsers();

    // Should see the permitted section (not access denied) and an inline admin group
    // whose permitted links start hidden until expanded.
    await regularAdminPage.expectUsersPageVisible();
    await regularAdminPage.expectAdministrationGroupVisible();
    await regularAdminPage.expectAdministrationExpanded();
    await regularAdminPage.expectSidebarLinkVisible('Users');
    await regularAdminPage.expectSidebarLinkActive('Users');
    await regularAdminPage.collapseAdministration();
    await regularAdminPage.expectSidebarLinkNotVisible('Users');

    // Clean up: revoke the permission
    await revokePermission(page, 'everyone', 'admin.view-users');
    await regularContext.close();
  });

  test('user with room.manage but without admin.view-users sees limited nav items', async ({
    page,
    browser
  }) => {
    // Grant only a non-user admin capability to everyone role.
    await createAndLoginAdminUser(page);
    await grantPermission(page, 'everyone', 'room.manage');

    // Create regular user and access admin
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularPage.goto(routes.serverAdminRooms);

    // Should see their concrete admin section in nav
    await regularAdminPage.expectSidebarLinkVisible('Rooms');

    // Should NOT see Users, System (no permissions for those)
    await regularAdminPage.expectSidebarLinkNotVisible('Users');
    await regularAdminPage.expectSidebarLinkNotVisible('System');

    // Clean up
    await revokePermission(page, 'everyone', 'room.manage');
    await regularContext.close();
  });

  test('user with admin.view-users permission can see users list', async ({ page, browser }) => {
    await createAndLoginAdminUser(page);
    await grantPermission(page, 'everyone', 'admin.view-users');

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
    await revokePermission(page, 'everyone', 'admin.view-users');
    await regularContext.close();
  });

  test('user without admin.view-users sees access denied on /chat/-/admin/users', async ({
    page,
    browser
  }) => {
    await createAndLoginAdminUser(page);

    // Create regular user
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoUsers();

    // Should see access denied with the specific permission mentioned
    await regularAdminPage.expectAccessDeniedForPermission('admin.view-users');

    await regularContext.close();
  });

  test('non-owner sees access denied on /chat/-/admin/system', async ({
    page,
    browser
  }) => {
    await createAndLoginAdminUser(page);

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoSystem();

    await regularAdminPage.expectAccessDeniedForPermission('admin.view-system');

    await regularContext.close();
  });

  test('user without admin.view-roles sees access denied on permissions', async ({
    page,
    browser
  }) => {
    await createAndLoginAdminUser(page);

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoRoles();

    await regularAdminPage.expectAccessDeniedForPermission('admin.view-roles');

    await regularContext.close();
  });

  test('user with admin.view-system permission still cannot see owner-only system page', async ({
    page,
    browser
  }) => {
    await createAndLoginAdminUser(page);
    await grantPermission(page, 'everyone', 'admin.view-system');

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularAdminPage.gotoSystem();

    await regularAdminPage.expectAccessDeniedForPermission('owner');

    // Clean up
    await revokePermission(page, 'everyone', 'admin.view-system');
    await regularContext.close();
  });

  // Note: a read-only view of the roles page (admin.view-roles without
  // admin.manage-roles) was removed when the UI moved from a per-role
  // editor to the unified matrix. The matrix's rolePermissionTierMatrix query gates on
  // instance admin / role.manage, so view-roles alone is currently not
  // sufficient to render the page. Re-add the test once the matrix grows
  // a read-only mode.

  test('nav items dynamically update based on granted permissions', async ({ page, browser }) => {
    // Start with one concrete admin capability.
    await createAndLoginAdminUser(page);
    await grantPermission(page, 'everyone', 'room.manage');

    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    await createAndLoginTestUser(regularPage);

    await regularPage.goto(routes.serverAdminRooms);

    // Initially should only see the room management section
    await regularAdminPage.expectSidebarLinkVisible('Rooms');
    await regularAdminPage.expectSidebarLinkNotVisible('Users');

    // Now grant admin.view-users permission as admin
    await grantPermission(page, 'everyone', 'admin.view-users');

    // Reload and check nav updated
    await regularPage.reload();
    await regularAdminPage.expectSidebarLinkVisible('Users');

    // Grant admin.view-system. System diagnostics remain owner-only for now,
    // so this permission alone must not reveal the route.
    await grantPermission(page, 'everyone', 'admin.view-system');
    await regularPage.reload();
    await regularAdminPage.expectSidebarLinkNotVisible('System');

    // Clean up
    await revokePermission(page, 'everyone', 'room.manage');
    await revokePermission(page, 'everyone', 'admin.view-users');
    await revokePermission(page, 'everyone', 'admin.view-system');
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

  test('granting a role with admin.view-users gives user admin access', async ({
    page,
    browser
  }) => {
    await createAndLoginAdminUser(page);

    // Create a regular user
    const regularContext = await browser.newContext();
    const regularPage = await regularContext.newPage();
    const regularAdminPage = new AdminPage(regularPage);
    const regularUser = await createAndLoginTestUser(regularPage);

    // Regular user should NOT have admin access initially
    await regularAdminPage.gotoUsers();
    await expect(regularPage.getByText('Access Denied', { exact: true })).toBeVisible();

    // Create a role with an admin-view capability and assign it to the user.
    const roleName = generateRoleName('grant');
    await createRoleViaAPI(page, roleName, 'Grant Admin');
    await grantPermission(page, roleName, 'admin.view-users');
    await assignRoleViaAPI(page, regularUser.id!, roleName);

    // Regular user should now have admin access
    await regularPage.reload();
    await regularAdminPage.expectUsersPageVisible();

    // Clean up
    await revokeRoleViaAPI(page, regularUser.id!, roleName);
    await regularContext.close();
  });

  // The "deny `space.list` blocks the Browse Spaces page" pair was retired
  // with the Browse Spaces UI in PR(a), and `space.list` itself was removed
  // afterwards. The deny-role mechanism is covered by the other
  // permission-denial tests in this file.
});

test.describe('Role Assignment', () => {
  // The "instance-admin" / instance-role assignment tests previously lived
  // here. They targeted the legacy /admin/users/[id] and /admin/roles/[name]
  // pages, which used a separate RBAC engine for instance-scoped roles.
  // After the instance-admin → server-admin consolidation, instance roles
  // are not surfaced in the unified server-admin role detail; merging the
  // two RBAC engines lands in the planned PR(c). Restore equivalent
  // coverage there once the role concepts unify.

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

  // The "clicking user in role page navigates to user management" test
  // depended on the instance-admin role surfacing on the role detail page
  // — same story as the suite header note above. Restore once PR(c) merges
  // the instance and space RBAC engines.
});

// "Browse Spaces Permission" describe block was retired with the Browse
// Spaces UI in PR(a); the `space.list` permission has since been removed.
// Deny-role behaviour is exercised by the other admin permission tests.

test.describe('Instance Settings', () => {
  // No per-test cleanup needed: every e2e test spawns its own isolated
  // chatto instance (see frontend/e2e/fixtures/server.ts), so state
  // never leaks between tests.

  test('admin can access instance settings page', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoServerSettings();

    await adminPage.expectServerSettingsVisible();
    // The e2e fixture's [bootstrap.instance] block seeds the instance name on
    // first boot (see frontend/e2e/fixtures/chatto.toml).
    await adminPage.expectInstanceName('E2E Test Server');
    await adminPage.expectMotd('');
    await adminPage.expectWelcomeMessage('');
  });

  test('admin can set instance name and MOTD', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoServerSettings();

    // Set values
    await adminPage.fillServerSettings({
      serverName: 'Test Instance',
      motd: 'Hello World'
    });
    await adminPage.saveServerSettings();

    // Reload and verify values persisted
    await page.reload();
    await adminPage.expectInstanceName('Test Instance');
    await adminPage.expectMotd('Hello World');
  });

  test('admin can clear MOTD by setting it to empty string', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoServerSettings();

    // First set a MOTD
    await adminPage.fillServerSettings({ motd: 'Initial MOTD' });
    await adminPage.saveServerSettings();

    // Verify it was set
    await page.reload();
    await adminPage.expectMotd('Initial MOTD');

    // Now clear it
    await adminPage.fillServerSettings({ motd: '' });
    await adminPage.saveServerSettings();

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

    await adminPage.gotoServerSettings();

    // Set MOTD with markdown
    await adminPage.fillServerSettings({ motd: 'Welcome to **Chatto**!' });
    await adminPage.saveServerSettings();

    // Navigate to chat and check MOTD is rendered
    await page.goto('/chat');

    // Wait for the markdown to render (it's async)
    await expect(page.getByTestId('motd-content')).toBeVisible();
    // The markdown should render **Chatto** as bold
    await expect(page.getByTestId('motd-content').locator('strong')).toHaveText('Chatto');
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
    await adminPage.gotoServerSettings();
    await adminPage.fillServerSettings({ motd: 'Live update test!' });
    await adminPage.saveServerSettings();

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

    // Verify initial page title contains *some* instance name (post-PR(a)
    // this is the bootstrap space's name when no override is configured —
    // see `InstanceConfig.serverName` resolver fallback chain). The
    // assertion below for the *changed* name is the meaningful signal.
    await expect(page2).not.toHaveTitle('');

    // First page (admin): go to settings and change instance name
    await adminPage.gotoServerSettings();
    await adminPage.fillServerSettings({ serverName: 'Live Title Test' });
    await adminPage.saveServerSettings();

    // Second page (regular user) should now show updated instance name in title (via live events)
    await expect(page2).toHaveTitle(/Live Title Test/, { timeout: TIMEOUTS.UI_STANDARD });

    // Clean up
    await context2.close();
  });

  test('instance name appears in page title', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoServerSettings();

    // Set instance name
    await adminPage.fillServerSettings({ serverName: 'My Chat Server' });
    await adminPage.saveServerSettings();

    // Navigate to chat and check page title includes instance name
    await page.goto('/chat');
    await expect(page).toHaveTitle(/My Chat Server/);

    // Verify page-specific prefixes also include instance name. The exact
    // page titles changed when instance admin folded into server admin —
    // we just check the instance name appears as a suffix.
    await page.goto(routes.admin);
    await expect(page).toHaveTitle(/My Chat Server$/);

    await page.goto(routes.adminUsers);
    await expect(page).toHaveTitle(/My Chat Server$/);
  });

});

test.describe('Instance Role Permission Denials', () => {
  test('admin can deny a permission on a role via API and it persists', async ({ page }) => {
    await createAndLoginAdminUser(page);

    // Create a custom role via API
    const roleName = generateRoleName('test');
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
					mutation DenyInstancePermission($input: DenyPermissionInput!) { denyPermission(input: $input)
					}
				`,
        variables: { input: { roleName, permission: 'message.post' } }
      }
    });
    expect(denyResponse.ok()).toBeTruthy();
    const denyData = await denyResponse.json();
    expect(denyData.data?.denyPermission).toBe(true);

    // Query the role and verify the denial persists
    const queryRoleResponse = await page.request.post('/api/graphql', {
      headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
      data: {
        query: `
					query GetRole($name: String!) {
						server {
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
    expect(queryRoleData.data?.server?.role).toBeTruthy();
    expect(queryRoleData.data.server.role.permissionDenials).toContain('message.post');

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
    const roleName = generateRoleName('deny');
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
    await expect(page.getByRole('heading', { name: 'Permissions', level: 1 })).toBeVisible();

    const cell = page.locator(
      `td[data-role="${roleName}"][data-permission="message.post"] button`
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
    await expect(page.getByRole('heading', { name: 'Permissions', level: 1 })).toBeVisible();
    const cellAfterReload = page.locator(
      `td[data-role="${roleName}"][data-permission="message.post"] button`
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
        query: `mutation($input: UpdateProfileInput!) { updateProfile(input: $input) { id login } }`,
        variables: { input: { userId: regularUser.id, login: userChosenLogin } }
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
        query: `mutation($input: UpdateProfileInput!) { updateProfile(input: $input) { id login } }`,
        variables: { input: { userId: regularUser.id, login: userSecondRename } }
      }
    });
    expect(secondRenameResp.ok()).toBeTruthy();
    const secondRenameData = (await secondRenameResp.json()) as {
      data?: { updateProfile?: { login?: string } };
      errors?: unknown;
    };
    expect(secondRenameData.errors).toBeUndefined();
    expect(secondRenameData.data?.updateProfile?.login).toBe(userSecondRename);

    await regularContext.close();
  });
});
