import { expect, type Browser, type Page } from '@playwright/test';
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
import {
  withServerUser,
  type ServerUserOptions,
  type ServerUserSession
} from './fixtures/serverUser';
import { connectPost, type E2EAdminRole, unwrapAdminRole } from './fixtures/connectHelpers';

type RegularAdminSession = ServerUserSession & { adminPage: AdminPage };

async function withRegularAdminPage<T>(
  browser: Browser,
  serverURL: string,
  run: (session: RegularAdminSession) => Promise<T>,
  options?: ServerUserOptions
): Promise<T> {
  return withServerUser(
    browser,
    serverURL,
    async (session) => run({ ...session, adminPage: new AdminPage(session.page) }),
    options
  );
}

async function createRoleViaAPI(page: Page, name: string, displayName: string): Promise<void> {
  const role = await createRoleViaConnect(page, name, displayName, `Test role: ${displayName}`);
  expect(role.name).toBe(name);
}

async function assignRoleViaAPI(page: Page, userId: string, roleName: string): Promise<void> {
  const data = await connectPost<{ assigned?: boolean }>(
    page,
    'chatto.admin.v1.AdminMemberService/AssignRole',
    { userId, roleName }
  );
  expect(data.assigned).toBe(true);
}

async function revokeRoleViaAPI(page: Page, userId: string, roleName: string): Promise<void> {
  const data = await connectPost<{ revoked?: boolean }>(
    page,
    'chatto.admin.v1.AdminMemberService/RevokeRole',
    { userId, roleName }
  );
  expect(data.revoked).toBe(true);
}

async function createRoleViaConnect(
  page: Page,
  name: string,
  displayName: string,
  description: string
): Promise<{ name?: string; permissionDenials?: string[] }> {
  const data = await connectPost<{ role?: E2EAdminRole }>(
    page,
    'chatto.admin.v1.AdminRoleService/CreateRole',
    { name, displayName, description }
  );
  const role = unwrapAdminRole(data.role);
  if (!role?.name) {
    throw new Error(`CreateRole did not return a role: ${JSON.stringify(data)}`);
  }
  return role;
}

async function deleteRoleViaConnect(page: Page, name: string): Promise<void> {
  const data = await connectPost<{ deleted?: boolean }>(
    page,
    'chatto.admin.v1.AdminRoleService/DeleteRole',
    { name }
  );
  expect(data.deleted).toBe(true);
}

async function getRoleViaConnect(
  page: Page,
  name: string
): Promise<{ name?: string; permissionDenials?: string[] }> {
  const data = await connectPost<{ role?: E2EAdminRole }>(
    page,
    'chatto.admin.v1.AdminRoleService/GetRole',
    { name }
  );
  const role = unwrapAdminRole(data.role);
  if (!role?.name) {
    throw new Error(`Role ${name} not found: ${JSON.stringify(data)}`);
  }
  return role;
}

async function setRolePermissionViaConnect(
  page: Page,
  roleName: string,
  permission: string,
  decision: 'PERMISSION_DECISION_DENY' | 'PERMISSION_DECISION_NONE'
): Promise<void> {
  const data = await connectPost<{ ok?: boolean }>(
    page,
    'chatto.admin.v1.AdminPermissionService/SetRolePermission',
    {
      roleName,
      permission,
      decision,
      scope: { kind: 'PERMISSION_SCOPE_KIND_SERVER' }
    }
  );
  expect(data.ok).toBe(true);
}

