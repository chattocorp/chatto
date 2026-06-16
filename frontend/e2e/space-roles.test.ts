import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  generateRoleName,
  loginAsAdminAndUsePrimarySpace,
  type TestUser
} from './fixtures/testUser';
import { csrfHeaders } from './fixtures/csrf';
import { unloadPageForIdentitySwitch } from './fixtures/navigation';
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
  const headers = await csrfHeaders(page);
  // Unload the SPA before switching identities. Otherwise the old authenticated
  // app can react to logout and race later navigations with its own redirect.
  await unloadPageForIdentitySwitch(page);
  const response = await page.request.post('/auth/logout', { headers });
  expect(response.ok()).toBeTruthy();
}

/**
 * Joins a space via GraphQL API.
 */
async function joinSpaceViaAPI(_page: Page, _spaceId: string): Promise<void> {
  // no-op post-#330 PR(a) — server membership is implicit on signup.
}

/**
 * Creates a room via GraphQL API and returns the room ID.
 */
async function createRoomViaAPI(
  page: Page,
  spaceIdOrName?: string,
  maybeName?: string
): Promise<string> {
  const roomName =
    maybeName ??
    (spaceIdOrName && spaceIdOrName !== 'server' ? spaceIdOrName : undefined) ??
    `testroom${Date.now()}`;
  const groupId = await getDefaultRoomGroupId(page);
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation CreateRoom($input: CreateRoomInput!) {
					createRoom(input: $input) { id name }
				}
			`,
      variables: { input: { name: roomName, groupId } }
    }
  });
  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.createRoom).toBeTruthy();
  return data.data.createRoom.id;
}

async function getDefaultRoomGroupId(page: Page): Promise<string> {
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: { query: `query { server { roomGroups { id } } }` }
  });
  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  const groupId = data.data?.server?.roomGroups?.[0]?.id;
  if (!groupId) {
    throw new Error(`No room group available for e2e room creation: ${JSON.stringify(data)}`);
  }
  return groupId;
}

/**
 * Joins a room via GraphQL API.
 */
async function joinRoomViaAPI(
  page: Page,
  spaceIdOrRoomId: string,
  maybeRoomId?: string
): Promise<void> {
  const roomId = maybeRoomId ?? spaceIdOrRoomId;
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation JoinRoom($input: JoinRoomInput!) {
					joinRoom(input: $input) { id }
				}
			`,
      variables: { input: { roomId } }
    }
  });
  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.joinRoom?.id).toBe(roomId);
}

/**
 * Grants a space permission to a role via GraphQL API.
 */
async function grantPermission(
  page: Page,
  _spaceId: string,
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

/**
 * Revokes a space permission from a role via GraphQL API.
 */
async function _revokePermission(page: Page, role: string, permission: string): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation RevokePerm($input: RevokePermissionInput!) {
					revokePermission(input: $input)
				}
			`,
      variables: { input: { roleName: role, permission } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.revokePermission).toBe(true);
}

/**
 * Denies a space permission for a role via GraphQL API.
 */
async function denyPermission(
  page: Page,
  _spaceId: string,
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
				mutation DenyPerm($input: DenyPermissionInput!) {
					denyPermission(input: $input)
				}
			`,
      variables: { input: { roleName: role, permission } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.denyPermission).toBe(true);
}

