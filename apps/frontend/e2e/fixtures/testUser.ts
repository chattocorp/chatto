import { expect, type Page } from '@playwright/test';
import { csrfHeaders } from './csrf';
import { unloadPageForIdentitySwitch } from './navigation';

export interface TestUser {
  id?: string;
  login: string;
  displayName: string;
  password: string;
}

/**
 * The admin email used for granting admin access in E2E tests.
 * Must match what's configured in e2e/fixtures/chatto.toml
 */
const ADMIN_EMAIL = 'admin@e2e-test.example.com';

/**
 * Logs in as the bootstrap admin user.
 * The admin user is created during server startup via the [bootstrap]
 * section in fixtures/chatto.toml, which assigns the owner role.
 *
 * Note: You must also verify the admin email to get config-based admin access
 * (for admin panel). Use verifyAdminEmail() after calling this if needed.
 */
export async function loginAsAdmin(page: Page): Promise<TestUser> {
  const adminUser: TestUser = {
    login: 'e2eadmin',
    displayName: 'Admin User',
    password: 'adminpassword123'
  };

  // Login via HTTP endpoint (user already created by bootstrap)
  const loginResponse = await page.request.post('/auth/login', {
    data: {
      login: adminUser.login,
      password: adminUser.password
    }
  });

  expect(loginResponse.ok()).toBeTruthy();

  // Get the user ID from the viewer query
  const meResponse = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `query { viewer { user { id } } }`
    }
  });

  expect(meResponse.ok()).toBeTruthy();
  const meData = await meResponse.json();
  adminUser.id = meData.data.viewer.user.id;

  return adminUser;
}

/**
 * Logs out the active request context without leaving the mounted SPA around to
 * react to the session change. Capture CSRF while still on the app, unload the
 * app while the session is still valid, then perform the logout request.
 */
export async function logoutCurrentUser(page: Page): Promise<void> {
  const headers = await csrfHeaders(page);
  await unloadPageForIdentitySwitch(page);
  const response = await page.request.post('/auth/logout', { headers });
  expect(response.ok()).toBeTruthy();
}

/**
 * Logs in as the bootstrap admin user (idempotent re-auth on the same page)
 * and returns the deployment's primary server — the bootstrap "E2E Test Server"
 * created by fixtures/chatto.toml. Admin-style tests use this to run room,
 * layout, and role operations with sufficient permissions.
 */
export async function loginAsAdminAndUsePrimaryServer(
  page: Page
): Promise<{ id: string; name: string }> {
  await loginAsAdmin(page);
  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `query { server { profile { name } } }`
    }
  });
  expect(resp.ok()).toBeTruthy();
  const data = await resp.json();
  const instance = data.data?.server;
  if (!instance) {
    throw new Error('Server query returned no data — bootstrap profile likely broken');
  }
  // Post-ADR-030 the kind discriminator stands in for legacy spaceId parameters.
  return {
    id: 'server',
    name: instance.profile.name
  };
}

/**
 * Verifies the admin email for the currently logged-in admin user.
 * This grants config-based admin access (for admin panel routes).
 */
export async function verifyAdminEmail(page: Page, userId: string): Promise<void> {
  const response = await page.request.post('/auth/test/verify-email', {
    headers: { 'Content-Type': 'application/json' },
    data: { userId, email: ADMIN_EMAIL }
  });
  expect(response.ok()).toBeTruthy();
}

/**
 * Grants a permission to a role (admin-only operation).
 * Must be called while logged in as an admin user.
 */
export async function grantPermission(page: Page, role: string, permission: string): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation GrantInstancePermission($input: GrantPermissionInput!) { grantPermission(input: $input)
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
 * Revokes a permission from a role (admin-only operation).
 * Must be called while logged in as an admin user.
 */
export async function revokePermission(
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
				mutation RevokeInstancePermission($input: RevokePermissionInput!) { revokePermission(input: $input)
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
 * Denies a permission on a role (admin-only operation).
 * This adds the permission to the role's permissionDenials list.
 * Must be called while logged in as an admin user.
 */
export async function denyPermission(page: Page, role: string, permission: string): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `
				mutation DenyInstancePermission($input: DenyPermissionInput!) { denyPermission(input: $input)
				}
			`,
      variables: { input: { roleName: role, permission } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.denyPermission).toBe(true);
}

/**
 * Clears the permission state on a role (admin-only operation).
 * This removes the permission from both grants and denials (neutral state).
 * Must be called while logged in as an admin user.
 */
export async function clearInstancePermissionState(
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
				mutation ClearServerPermissionState($input: ClearPermissionStateInput!) { clearPermissionState(input: $input)
				}
			`,
      variables: { input: { roleName: role, permission } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.clearPermissionState).toBe(true);
}

let denyRoleCounter = 0;

/** Convert a number to a lowercase letter sequence: 1→a, 2→b, ..., 26→z, 27→aa, etc. */
function numberToLetters(n: number): string {
  let result = '';
  while (n > 0) {
    n--;
    result = String.fromCharCode(97 + (n % 26)) + result;
    n = Math.floor(n / 26);
  }
  return result;
}

/**
 * Creates a custom role that denies a permission, then assigns it to a user.
 * Returns the role name so it can be revoked later.
 * Must be called while logged in as an admin user.
 */
export async function denyUserPermission(
  page: Page,
  userId: string,
  permission: string
): Promise<string> {
  const suffix = numberToLetters(++denyRoleCounter);
  const roleName = `deny${suffix}`;
  const displayName = `Deny ${permission} #${denyRoleCounter}`;

  // Create role
  const createResp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation CreateRole($input: CreateRoleInput!) { createRole(input: $input) { name } }`,
      variables: {
        input: { name: roleName, displayName, description: `Auto-created to deny ${permission}` }
      }
    }
  });
  expect(createResp.ok()).toBeTruthy();

  // Deny permission on role
  await denyPermission(page, roleName, permission);

  // Assign role to user
  const assignResp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation AssignRole($input: AssignRoleInput!) { assignRole(input: $input) }`,
      variables: { input: { userId, roleName } }
    }
  });
  expect(assignResp.ok()).toBeTruthy();

  return roleName;
}