async function updateOwnProfileViaConnect(
  page: Page,
  input: { login?: string; displayName?: string }
): Promise<{ login?: string; displayName?: string }> {
  const data = await connectPost<{ user?: { login?: string; displayName?: string } }>(
    page,
    'chatto.api.v1.AccountService/UpdateProfile',
    input
  );
  if (!data.user) {
    throw new Error(`UpdateProfile did not return a user: ${JSON.stringify(data)}`);
  }
  return data.user;
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
  test('non-admin users see access denied message on /chat/-/admin', async ({
    page,
    adminPage
  }) => {
    // First, ensure there's already an admin user (the first user gets auto-promoted)
    // by creating a throwaway user that claims the first-user admin spot
    await createAndLoginAdminUser(page);

    // Now create a regular (non-admin) user - they won't be the first user
    await createAndLoginTestUser(page);

    // Try to access admin page
    await adminPage.goto();

    // Should see access denied message
    await adminPage.expectAccessDenied();
    await adminPage.expectAdminGearNotVisible();
    await expect(page.getByText('You do not have permission to access this page.')).toBeVisible();

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

    // Admin pages use their own sidebar and highlight the current page.
    await adminPage.expectBackToChatVisible();
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
    await expect(
      page.getByRole('cell', { name: `@${adminUser.login}`, exact: true })
    ).toBeVisible();

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
  test('server header gear opens the first permitted admin page', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await page.goto(routes.space());

    await adminPage.expectAdminGearVisible();
    await adminPage.navigateToAdminViaGear();
    await adminPage.expectGeneralPageVisible();
    await adminPage.expectBackToChatVisible();
    await adminPage.expectSidebarLinkActive('General');
  });

  test('admin pages use the dedicated server-admin sidebar shell', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoUsers();

    await adminPage.expectUsersPageVisible();
    await adminPage.expectBackToChatVisible();
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
    const permissions = ['admin.view-users', 'admin.view-system', 'role.manage', 'room.manage'];

    for (const permission of permissions) {
      try {
        await setRolePermissionViaConnect(page, 'everyone', permission, 'PERMISSION_DECISION_NONE');
      } catch {
        // Ignore errors - permission may not have been granted
      }
    }
  });

  test('user with a concrete admin capability can access their admin section', async ({
    page,
    browser,
    serverURL
  }) => {
    // First, as admin, grant an admin-view capability to everyone role.
    await createAndLoginAdminUser(page);
    await grantPermission(page, 'everyone', 'admin.view-users');

    await withRegularAdminPage(browser, serverURL, async ({ adminPage: regularAdminPage }) => {
      await regularAdminPage.gotoUsers();

      // Should see the permitted section (not access denied) and the dedicated
      // admin sidebar should only expose their allowed links.
      await regularAdminPage.expectUsersPageVisible();
      await regularAdminPage.expectSidebarLinkVisible('Users');
      await regularAdminPage.expectSidebarLinkActive('Users');
    });

    // Clean up: revoke the permission
    await revokePermission(page, 'everyone', 'admin.view-users');
  });

  test('user with room.manage but without admin.view-users sees limited nav items', async ({
    page,
    browser,
    serverURL
  }) => {
    // Grant only a non-user admin capability to everyone role.
    await createAndLoginAdminUser(page);
    await grantPermission(page, 'everyone', 'room.manage');

    await withRegularAdminPage(browser, serverURL, async ({ page: regularPage, adminPage }) => {
      await regularPage.goto(routes.serverAdminRooms);

      // Should see their concrete admin section in nav
      await adminPage.expectSidebarLinkVisible('Rooms');

      // Should NOT see Users, System (no permissions for those)
      await adminPage.expectSidebarLinkNotVisible('Users');
      await adminPage.expectSidebarLinkNotVisible('System');
    });

    // Clean up
    await revokePermission(page, 'everyone', 'room.manage');
  });

  test('user with admin.view-users permission can see users list', async ({
    page,
    browser,
    serverURL
  }) => {
    await createAndLoginAdminUser(page);
    await grantPermission(page, 'everyone', 'admin.view-users');

    await withRegularAdminPage(browser, serverURL, async ({ adminPage: regularAdminPage }) => {
      await regularAdminPage.gotoUsers();

      // Should see the users page with data
      await regularAdminPage.expectUsersPageVisible();
      await regularAdminPage.expectUsersTableHeadersVisible();
      // Should see at least one user in the list (the user count)
      await regularAdminPage.expectUserCountVisible();
    });

    // Clean up
    await revokePermission(page, 'everyone', 'admin.view-users');
  });

  test('user without admin.view-users sees access denied on /chat/-/admin/users', async ({
    page,
    browser,
    serverURL
  }) => {
    await createAndLoginAdminUser(page);

    await withRegularAdminPage(browser, serverURL, async ({ adminPage: regularAdminPage }) => {
      await regularAdminPage.gotoUsers();

      // Should see access denied with the specific permission mentioned
      await regularAdminPage.expectAccessDeniedForPermission('admin.view-users');
    });
  });

  test('non-owner sees access denied on /chat/-/admin/system', async ({
    page,
    browser,
    serverURL
  }) => {
    await createAndLoginAdminUser(page);

    await withRegularAdminPage(browser, serverURL, async ({ adminPage: regularAdminPage }) => {
      await regularAdminPage.gotoSystem();

      await regularAdminPage.expectAccessDeniedForPermission('admin.view-system');
    });
  });

  test('user without admin.view-roles sees access denied on permissions', async ({
    page,
    browser,
    serverURL
  }) => {
    await createAndLoginAdminUser(page);

    await withRegularAdminPage(browser, serverURL, async ({ adminPage: regularAdminPage }) => {
      await regularAdminPage.gotoRoles();

      await regularAdminPage.expectAccessDeniedForPermission('admin.view-roles');
    });
  });

  test('user with admin.view-system permission still cannot see owner-only system page', async ({
    page,
    browser,
    serverURL
  }) => {
    await createAndLoginAdminUser(page);
    await grantPermission(page, 'everyone', 'admin.view-system');

    await withRegularAdminPage(browser, serverURL, async ({ adminPage: regularAdminPage }) => {
      await regularAdminPage.gotoSystem();

      await regularAdminPage.expectAccessDeniedForPermission('admin.view-system');
      await regularAdminPage.expectSidebarLinkNotVisible('System');
      await expect(regularAdminPage.page.getByText('Connected to Server')).toHaveCount(0);
    });

    // Clean up
    await revokePermission(page, 'everyone', 'admin.view-system');
  });

  // Note: a read-only view of the roles page (admin.view-roles without
  // admin.manage-roles) was removed when the UI moved from a per-role
  // editor to the unified matrix. The matrix's rolePermissionTierMatrix query gates on
  // instance admin / role.manage, so view-roles alone is currently not
  // sufficient to render the page. Re-add the test once the matrix grows
  // a read-only mode.

  test('nav items dynamically update based on granted permissions', async ({
    page,
    browser,
    serverURL
  }) => {
    // Start with one concrete admin capability.
    await createAndLoginAdminUser(page);
    await grantPermission(page, 'everyone', 'room.manage');

    await withRegularAdminPage(browser, serverURL, async ({ page: regularPage, adminPage }) => {
      await regularPage.goto(routes.serverAdminRooms);

      // Initially should only see the room management section
      await adminPage.expectSidebarLinkVisible('Rooms');
      await adminPage.expectSidebarLinkNotVisible('Users');

      // Now grant admin.view-users permission as admin
      await grantPermission(page, 'everyone', 'admin.view-users');

      // Reload and check nav updated
      await regularPage.reload();
      await adminPage.expectSidebarLinkVisible('Users');

      // Grant admin.view-system. System diagnostics remain owner-only for now,
      // so this permission alone must not reveal the route.
      await grantPermission(page, 'everyone', 'admin.view-system');
      await regularPage.reload();
      await adminPage.expectSidebarLinkNotVisible('System');
    });

    // Clean up
    await revokePermission(page, 'everyone', 'room.manage');
    await revokePermission(page, 'everyone', 'admin.view-users');
    await revokePermission(page, 'everyone', 'admin.view-system');
  });
});

