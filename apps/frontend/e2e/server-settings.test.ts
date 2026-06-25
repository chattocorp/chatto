import { expect, type Page } from '@playwright/test';
import { test } from './setup';
import {
  createAndLoginTestUser,
  logoutCurrentUser,
  loginAsAdminAndUsePrimaryServer,
  type TestUser
} from './fixtures/testUser';
import * as routes from './routes';

interface TestServer {
  id: string;
  name: string;
}

/** Log in as the bootstrap admin and return the primary server metadata. */
async function usePrimaryServerViaAPI(
  page: Page,
  _options?: { name?: string }
): Promise<TestServer> {
  return loginAsAdminAndUsePrimaryServer(page);
}

/**
 * Creates a second test user (different from the server admin).
 * The user has a verified email so account setup mirrors a real member.
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
  await logoutCurrentUser(page);
}

/**
 * Navigates to the primary server root.
 */
async function gotoServer(page: Page): Promise<void> {
  await page.goto(routes.space());
}

test.describe('Server Admin Page', () => {
  test('server admin can access admin page', async ({ serverAdminPage }) => {
    const { page } = serverAdminPage;

    // Create user and load the primary server
    await createAndLoginTestUser(page);
    const server = await usePrimaryServerViaAPI(page, { name: 'Admin Settings Test' });

    // Navigate to server admin General settings
    await serverAdminPage.gotoGeneralDirectly(server.id);

    // Should see the form with server name
    await expect(serverAdminPage.nameInput).toBeVisible();
    await serverAdminPage.expectName(server.name);
  });

  test('server admin can edit name and save changes', async ({ serverAdminPage }) => {
    const { page } = serverAdminPage;

    // Create user and load the primary server
    await createAndLoginTestUser(page);
    const server = await usePrimaryServerViaAPI(page, { name: 'Original Name' });

    // Navigate to General settings page
    await serverAdminPage.gotoGeneralDirectly(server.id);

    // Change the name
    const newName = `Updated Name ${Date.now()}`;
    await serverAdminPage.updateName(newName);

    // Should see success message
    await serverAdminPage.expectSaveSuccess();

    // Reload page to verify the name persisted
    await page.reload();
    await serverAdminPage.expectName(newName);
  });

  test('server name with leading whitespace shows validation error', async ({
    serverAdminPage
  }) => {
    const { page } = serverAdminPage;

    // Create user and load the primary server
    await createAndLoginTestUser(page);
    const server = await usePrimaryServerViaAPI(page, { name: 'Whitespace Test' });

    // Navigate to General settings page
    await serverAdminPage.gotoGeneralDirectly(server.id);

    // Type a name with leading whitespace
    await serverAdminPage.setName(' Leading Space');

    // Should show validation error
    await serverAdminPage.expectValidationError('Name cannot have leading or trailing whitespace');

    // Save button should be disabled
    await serverAdminPage.expectSaveDisabled();
  });

  test('server name with trailing whitespace shows validation error', async ({
    serverAdminPage
  }) => {
    const { page } = serverAdminPage;

    // Create user and load the primary server
    await createAndLoginTestUser(page);
    const server = await usePrimaryServerViaAPI(page, { name: 'Whitespace Test' });

    // Navigate to General settings page
    await serverAdminPage.gotoGeneralDirectly(server.id);

    // Type a name with trailing whitespace
    await serverAdminPage.setName('Trailing Space ');

    // Should show validation error
    await serverAdminPage.expectValidationError('Name cannot have leading or trailing whitespace');

    // Save button should be disabled
    await serverAdminPage.expectSaveDisabled();
  });

  test('admin link only visible for server admins', async ({ serverAdminPage }) => {
    const { page } = serverAdminPage;

    // Create first user (server admin)
    await createAndLoginTestUser(page);
    const server = await usePrimaryServerViaAPI(page, { name: 'Settings Link Test' });

    // Navigate to server - admin should see admin link in sidebar
    await gotoServer(page);
    await serverAdminPage.expectAdminLinkVisible();

    // Create second user (non-admin)
    const nonAdmin = await createSecondTestUser(page);

    // Log out admin and log in as non-admin
    await logoutUser(page);
    await loginUser(page, nonAdmin.login, nonAdmin.password);

    // Navigate to server - non-admin should NOT see admin link
    await gotoServer(page);

    // Wait for the page to load (server name should be visible)
    await expect(page.getByRole('heading', { name: server.name })).toBeVisible();

    // Settings link should not be visible
    await serverAdminPage.expectAdminLinkNotVisible();
  });

  test('server admin can upload and remove a logo', async ({ serverAdminPage }) => {
    const { page } = serverAdminPage;

    // Create user and load the primary server
    await createAndLoginTestUser(page);
    const server = await usePrimaryServerViaAPI(page, { name: 'Logo Upload Test' });

    // Navigate to General settings page
    await serverAdminPage.gotoGeneralDirectly(server.id);

    // Should see Logo section
    await serverAdminPage.expectLogoSectionVisible();
    await serverAdminPage.expectUploadLogoButtonVisible();

    // Create a minimal valid 1x1 red PNG for testing
    const pngData = Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==',
      'base64'
    );

    // Upload logo
    await serverAdminPage.uploadLogo(pngData, 'test-logo.png');

    // Should see success toast
    await serverAdminPage.expectToast('Logo uploaded successfully', 15000);

    // Button text should change to "Change Logo"
    await serverAdminPage.expectChangeLogoButtonVisible();

    // Remove button should now be visible
    await serverAdminPage.expectRemoveLogoButtonVisible();

    // Logo image should be displayed in the preview
    await serverAdminPage.expectLogoPreviewVisible();

    // Remove the logo
    await serverAdminPage.removeLogo();

    // Should see success toast
    await serverAdminPage.expectToast('Logo removed');

    // Button should go back to "Upload Logo"
    await serverAdminPage.expectUploadLogoButtonVisible();

    // Remove button should no longer be visible
    await serverAdminPage.expectRemoveLogoButtonNotVisible();
  });

  test('server admin can upload banner and it updates in sidebar immediately', async ({
    serverAdminPage
  }) => {
    const { page } = serverAdminPage;

    // Create user and load the primary server
    await createAndLoginTestUser(page);
    const server = await usePrimaryServerViaAPI(page, { name: 'Banner Realtime Test' });

    // Navigate to General settings page
    await serverAdminPage.gotoGeneralDirectly(server.id);

    // Should see Banner section
    await serverAdminPage.expectBannerSectionVisible();
    await serverAdminPage.expectUploadBannerButtonVisible();

    // Banner should NOT be visible in sidebar initially
    await serverAdminPage.expectSidebarBannerNotVisible();

    // Create a minimal valid 1x1 red PNG for testing
    const pngData = Buffer.from(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFBQIAX8jx0gAAAABJRU5ErkJggg==',
      'base64'
    );

    // Upload banner
    await serverAdminPage.uploadBanner(pngData, 'test-banner.png');

    // Should see success toast
    await serverAdminPage.expectToast('Banner uploaded successfully', 15000);

    // Button text should change to "Change Banner"
    await serverAdminPage.expectChangeBannerButtonVisible();

    // Remove button should now be visible
    await serverAdminPage.expectRemoveBannerButtonVisible();

    // Banner image should be displayed in the preview
    await serverAdminPage.expectBannerPreviewVisible();

    // CRITICAL: Banner should now be visible in sidebar WITHOUT page reload
    // This tests that the ServerUpdatedEvent is being received and processed
    await serverAdminPage.expectSidebarBannerVisible();

    // Remove the banner
    await serverAdminPage.removeBanner();

    // Should see success toast
    await serverAdminPage.expectToast('Banner removed');

    // Button should go back to "Upload Banner"
    await serverAdminPage.expectUploadBannerButtonVisible();

    // Remove button should no longer be visible
    await serverAdminPage.expectRemoveBannerButtonNotVisible();

    // Banner should no longer be visible in sidebar
    await serverAdminPage.expectSidebarBannerNotVisible();
  });
});
