import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  loginAsAdminAndUsePrimarySpace,
  type TestUser
} from './fixtures/testUser';
import { TIMEOUTS } from './constants';
import { csrfHeaders } from './fixtures/csrf';
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
  await page.request.post('/auth/logout', { headers: await csrfHeaders(page) });
}

/**
 * Vestigial helper kept for source-compat: post-#330 PR(a) joinSpace is gone.
 */
async function joinSpaceViaAPI(_page: Page, _spaceId: string): Promise<void> {
  // no-op
}

test.describe('Space Admin Members', () => {
  test.describe('Members List Page', () => {
    test('space admin can view members list', async ({ spaceAdminPage }) => {
      const { page } = spaceAdminPage;

      // Create user and space (creator is admin)
      const admin = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to members list
      await spaceAdminPage.gotoMembersDirectly(space.id);

      // Should see the members page header
      await expect(page.getByRole('heading', { name: 'Members', exact: true })).toBeVisible();

      // Should see the admin user in the list
      await expect(page.getByText(admin.login)).toBeVisible();
    });

    test('space admin can navigate to their own member details from list', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      const admin = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to members list
      await spaceAdminPage.gotoMembersDirectly(space.id);

      // Wait for members to load
      await expect(page.getByText(`@${admin.login}`)).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Click on the row containing the admin's login (DataTable uses onRowClick)
      await page.getByRole('row').filter({ hasText: admin.login }).click();

      // Should navigate to member details page
      await expect(page).toHaveURL(routes.serverAdminMember(admin.id!));

      // Should see member details page
      await expect(page.getByRole('heading', { name: 'Member Details' })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
    });
  });

  test.describe('Member Details Page', () => {
    test('admin can view their own member details', async ({ spaceAdminPage }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space (admin is automatically the first member)
      const admin = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to the admin's own member details page
      await spaceAdminPage.gotoMemberDetails(space.id, admin.id!);

      // The Member Details heading should be visible (page loaded)
      await expect(page.getByRole('heading', { name: 'Member Details' })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Should NOT be loading or showing error
      await expect(page.getByText('Loading member...')).not.toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });
      await expect(page.getByText('Member not found')).not.toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });

      // Should see User Details panel
      await expect(page.locator('h2', { hasText: 'User Details' })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Should see admin's login
      await expect(page.getByText(`@${admin.login}`)).toBeVisible();
    });

    test('member details page shows role assignments', async ({ spaceAdminPage }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      const admin = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to admin's own member details
      await spaceAdminPage.gotoMemberDetails(space.id, admin.id!);

      // Wait for page to load
      await expect(page.getByRole('heading', { name: 'Member Details' })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Should NOT be loading
      await expect(page.getByText('Loading member...')).not.toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Should see Role Assignments section heading
      await expect(page.locator('h2', { hasText: 'Role Assignments' })).toBeVisible();

      // Should see at least one checkbox (role assignment control)
      await expect(page.locator('input[type="checkbox"]').first()).toBeVisible();
    });

    test('back to members button works', async ({ spaceAdminPage }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      const admin = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Navigate to admin's own member details
      await spaceAdminPage.gotoMemberDetails(space.id, admin.id!);

      // Wait for page to load
      await expect(page.getByRole('heading', { name: 'Member Details' })).toBeVisible({
        timeout: TIMEOUTS.REALTIME_EVENT
      });

      // Click back button
      await spaceAdminPage.backToMembersButton.click();

      // Should navigate back to members list
      await expect(page).toHaveURL(routes.serverAdminMembers);
      await expect(page.getByRole('heading', { name: 'Members', exact: true })).toBeVisible();
    });

    test('non-admin member sees access denied on member details page', async ({
      spaceAdminPage
    }) => {
      const { page } = spaceAdminPage;

      // Create admin user and space
      const admin = await createAndLoginTestUser(page);
      const space = await createSpaceViaAPI(page);

      // Create and add a second member (non-admin)
      const member = await createSecondTestUser(page);
      await logoutUser(page);
      await loginUser(page, member.login, member.password);
      await joinSpaceViaAPI(page, space.id);

      // Try to access the admin's member details page as non-admin
      await page.goto(routes.serverAdminMember(admin.id!));

      // Should see access denied
      await spaceAdminPage.expectAccessDenied();
    });
  });
});
