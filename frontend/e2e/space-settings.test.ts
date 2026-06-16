import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  loginAsAdminAndUsePrimarySpace,
  type TestUser
} from './fixtures/testUser';
import { csrfHeaders } from './fixtures/csrf';
import * as routes from './routes';

interface TestSpace {
  id: string;
  name: string;
}

/**
 * Issue #330 / ADR-027: createSpace mutation is gone. Re-login as e2eadmin
 * (bootstrap space owner) and return the primary space so the admin-style
 * tests in this file still run with sufficient permissions.
 */
async function createSpaceViaAPI(
  page: Page,
  _options?: { name?: string }
): Promise<TestSpace> {
  return loginAsAdminAndUsePrimarySpace(page);
}

/**
 * Creates a second test user (different from the space creator).
 * The user has a verified email so they can join/create spaces.
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
  // Unload the SPA before switching identities. Otherwise the old authenticated
  // app can react to logout and race later navigations with its own redirect.
  await page.goto('about:blank');
  const response = await page.request.post('/auth/logout', { headers: await csrfHeaders(page) });
  expect(response.ok()).toBeTruthy();
}

/**
 * Joins a space via GraphQL API (requires authenticated user).
 */
async function joinSpaceViaAPI(_page: Page, _spaceId: string): Promise<void> {
  // no-op post-#330 PR(a) — server membership is implicit on signup.
}

/**
 * Navigates to a specific space by ID.
 */
async function gotoSpace(page: Page): Promise<void> {
  await page.goto(routes.space());
}