test.describe('Space Roles Management', () => {
  test.describe('Roles List Page', () => {
    test('space admin can view roles list', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      // Create user and space (creator is admin)
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to roles list
      await spaceRolesPage.gotoRolesList(space.id);

      // Should see the roles table with default roles
      await spaceRolesPage.expectRolesListVisible();
      await spaceRolesPage.expectRoleInList('owner');
      await spaceRolesPage.expectRoleInList('everyone');
    });

    test('space admin can see Create Role button', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      await spaceRolesPage.gotoRolesList(space.id);
      await spaceRolesPage.expectCreateRoleButtonVisible();
    });

    test('non-admin member sees access denied on roles page', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create and login as non-admin user
      const nonAdmin = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, nonAdmin.login, nonAdmin.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to roles list (bypassing nav filtering)
      await page.goto(routes.serverAdminRoles);

      // Users without roles.manage permission see Access Denied
      await spaceRolesPage.expectAccessDenied();
    });
  });

  test.describe('Create Role', () => {
    test('space admin can create a new role', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const roleName = generateRoleName('test');

      // Create a new role
      await spaceRolesPage.createRole(space.id, {
        name: roleName,
        displayName: 'Test Role',
        description: 'A role for testing'
      });

      // Should be redirected to edit page for the new role
      await spaceRolesPage.expectRoleName(roleName);

      // Navigate back to list and verify role appears
      await spaceRolesPage.backToRolesButton.click();
      await spaceRolesPage.expectRoleInList(roleName);
    });

    test('role name validation rejects invalid characters', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      await spaceRolesPage.gotoCreateRole(space.id);

      // Try to enter an invalid name (uppercase)
      await spaceRolesPage.fillRoleForm({ name: 'InvalidName' });
      await spaceRolesPage.expectValidationError(
        'Name must use lowercase letters, numbers, and dashes; start with a letter'
      );

      // Try special characters
      await spaceRolesPage.fillRoleForm({ name: 'test-role!' });
      await spaceRolesPage.expectValidationError(
        'Name must use lowercase letters, numbers, and dashes; start with a letter'
      );
    });

    test('role name validation rejects names starting with number', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      await spaceRolesPage.gotoCreateRole(space.id);

      // Try to enter a name starting with a number
      await spaceRolesPage.fillRoleForm({ name: '123role' });
      await spaceRolesPage.expectValidationError(
        'Name must use lowercase letters, numbers, and dashes; start with a letter'
      );
    });

    test('non-admin member sees access denied on create page', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create and login as non-admin user
      const nonAdmin = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, nonAdmin.login, nonAdmin.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to create role page (bypassing method that expects success)
      await page.goto(routes.serverAdminRolesNew);

      // Should see access denied
      await spaceRolesPage.expectAccessDenied();
    });
  });

  test.describe('Edit Role', () => {
    test('space admin can edit a custom role', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const roleName = generateRoleName('edit');

      // Create a role first
      await spaceRolesPage.createRole(space.id, {
        name: roleName,
        displayName: 'Edit Test Role',
        description: 'Original description'
      });

      // Update the display name and description
      await spaceRolesPage.fillRoleForm({
        displayName: 'Updated Role Name',
        description: 'Updated description'
      });
      await spaceRolesPage.saveChangesButton.click();

      // Verify changes persist after reload
      await page.reload();
      await expect(spaceRolesPage.displayNameInput).toHaveValue('Updated Role Name');
      await expect(spaceRolesPage.descriptionInput).toHaveValue('Updated description');
    });

    test('space admin can grant permission to a role', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const roleName = generateRoleName('perm');

      // Create a role
      await spaceRolesPage.createRole(space.id, {
        name: roleName,
        displayName: 'Permission Test Role'
      });

      // The role should start without server.manage permission
      await spaceRolesPage.expectPermissionNotGranted('server.manage');

      // Grant the permission
      await spaceRolesPage.togglePermission('server.manage');

      // Wait for the toast confirmation, then verify persistence
      await spaceRolesPage.expectToast('Granted server.manage');
      await page.reload();
      await spaceRolesPage.expectPermissionGranted('server.manage');
    });

    test('space admin can clear permission from a role', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // The everyone role has room.list by default
      await spaceRolesPage.gotoEditRole(space.id, 'everyone');
      await spaceRolesPage.expectPermissionGranted('room.list');

      // Clear the permission (set to neutral)
      await spaceRolesPage.togglePermission('room.list');

      // Wait for the toast confirmation, then verify persistence
      await spaceRolesPage.expectToast('Cleared room.list');
      await page.reload();
      await spaceRolesPage.expectPermissionNotGranted('room.list');
    });

    test('system role owner has virtual admin permissions granted read-only', async ({
      spaceRolesPage
    }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      await spaceRolesPage.gotoEditRole(space.id, 'owner');

      // Owner permissions are virtual, not persisted editable grants.
      await spaceRolesPage.expectOwnerPermissionVirtuallyGranted('user.delete-any');
      await spaceRolesPage.expectPermissionReadOnly('user.delete-any');
    });

    test('system roles cannot be deleted', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Check owner role
      await spaceRolesPage.gotoEditRole(space.id, 'owner');
      await spaceRolesPage.expectDeleteRoleButtonNotVisible();

      // Check everyone role
      await spaceRolesPage.gotoEditRole(space.id, 'everyone');
      await spaceRolesPage.expectDeleteRoleButtonNotVisible();
    });
  });

  test.describe('Delete Role', () => {
    test('space admin can delete a custom role', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const roleName = generateRoleName('del');

      // Create a role
      await spaceRolesPage.createRole(space.id, {
        name: roleName,
        displayName: 'Delete Test Role'
      });

      // Delete the role
      await spaceRolesPage.deleteCurrentRole();

      // Should be redirected to roles list
      await spaceRolesPage.expectRolesListVisible();

      // Role should no longer be in the list
      await spaceRolesPage.expectRoleNotInList('Delete Test Role');
    });

    test('custom roles show delete button', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const roleName = generateRoleName('custom');

      // Create a role
      await spaceRolesPage.createRole(space.id, {
        name: roleName,
        displayName: 'Custom Role'
      });

      // Should see delete button for custom role
      await spaceRolesPage.expectDeleteRoleButtonVisible();
    });
  });
});

