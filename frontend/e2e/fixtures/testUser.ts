import { expect, type Page } from '@playwright/test';

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
 * The admin user is created during server startup via the bootstrap endpoint.
 * This user has the first-user admin role assigned.
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

  // Get the user ID from the me query
  const meResponse = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `query { me { id } }`
    }
  });

  expect(meResponse.ok()).toBeTruthy();
  const meData = await meResponse.json();
  adminUser.id = meData.data.me.id;

  return adminUser;
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
 * Grants an instance permission to a role (admin-only operation).
 * Must be called while logged in as an admin user.
 */
export async function grantInstancePermission(
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
				mutation GrantInstancePermission($input: GrantInstancePermissionInput!) { grantInstancePermission(input: $input)
				}
			`,
      variables: { input: { role, permission } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.grantInstancePermission).toBe(true);
}

/**
 * Revokes an instance permission from a role (admin-only operation).
 * Must be called while logged in as an admin user.
 */
export async function revokeInstancePermission(
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
				mutation RevokeInstancePermission($input: RevokeInstancePermissionInput!) { revokeInstancePermission(input: $input)
				}
			`,
      variables: { input: { role, permission } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.revokeInstancePermission).toBe(true);
}

/**
 * Denies an instance permission on a role (admin-only operation).
 * This adds the permission to the role's permissionDenials list.
 * Must be called while logged in as an admin user.
 */
export async function denyInstancePermission(
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
				mutation DenyInstancePermission($input: DenyInstancePermissionInput!) { denyInstancePermission(input: $input)
				}
			`,
      variables: { input: { role, permission } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.denyInstancePermission).toBe(true);
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
				mutation ClearInstancePermissionState($input: ClearInstancePermissionStateInput!) { clearInstancePermissionState(input: $input)
				}
			`,
      variables: { input: { role, permission } }
    }
  });

  expect(response.ok()).toBeTruthy();
  const data = await response.json();
  expect(data.data?.clearInstancePermissionState).toBe(true);
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
 * Creates a custom instance role that denies a permission, then assigns it to a user.
 * Returns the role name so it can be revoked later.
 * Must be called while logged in as an admin user.
 */
export async function denyUserInstancePermission(
  page: Page,
  userId: string,
  permission: string
): Promise<string> {
  const suffix = numberToLetters(++denyRoleCounter);
  const roleName = `instance-deny${suffix}`;
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
  await denyInstancePermission(page, roleName, permission);

  // Assign role to user
  const assignResp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation AssignRole($input: AssignInstanceRoleInput!) { assignInstanceRole(input: $input) }`,
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
export async function clearUserInstancePermissionOverride(
  page: Page,
  userId: string,
  _permission: string,
  roleName?: string
): Promise<void> {
  if (!roleName) {
    // If no role name provided, we can't clean up properly.
    // Tests should track the role name from denyUserInstancePermission.
    throw new Error('clearUserInstancePermissionOverride requires roleName parameter');
  }

  const resp = await page.request.post('/api/graphql', {
    headers: { 'Content-Type': 'application/json', 'X-REQUEST-TYPE': 'GraphQL' },
    data: {
      query: `mutation RevokeRole($input: RevokeInstanceRoleInput!) { revokeInstanceRole(input: $input) }`,
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

/**
 * Joins a space via GraphQL API.
 * Useful in multi-user tests where a second user needs to join an existing space.
 */
export async function joinSpace(page: Page, spaceId: string): Promise<void> {
  const response = await page.request.post('/api/graphql', {
    headers: {
      'Content-Type': 'application/json',
      'X-REQUEST-TYPE': 'GraphQL'
    },
    data: {
      query: `mutation JoinSpace($input: JoinSpaceInput!) { joinSpace(input: $input) }`,
      variables: { input: { spaceId } }
    }
  });
  expect(response.ok()).toBeTruthy();
}

export interface CreateTestUserOptions {
  /** Custom prefix for the login (default: 'testuser') */
  loginPrefix?: string;
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

  // Create user via the test-only endpoint. Bypasses the full registration
  // flow (email delivery, token verification, session creation) — these are
  // covered by dedicated registration-flow tests, not every test that needs
  // *some* user. The endpoint is gated behind the `test_endpoints` build
  // tag and is never compiled into production binaries.
  const createUserResponse = await page.request.post('/auth/test/create-user', {
    headers: { 'Content-Type': 'application/json' },
    data: {
      login: testUser.login,
      displayName: testUser.displayName,
      password: testUser.password
    }
  });
  expect(createUserResponse.ok()).toBeTruthy();
  const createUserData = (await createUserResponse.json()) as { id: string };
  testUser.id = createUserData.id;

  // Verify user's email so admin checks, password reset, etc. behave as if
  // the user completed real registration.
  const verifyResponse = await page.request.post('/auth/test/verify-email', {
    headers: { 'Content-Type': 'application/json' },
    data: { userId: testUser.id, email: `${testUser.login}@example.com` }
  });
  expect(verifyResponse.ok()).toBeTruthy();

  // Login via HTTP endpoint to get the session cookie on the page.
  const loginResponse = await page.request.post('/auth/login', {
    data: { login: testUser.login, password: testUser.password }
  });
  expect(loginResponse.ok()).toBeTruthy();
  const loginData = await loginResponse.json();
  expect(loginData.success).toBe(true);

  return testUser;
}

/**
 * Generates a valid space role name with only lowercase letters.
 * Space role names must match ^[a-z]{1,32}$.
 * @param prefix - A lowercase letter prefix (e.g., 'test', 'edit')
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

/**
 * Generates a valid instance role name.
 * Instance role names must match ^instance-[a-z]{1,23}$.
 * @param suffix - A lowercase letter suffix (e.g., 'test', 'deny')
 * @returns A unique role name like 'instance-testabcdefgh'
 */
export function generateInstanceRoleName(suffix: string): string {
  const chars = 'abcdefghijklmnopqrstuvwxyz';
  let randomPart = '';
  for (let i = 0; i < 6; i++) {
    randomPart += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return `instance-${suffix}${randomPart}`;
}