test.describe('Space Admin Page', () => {
  test('space admin can access admin page', async ({ spaceAdminPage }) => {
    const { page } = spaceAdminPage;

    // Create user and space (creator is admin)
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page, { name: 'Admin Settings Test' });

    // Navigate to space and go to General settings page
    await spaceAdminPage.gotoGeneralDirectly(space.id);

    // Should see the form with space name
    await expect(spaceAdminPage.nameInput).toBeVisible();
    await spaceAdminPage.expectName(space.name);
  });

  test('space admin can edit name and save changes', async ({ spaceAdminPage }) => {
    const { page } = spaceAdminPage;

    // Create user and space
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page, { name: 'Original Name' });

    // Navigate to General settings page
    await spaceAdminPage.gotoGeneralDirectly(space.id);

    // Change the name
    const newName = `Updated Name ${Date.now()}`;
    await spaceAdminPage.updateName(newName);

    // Should see success message
    await spaceAdminPage.expectSaveSuccess();

    // Reload page to verify the name persisted
    await page.reload();
    await spaceAdminPage.expectName(newName);
  });

  test('space name with leading whitespace shows validation error', async ({ spaceAdminPage }) => {
    const { page } = spaceAdminPage;

    // Create user and space
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page, { name: 'Whitespace Test' });

    // Navigate to General settings page
    await spaceAdminPage.gotoGeneralDirectly(space.id);

    // Type a name with leading whitespace
    await spaceAdminPage.setName(' Leading Space');

    // Should show validation error
    await spaceAdminPage.expectValidationError(
      'Name cannot have leading or trailing whitespace'
    );

    // Save button should be disabled
    await spaceAdminPage.expectSaveDisabled();
  });

  test('space name with trailing whitespace shows validation error', async ({ spaceAdminPage }) => {
    const { page } = spaceAdminPage;

    // Create user and space
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page, { name: 'Whitespace Test' });

    // Navigate to General settings page
    await spaceAdminPage.gotoGeneralDirectly(space.id);

    // Type a name with trailing whitespace
    await spaceAdminPage.setName('Trailing Space ');

    // Should show validation error
    await spaceAdminPage.expectValidationError(
      'Name cannot have leading or trailing whitespace'
    );

    // Save button should be disabled
    await spaceAdminPage.expectSaveDisabled();
  });

  test('admin link only visible for space admins', async ({ spaceAdminPage }) => {
    const { page } = spaceAdminPage;

    // Create first user (space creator/admin)
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page, { name: 'Settings Link Test' });

    // Navigate to space - admin should see admin link in sidebar
    await gotoSpace(page, space.id);
    await spaceAdminPage.expectAdminLinkVisible();

    // Create second user (non-admin)
    const nonAdmin = await createSecondTestUser(page);

    // Log out admin and log in as non-admin
    await logoutUser(page);
    await loginUser(page, nonAdmin.login, nonAdmin.password);

    // Join the space as non-admin
    await joinSpaceViaAPI(page, space.id);

    // Navigate to space - non-admin should NOT see admin link
    await gotoSpace(page, space.id);

    // Wait for the page to load (space name should be visible)
    await expect(page.getByRole('heading', { name: space.name })).toBeVisible();

    // Settings link should not be visible
    await spaceAdminPage.expectAdminLinkNotVisible();
  });

  test('space admin can upload and remove a logo', async ({ spaceAdminPage }) => {
    const { page } = spaceAdminPage;

    // Create user and space
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page, { name: 'Logo Upload Test' });

    // Navigate to General settings page
    await spaceAdminPage.gotoGeneralDirectly(space.id);

    // Should see Logo section
    await spaceAdminPage.expectLogoSectionVisible();
    await spaceAdminPage.expectUploadLogoButtonVisible();

    // Create a minimal valid 1x1 red PNG for testing
    const pngData = Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==',
      'base64'
    );

    // Upload logo
    await spaceAdminPage.uploadLogo(pngData, 'test-logo.png');

    // Should see success toast
    await spaceAdminPage.expectToast('Logo uploaded successfully', 15000);

    // Button text should change to "Change Logo"
    await spaceAdminPage.expectChangeLogoButtonVisible();

    // Remove button should now be visible
    await spaceAdminPage.expectRemoveLogoButtonVisible();

    // Logo image should be displayed in the preview
    await spaceAdminPage.expectLogoPreviewVisible();

    // Remove the logo
    await spaceAdminPage.removeLogo();

    // Should see success toast
    await spaceAdminPage.expectToast('Logo removed');

    // Button should go back to "Upload Logo"
    await spaceAdminPage.expectUploadLogoButtonVisible();

    // Remove button should no longer be visible
    await spaceAdminPage.expectRemoveLogoButtonNotVisible();
  });

  test('space admin can upload banner and it updates in sidebar immediately', async ({
    spaceAdminPage
  }) => {
    const { page } = spaceAdminPage;

    // Create user and space
    await createAndLoginTestUser(page);
    const space = await createSpaceViaAPI(page, { name: 'Banner Realtime Test' });

    // Navigate to General settings page
    await spaceAdminPage.gotoGeneralDirectly(space.id);

    // Should see Banner section
    await spaceAdminPage.expectBannerSectionVisible();
    await spaceAdminPage.expectUploadBannerButtonVisible();

    // Banner should NOT be visible in sidebar initially
    await spaceAdminPage.expectSidebarBannerNotVisible();

    // Create a minimal valid 1x1 red PNG for testing
    const pngData = Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==',
      'base64'
    );

    // Upload banner
    await spaceAdminPage.uploadBanner(pngData, 'test-banner.png');

    // Should see success toast
    await spaceAdminPage.expectToast('Banner uploaded successfully', 15000);

    // Button text should change to "Change Banner"
    await spaceAdminPage.expectChangeBannerButtonVisible();

    // Remove button should now be visible
    await spaceAdminPage.expectRemoveBannerButtonVisible();

    // Banner image should be displayed in the preview
    await spaceAdminPage.expectBannerPreviewVisible();

    // CRITICAL: Banner should now be visible in sidebar WITHOUT page reload
    // This tests that the ServerUpdatedEvent is being received and processed
    await spaceAdminPage.expectSidebarBannerVisible();

    // Remove the banner
    await spaceAdminPage.removeBanner();

    // Should see success toast
    await spaceAdminPage.expectToast('Banner removed');

    // Button should go back to "Upload Banner"
    await spaceAdminPage.expectUploadBannerButtonVisible();

    // Remove button should no longer be visible
    await spaceAdminPage.expectRemoveBannerButtonNotVisible();

    // Banner should no longer be visible in sidebar
    await spaceAdminPage.expectSidebarBannerNotVisible();
  });

});