test.describe('Roles Management', () => {
  test.describe('Roles List', () => {
    test('space admin can see roles in roles list', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      await spaceRolesPage.gotoRolesList(space.id);

      // The unified roles matrix should be visible
      await spaceRolesPage.expectRolesPanelVisible();

      // Should see system roles (not universal roles like `everyone`)
      await spaceRolesPage.expectRoleInList('admin');
    });

    // Removed: "space admin can navigate to role detail page".
    // The matrix gates role column-header clicks on
    // admin.manage-roles, so a non-admin space admin sees the header
    // as plain text — there's nothing to click. The
    // unit specs cover the onRoleClick wiring; the navigation flow itself
    // is exercised end-to-end by `admin can deny a permission on a role
    // via UI and it persists` in admin.test.ts.
  });

  test.describe('Role Permissions', () => {
    test('space admin can grant permission to role', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to moderator role detail page; admin already has role.manage by default.
      await spaceRolesPage.gotoRoleDetail(space.id, 'moderator');

      // The role should start without role.manage permission
      await spaceRolesPage.expectPermissionNotGranted('role.manage');

      // Grant the permission
      await spaceRolesPage.togglePermission('role.manage');

      // Wait for toast and verify persistence
      await spaceRolesPage.expectToast('Granted role.manage');
      await page.reload();
      await spaceRolesPage.expectPermissionGranted('role.manage');
    });

    test('space admin can deny permission for role', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to admin role detail page
      await spaceRolesPage.gotoRoleDetail(space.id, 'admin');

      // Deny a permission
      await spaceRolesPage.denyPermission('room.list');

      // Wait for toast and verify persistence
      await spaceRolesPage.expectToast('Denied room.list');
      await page.reload();
      await spaceRolesPage.expectPermissionDenied('room.list');
    });

    test('space admin can clear permission from role', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to admin role detail page
      await spaceRolesPage.gotoRoleDetail(space.id, 'admin');

      // First grant a permission
      await spaceRolesPage.togglePermission('role.manage');
      await spaceRolesPage.expectToast('Granted role.manage');
      await page.reload();

      // Now clear it (uncheck the Allow checkbox)
      await spaceRolesPage.togglePermission('role.manage');
      await spaceRolesPage.expectToast('Cleared role.manage');

      // Verify it's no longer granted
      await page.reload();
      await spaceRolesPage.expectPermissionNotGranted('role.manage');
    });
  });

  test.describe('Instance Role Permission Enforcement', () => {
    test('user with everyone role grant gets permission', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant role.manage to the "everyone" space role
      await grantPermission(page, space.id, 'everyone', 'role.manage');

      // Create second user
      const regularUser = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, regularUser.login, regularUser.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to roles list - should have create/manage access via everyone role grant.
      // The matrix itself is intentionally hidden from non-admins because role
      // permission inspection is still restricted.
      await spaceRolesPage.gotoRolesList(space.id);
      await spaceRolesPage.expectCreateRoleButtonVisible();
    });

    test('user with everyone role denial is blocked', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Deny role.manage on the "everyone" space role
      await denyPermission(page, space.id, 'everyone', 'role.manage');

      // Create second user
      const regularUser = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, regularUser.login, regularUser.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to roles list - should be denied because everyone role has denial
      await page.goto(routes.serverAdminRoles);
      await spaceRolesPage.expectAccessDenied();
    });
  });
});