test.describe('User Permission Management', () => {
  test('admin can navigate to user management page', async ({ page, adminPage }) => {
    const adminUser = await createAndLoginAdminUser(page);

    // Go to users list
    await adminPage.gotoUsers();

    // Click on a user row (the admin user themselves)
    await adminPage.clickUser(adminUser.login);

    // Should see user details
    await adminPage.expectUserManagementVisible();
    // The login should appear in the user profile section
    const userDetailsPanel = page.locator('.panel-shell').filter({
      has: page.getByRole('heading', { name: 'User Details' })
    });
    await expect(userDetailsPanel.getByText(`@${adminUser.login}`, { exact: true })).toBeVisible();
  });

  test('granting a role with admin.view-users gives user admin access', async ({
    page,
    browser,
    serverURL
  }) => {
    await createAndLoginAdminUser(page);

    await withRegularAdminPage(
      browser,
      serverURL,
      async ({ page: regularPage, user: regularUser, adminPage: regularAdminPage }) => {
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
      }
    );
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
  // the remaining RBAC concepts.
});

// "Browse Spaces Permission" describe block was retired with the Browse
// Spaces UI in PR(a); the `space.list` permission has since been removed.
// Deny-role behaviour is exercised by the other admin permission tests.

test.describe('Instance Settings', () => {
  // No per-test cleanup needed: every e2e test spawns its own isolated
  // chatto instance (see apps/frontend/e2e/fixtures/server.ts), so state
  // never leaks between tests.

  test('admin can access instance settings page', async ({ page, adminPage }) => {
    await createAndLoginAdminUser(page);

    await adminPage.gotoServerSettings();

    await adminPage.expectServerSettingsVisible();
    // The e2e fixture's [bootstrap.instance] block seeds the instance name on
    // first boot (see apps/frontend/e2e/fixtures/chatto.toml).
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
    browser,
    serverURL
  }) => {
    // First admin user logs in and goes to settings
    await createAndLoginAdminUser(page);

    await withServerUser(browser, serverURL, async ({ page: page2 }) => {
      // Second context - a regular user viewing the chat
      await page2.goto('/chat');

      // Verify no MOTD initially on second page
      await expect(page2.getByTestId('motd-content')).not.toBeVisible();

      // First page (admin): go to settings and set MOTD
      await adminPage.gotoServerSettings();
      await adminPage.fillServerSettings({ motd: 'Live update test!' });
      await adminPage.saveServerSettings();

      // Second page (regular user) should now show the MOTD (via live events)
      await expect(page2.getByTestId('motd-content')).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });
      await expect(page2.getByTestId('motd-content')).toContainText('Live update test!');
    });
  });

  test('instance name changes update page title in real-time for connected clients', async ({
    page,
    adminPage,
    browser,
    serverURL
  }) => {
    // First admin user logs in
    await createAndLoginAdminUser(page);

    await withServerUser(browser, serverURL, async ({ page: page2 }) => {
      // Second context - a regular user viewing the chat
      await page2.goto(routes.spaces);

      // Verify initial page title contains *some* instance name (post-PR(a)
      // this is the bootstrap server's name when no override is configured —
      // see `InstanceConfig.serverName` resolver fallback chain). The
      // assertion below for the *changed* name is the meaningful signal.
      await expect(page2).not.toHaveTitle('');

      // First page (admin): go to settings and change instance name
      await adminPage.gotoServerSettings();
      await adminPage.fillServerSettings({ serverName: 'Live Title Test' });
      await adminPage.saveServerSettings();

      // Second page (regular user) should now show updated instance name in title (via live events)
      await expect(page2).toHaveTitle(/Live Title Test/, { timeout: TIMEOUTS.UI_STANDARD });
    });
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
    const createdRole = await createRoleViaConnect(
      page,
      roleName,
      'Test Denial Role',
      'A role for testing permission denials'
    );
    expect(createdRole.permissionDenials ?? []).toEqual([]);

    // Deny a permission on the role
    await setRolePermissionViaConnect(page, roleName, 'message.post', 'PERMISSION_DECISION_DENY');

    // Query the role and verify the denial persists
    const role = await getRoleViaConnect(page, roleName);
    expect(role.permissionDenials).toContain('message.post');

    // Clean up - delete the role
    await deleteRoleViaConnect(page, roleName);
  });

  test('admin can deny a permission on a role via UI and it persists', async ({
    page,
    adminPage
  }) => {
    await createAndLoginAdminUser(page);

    // Create a custom role via API
    const roleName = generateRoleName('deny');
    await createRoleViaConnect(
      page,
      roleName,
      'UI Denial Test Role',
      'A role for testing permission denial UI'
    );

    // The matrix lives on the roles listing page now: rows are permissions,
    // columns are roles, and each cell is a button whose aria-label encodes
    // both the role displayName and the permission. Clicking cycles
    // neutral → allow → deny → neutral, so two clicks lands a fresh role on
    // Deny.
    const displayName = 'UI Denial Test Role';
    await adminPage.gotoRoles();
    await expect(page.getByRole('heading', { name: 'Permissions', level: 1 })).toBeVisible();

    const cell = page.locator(`td[data-role="${roleName}"][data-permission="message.post"] button`);
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
    await deleteRoleViaConnect(page, roleName);
  });
});

test.describe('Identity Editing', () => {
  test('admin can rename a user and reset their cooldown', async ({
    page,
    adminPage,
    browser,
    serverURL
  }) => {
    await createAndLoginAdminUser(page);

    await withServerUser(
      browser,
      serverURL,
      async ({ page: regularPage, user: regularUser }) => {
        // The regular user changes their own login first to set a cooldown
        // timestamp. We need this to verify Reset cooldown actually clears it.
        const userChosenLogin = `userpicked${Date.now()}`;
        const userRename = await updateOwnProfileViaConnect(regularPage, {
          login: userChosenLogin
        });
        expect(userRename.login).toBe(userChosenLogin);

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
        const secondRename = await updateOwnProfileViaConnect(regularPage, {
          login: userSecondRename
        });
        expect(secondRename.login).toBe(userSecondRename);
      },
      { userOptions: { loginPrefix: 'edituser' } }
    );
  });
});