/**
 * Revokes a deny role from a user, effectively clearing the permission denial.
 * Must be called while logged in as an admin user.
 */
export async function clearUserPermissionOverride(
  page: Page,
  userId: string,
  _permission: string,
  roleName?: string
): Promise<void> {
  if (!roleName) {
    // If no role name provided, we can't clean up properly.
    // Tests should track the role name from denyUserPermission.
    throw new Error('clearUserPermissionOverride requires roleName parameter');
  }

  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation RevokeRole($input: RevokeRoleInput!) { revokeRole(input: $input) }`,
      variables: { input: { userId, roleName } }
    }
  });
  expect(resp.ok()).toBeTruthy();
}

/**
 * Logs in an existing test user (created by createAndLoginTestUser).
 * Useful for multi-tab tests where the same user needs to be logged into multiple pages.
 */
export async function loginTestUser(page: Page, user: TestUser): Promise<void> {
  const loginResponse = await page.request.post('/auth/login', {
    data: {
      login: user.login,
      password: user.password
    }
  });

  expect(loginResponse.ok()).toBeTruthy();
  const loginData = await loginResponse.json();
  expect(loginData.success).toBe(true);
}

/** Navigate an authenticated user to the local server's chat surface. */
export async function openServer(page: Page): Promise<void> {
  await page.goto('/chat');
  await page.waitForURL((url) => url.pathname.startsWith('/chat'));
}

export interface CreateTestUserOptions {
  /** Custom prefix for the login (default: 'testuser') */
  loginPrefix?: string;
  /**
   * Skip the auto-join into the bootstrap default rooms (announcements +
   * general). Tests that exercise the "fresh user with empty sidebar"
   * path (e.g. Join-all-on-Overview coverage) opt out via this flag.
   */
  skipDefaultRooms?: boolean;
}

/**
 * Creates a test user (with verified email) and logs them in.
 * Returns the user credentials for use in tests.
 */
export async function createAndLoginTestUser(
  page: Page,
  options?: CreateTestUserOptions
): Promise<TestUser> {
  const timestamp = Date.now();
  const prefix = options?.loginPrefix ?? 'testuser';
  const testUser: TestUser = {
    login: `${prefix}${timestamp}`,
    displayName: `Test User ${timestamp}`,
    password: 'testpassword123'
  };

  // Create, verify, log in, and optionally join the default rooms via one
  // test-only endpoint. The production registration/login flow is covered by
  // dedicated tests; most E2E tests just need a ready authenticated user.
  const createUserResponse = await page.request.post('/auth/test/create-user-session', {
    headers: { 'Content-Type': 'application/json' },
    data: {
      login: testUser.login,
      displayName: testUser.displayName,
      password: testUser.password,
      email: `${testUser.login}@example.com`,
      joinDefaultRooms: !options?.skipDefaultRooms
    }
  });
  expect(createUserResponse.ok()).toBeTruthy();
  const createUserData = (await createUserResponse.json()) as {
    user: { id: string };
  };
  testUser.id = createUserData.user.id;

  return testUser;
}

/**
 * Generates a valid role name with only lowercase letters.
 * Role names must match ^[a-z]{1,32}$.
 * @param prefix - A lowercase letter prefix (e.g., 'test', 'edit', 'deny')
 * @returns A unique role name like 'testabcdefgh'
 */
export function generateRoleName(prefix: string): string {
  const chars = 'abcdefghijklmnopqrstuvwxyz';
  let suffix = '';
  for (let i = 0; i < 8; i++) {
    suffix += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return prefix + suffix;
}