test.describe('Space Permission Enforcement', () => {
  test.describe('rooms.manage permission (room creation)', () => {
    test('space admin can create a room via admin page', async ({ page }) => {
      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to admin rooms page
      await page.goto(routes.serverAdminRooms);

      // Should see the "New Room" button
      const newRoomButton = page.getByRole('button', { name: 'New Room' });
      await expect(newRoomButton).toBeVisible();

      // Click to create room - modal should appear
      await newRoomButton.click();
      await expect(page.getByRole('dialog')).toBeVisible();
      await expect(page.getByLabel('Room Name')).toBeVisible();
    });

    test('non-admin user cannot access admin rooms page', async ({ page }) => {
      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create second user and log them in
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to admin rooms page - should be denied
      await page.goto(routes.serverAdminRooms);
      await expect(page.getByText('Access Denied', { exact: true })).toBeVisible();
    });
  });

  test.describe('roles.manage permission', () => {
    test('user with roles.manage permission can access roles settings', async ({
      spaceRolesPage
    }) => {
      const { page } = spaceRolesPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Grant role.manage to everyone role
      await grantPermission(page, space.id, 'everyone', 'role.manage');

      // Create second user and log them in
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to roles list
      await spaceRolesPage.gotoRolesList(space.id);

      // Should see Create Role button (has roles.manage)
      await spaceRolesPage.expectCreateRoleButtonVisible();
    });

    test('user without roles.manage permission sees access denied', async ({ spaceRolesPage }) => {
      const { page } = spaceRolesPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create second user (everyone role doesn't have role.manage by default)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate directly to roles list (bypassing nav filtering)
      await page.goto(routes.serverAdminRoles);

      // Users without roles.manage permission see Access Denied
      await spaceRolesPage.expectAccessDenied();
    });
  });

  test.describe('space.manage permission', () => {
    test('user with space.manage permission can see settings link', async ({ spaceAdminPage }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to space
      await page.goto(routes.space());

      // Admin should see settings link in sidebar
      await spaceAdminPage.expectAdminLinkVisible();
    });

    test('user without space.manage permission cannot see settings link', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create second user (regular member)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Navigate to space
      await page.goto(routes.space());
      await expect(page.getByRole('heading', { name: space.name })).toBeVisible();

      // Non-admin should not see settings link in sidebar
      await spaceAdminPage.expectAdminLinkNotVisible();
    });
  });

  test.describe('room.list permission', () => {
    test('user with room.list permission can list rooms via API', async ({ page }) => {
      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create second user (everyone role has room.list by default)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Try to list rooms via GraphQL
      const response = await page.request.post('/api/graphql', {
        headers: {
          'Content-Type': 'application/json',
          'X-REQUEST-TYPE': 'GraphQL'
        },
        data: {
          query: `
						query ListRooms {
							server {
								rooms { id name }
							}
						}
					`,
          variables: { spaceId: space.id }
        }
      });

      const data = await response.json();

      // Should succeed - member has room.list
      expect(data.errors).toBeUndefined();
      expect(data.data?.server?.rooms).toBeDefined();
    });

    test('user without room.list permission cannot list rooms via API', async ({ page }) => {
      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      const hiddenRoomId = await createRoomViaAPI(page);

      // Deny room.list from everyone role. Users can still see rooms they
      // already joined; this assertion checks that an unjoined room is filtered.
      await denyPermission(page, space.id, 'everyone', 'room.list');

      // Create second user and log them in
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Try to list rooms via GraphQL
      const response = await page.request.post('/api/graphql', {
        headers: {
          'Content-Type': 'application/json',
          'X-REQUEST-TYPE': 'GraphQL'
        },
        data: {
          query: `
						query ListRooms {
							server {
								rooms { id name }
							}
						}
					`,
          variables: { spaceId: space.id }
        }
      });

      const data = await response.json();

      expect(data.errors).toBeUndefined();
      expect(data.data?.server?.rooms ?? []).not.toContainEqual(
        expect.objectContaining({ id: hiddenRoomId })
      );
    });

    // The three tests that previously asserted "room.list gates the
    // sidebar's Browse Rooms link / page" are obsolete: Browse Rooms is
    // retired, the Overview page is always reachable, and `room.list`
    // now only filters which rooms appear in the directory inside
    // Overview (covered by the `Server.rooms` resolver, exercised in
    // unit tests).
  });

  test.describe('room.join permission', () => {
    test('user with room.join permission can join a room', async ({ page }) => {
      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const groupId = await getDefaultRoomGroupId(page);

      // Create a room (admin has room.create by default)
      const roomResponse = await page.request.post('/api/graphql', {
        headers: {
          'Content-Type': 'application/json',
          'X-REQUEST-TYPE': 'GraphQL'
        },
        data: {
          query: `
						mutation CreateRoom($input: CreateRoomInput!) {
							createRoom(input: $input) { id name }
						}
					`,
          variables: {
            input: { name: `testroom${Date.now()}`, groupId }
          }
        }
      });
      expect(roomResponse.ok()).toBeTruthy();
      const roomData = await roomResponse.json();
      expect(roomData.errors).toBeUndefined();
      expect(roomData.data?.createRoom).toBeTruthy();
      const roomId = roomData.data.createRoom.id;

      // Create second user (everyone role has room.join by default)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Try to join the room
      const joinResponse = await page.request.post('/api/graphql', {
        headers: {
          'Content-Type': 'application/json',
          'X-REQUEST-TYPE': 'GraphQL'
        },
        data: {
          query: `
						mutation JoinRoom($input: JoinRoomInput!) {
							joinRoom(input: $input) { id }
						}
					`,
          variables: { input: { roomId } }
        }
      });

      const joinData = await joinResponse.json();

      // Should succeed - member has room.join
      expect(joinData.errors).toBeUndefined();
      expect(joinData.data?.joinRoom?.id).toBe(roomId);
    });

    test('user without room.join permission cannot join a room', async ({ page }) => {
      // Create admin user and space
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const groupId = await getDefaultRoomGroupId(page);

      // Create a room (admin has room.create by default)
      const roomResponse = await page.request.post('/api/graphql', {
        headers: {
          'Content-Type': 'application/json',
          'X-REQUEST-TYPE': 'GraphQL'
        },
        data: {
          query: `
						mutation CreateRoom($input: CreateRoomInput!) {
							createRoom(input: $input) { id name }
						}
					`,
          variables: {
            input: { name: `testroom${Date.now()}`, groupId }
          }
        }
      });
      expect(roomResponse.ok()).toBeTruthy();
      const roomData = await roomResponse.json();
      expect(roomData.errors).toBeUndefined();
      expect(roomData.data?.createRoom).toBeTruthy();
      const roomId = roomData.data.createRoom.id;

      // Deny room.join from everyone role
      await denyPermission(page, space.id, 'everyone', 'room.join');

      // Create second user and log them in
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Try to join the room
      const joinResponse = await page.request.post('/api/graphql', {
        headers: {
          'Content-Type': 'application/json',
          'X-REQUEST-TYPE': 'GraphQL'
        },
        data: {
          query: `
						mutation JoinRoom($input: JoinRoomInput!) {
							joinRoom(input: $input) { id }
						}
					`,
          variables: { input: { roomId } }
        }
      });

      const joinData = await joinResponse.json();

      // Should fail - room.join is denied
      expect(joinData.errors).toBeDefined();
      expect(joinData.errors.length).toBeGreaterThan(0);
    });

    test('chat input is disabled when user lacks message.post permission', async ({ page }) => {
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const groupId = await getDefaultRoomGroupId(page);

      const roomResponse = await page.request.post('/api/graphql', {
        headers: {
          'Content-Type': 'application/json',
          'X-REQUEST-TYPE': 'GraphQL'
        },
        data: {
          query: `
						mutation CreateRoom($input: CreateRoomInput!) {
							createRoom(input: $input) { id name }
						}
					`,
          variables: {
            input: { name: `testroom${Date.now()}`, groupId }
          }
        }
      });
      expect(roomResponse.ok()).toBeTruthy();
      const roomData = await roomResponse.json();
      const roomId = roomData.data.createRoom.id;

      // Deny message.post for everyone role at space level
      await denyPermission(page, space.id, 'everyone', 'message.post');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      const joinResponse = await page.request.post('/api/graphql', {
        headers: {
          'Content-Type': 'application/json',
          'X-REQUEST-TYPE': 'GraphQL'
        },
        data: {
          query: `
						mutation JoinRoom($input: JoinRoomInput!) {
							joinRoom(input: $input) { id }
						}
					`,
          variables: { input: { roomId } }
        }
      });
      expect((await joinResponse.json()).data?.joinRoom?.id).toBe(roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));

      // Chat input should be disabled
      await expect(page.getByTestId('message-input')).toHaveAttribute('contenteditable', 'false');

      // Send button should also be disabled
      await expect(page.getByTitle('Send message')).toBeDisabled();
    });

    test('reaction buttons hidden when user lacks message.react permission', async ({
      page,
      roomPage
    }) => {
      // Admin creates space, room, joins, and sends a message
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Admin navigates and sends a message
      await page.goto(routes.room(roomId));
      await roomPage.sendMessage('Hello world');

      // Deny message.react for everyone role
      await denyPermission(page, space.id, 'everyone', 'message.react');

      // Create second user, join space and room
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));

      // Wait for message to be visible
      await expect(page.getByText('Hello world')).toBeVisible();

      // Open context menu via toolbar — reaction buttons should not be present
      const message = roomPage.getMessage('Hello world');
      await message.expectContextMenuNoReaction();
    });

    // Retired permission coverage removed here:
    // - message.edit-own / message.delete-own no longer exist; authors can
    //   edit/delete their own messages subject to room membership.
    // - message.edit-any / message.delete-any were consolidated into
    //   message.manage plus hierarchy checks. The current behavior is covered
    //   by room-permissions.test.ts and backend resolver tests.
  });

  test.describe('room.manage permission', () => {
    test('administration gear hidden when user lacks room.manage permission', async ({ page }) => {
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Create second user without room.manage (everyone role doesn't have it by default)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));
      await expect(page.getByTitle('Leave room')).toBeVisible();

      // Administration gear should NOT be visible
      await expect(page.getByRole('link', { name: 'Server administration' })).not.toBeVisible();
    });

    test('administration gear visible when user has room.manage permission', async ({ page }) => {
      // Admin creates space and room
      await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);
      const roomId = await createRoomViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Grant server-scope room.manage to everyone.
      await grantPermission(page, space.id, 'everyone', 'room.manage');

      // Create second user and log in
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);
      await joinRoomViaAPI(page, space.id, roomId);

      // Navigate to the room
      await page.goto(routes.room(roomId));
      await expect(page.getByTitle('Leave room')).toBeVisible();

      // Administration gear should be visible
      await expect(page.getByRole('link', { name: 'Server administration' })).toBeVisible();
    });
  });
});
