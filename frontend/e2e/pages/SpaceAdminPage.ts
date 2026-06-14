import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';

/**
 * Page object for the Space Admin pages.
 * Covers General (name, branding) and Permissions pages.
 */
export class SpaceAdminPage {
  constructor(readonly page: Page) {}

  // --- Locators ---

  /** The server-admin gear link in the server header. */
  get adminLink(): Locator {
    return this.page.getByRole('link', { name: 'Server administration' });
  }

  /** Dedicated server-admin sidebar link container. */
  get adminLinks(): Locator {
    return this.page.locator('nav').first();
  }

  /** Back-compat default admin nav item. */
  get homeNavItem(): Locator {
    return this.generalNavItem;
  }

  /** Sidebar navigation item for General settings */
  get generalNavItem(): Locator {
    return this.adminLinks.getByRole('link', { name: 'General', exact: true });
  }

  /** Sidebar navigation item for Rooms settings */
  get roomsNavItem(): Locator {
    return this.adminLinks.getByRole('link', { name: 'Rooms', exact: true });
  }

  /** Sidebar navigation item for Permissions settings */
  get rolesNavItem(): Locator {
    return this.adminLinks.getByRole('link', { name: 'Permissions', exact: true });
  }

  /** Sidebar navigation item for the Members settings page. */
  get membersNavItem(): Locator {
    return this.adminLinks.getByRole('link', { name: 'Members', exact: true });
  }

  /** Access Denied heading */
  get accessDeniedHeading(): Locator {
    return this.page.getByText('Access Denied', { exact: true });
  }

  /** General settings heading (shown when user has space.manage permission) */
  get generalSettingsHeading(): Locator {
    // Use h1 specifically to avoid matching section h2 headings
    return this.page.locator('h1', { hasText: 'General' });
  }

  /** Legacy back-to-space locator from the retired admin-only sidebar. */
  get backToSpaceLink(): Locator {
    return this.page.getByRole('link', { name: 'Back to Space' });
  }

  /** The space name input field */
  get nameInput(): Locator {
    return this.page.getByRole('textbox', { name: 'Name' });
  }

  /** The Save Changes button */
  get saveButton(): Locator {
    return this.page.getByRole('button', { name: 'Save Changes' });
  }

  /** The Upload Logo button */
  get uploadLogoButton(): Locator {
    return this.page.getByRole('button', { name: 'Upload Logo' });
  }

  /** The Change Logo button (shown when logo exists) */
  get changeLogoButton(): Locator {
    return this.page.getByRole('button', { name: 'Change Logo' });
  }

  /** The Remove logo button */
  get removeLogoButton(): Locator {
    return this.page.getByRole('button', { name: 'Remove' });
  }

  /** The logo preview image */
  get logoPreview(): Locator {
    // Panel uses div structure with h2 heading, not section
    return this.page.locator('div:has(h2:has-text("Logo")) img[alt="Server logo"]');
  }

  /** The Logo section heading */
  get logoHeading(): Locator {
    return this.page.getByRole('heading', { name: 'Logo', exact: true });
  }

  /** The Upload Banner button */
  get uploadBannerButton(): Locator {
    return this.page.getByRole('button', { name: 'Upload Banner' });
  }

  /** The Change Banner button (shown when banner exists) */
  get changeBannerButton(): Locator {
    return this.page.getByRole('button', { name: 'Change Banner' });
  }

  /** The Remove banner button */
  get removeBannerButton(): Locator {
    // Panel uses div structure with h2 heading, not section
    return this.page
      .locator('div:has(h2:has-text("Banner"))')
      .getByRole('button', { name: 'Remove' });
  }

  /** The banner preview image in settings */
  get bannerPreview(): Locator {
    return this.page.getByTestId('banner-drop-zone').getByRole('img', { name: 'Server banner' });
  }

  /** The banner image in the sidebar */
  get sidebarBanner(): Locator {
    return this.page.locator('img[alt="Server banner"]').first();
  }

  /** The Banner section heading */
  get bannerHeading(): Locator {
    return this.page.getByRole('heading', { name: 'Banner', exact: true });
  }

  /** The default admin page heading. */
  get pageHeading(): Locator {
    return this.page.getByRole('heading', { name: 'General', level: 1 }).last();
  }

  /** The sidebar heading showing the server name in admin mode. */
  get sidebarHeading(): Locator {
    return this.page.getByRole('heading', { level: 1 }).first();
  }

  // --- Navigation ---

  /**
   * Navigate to a space and then to its admin page via the sidebar link.
   */
  async goto(_spaceId: string): Promise<void> {
    await this.page.goto(routes.space());
    await this.adminLink.click();
    await this.page.waitForURL(routes.serverAdminGeneral);
    await expect(this.pageHeading).toBeVisible();
  }

  /**
   * Navigate directly to the admin page URL.
   */
  async gotoDirectly(spaceId: string): Promise<void> {
    await this.page.goto(routes.serverAdminGeneral);
    await expect(this.pageHeading).toBeVisible();
  }

  /**
   * Click the Admin link in the sidebar (from a space page).
   */
  async clickAdminLink(_spaceId: string): Promise<void> {
    await this.adminLink.click();
    await this.page.waitForURL(routes.serverAdminGeneral);
    await expect(this.pageHeading).toBeVisible();
  }

  // --- Form Interactions ---

  /**
   * Update the space name.
   */
  async setName(name: string): Promise<void> {
    await this.nameInput.fill(name);
  }

  /**
   * Click the Save Changes button.
   */
  async save(): Promise<void> {
    await this.saveButton.click();
  }

  /**
   * Update the space name and save changes.
   */
  async updateName(name: string): Promise<void> {
    await this.setName(name);
    await this.save();
  }

  async expandAdminGroup(): Promise<void> {
    // Compatibility no-op: admin links now live in their own sidebar.
  }

  // --- Logo Interactions ---

  /**
   * Upload a logo image.
   * @param buffer - The image data buffer
   * @param filename - The filename to use (default: 'logo.png')
   * @param mimeType - The MIME type (default: 'image/png')
   */
  async uploadLogo(
    buffer: Buffer,
    filename: string = 'logo.png',
    mimeType: string = 'image/png'
  ): Promise<void> {
    const fileChooserPromise = this.page.waitForEvent('filechooser');
    // Click whichever button is visible (Upload Logo or Change Logo)
    const uploadButton = this.page.getByRole('button', { name: /Upload Logo|Change Logo/ });
    await uploadButton.click();
    const fileChooser = await fileChooserPromise;
    await fileChooser.setFiles({
      name: filename,
      mimeType,
      buffer
    });
  }

  /**
   * Remove the current logo.
   */
  async removeLogo(): Promise<void> {
    await this.removeLogoButton.click();
  }

  // --- Banner Interactions ---

  /**
   * Upload a banner image.
   * @param buffer - The image data buffer
   * @param filename - The filename to use (default: 'banner.png')
   * @param mimeType - The MIME type (default: 'image/png')
   */
  async uploadBanner(
    buffer: Buffer,
    filename: string = 'banner.png',
    mimeType: string = 'image/png'
  ): Promise<void> {
    const fileChooserPromise = this.page.waitForEvent('filechooser');
    // Click whichever button is visible (Upload Banner or Change Banner)
    const uploadButton = this.page.getByRole('button', { name: /Upload Banner|Change Banner/ });
    await uploadButton.click();
    const fileChooser = await fileChooserPromise;
    await fileChooser.setFiles({
      name: filename,
      mimeType,
      buffer
    });
  }

  /**
   * Remove the current banner.
   */
  async removeBanner(): Promise<void> {
    await this.removeBannerButton.click();
  }

  // --- Assertions ---

  /**
   * Assert the admin page is visible.
   */
  async expectVisible(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
  }

  /**
   * Assert the admin link in sidebar is visible.
   */
  async expectAdminLinkVisible(): Promise<void> {
    await expect(this.adminLink).toBeVisible();
  }

  /**
   * Assert the admin link in sidebar is NOT visible.
   */
  async expectAdminLinkNotVisible(): Promise<void> {
    await expect(this.adminLink).not.toBeVisible();
  }

  /**
   * Assert that the name input has the expected value.
   */
  async expectName(name: string): Promise<void> {
    await expect(this.nameInput).toHaveValue(name);
  }

  /**
   * Assert that the Save Changes button is disabled.
   */
  async expectSaveDisabled(): Promise<void> {
    await expect(this.saveButton).toBeDisabled();
  }

  /**
   * Assert that the Save Changes button is enabled.
   */
  async expectSaveEnabled(): Promise<void> {
    await expect(this.saveButton).toBeEnabled();
  }

  /**
   * Assert that a validation error message is visible.
   */
  async expectValidationError(message: string): Promise<void> {
    await expect(this.page.getByText(message)).toBeVisible();
  }

  /**
   * Assert that the "Saved!" success message is visible.
   */
  async expectSaveSuccess(): Promise<void> {
    await expect(this.page.getByText('Saved!')).toBeVisible();
  }

  /**
   * Assert that the Logo section is visible.
   */
  async expectLogoSectionVisible(): Promise<void> {
    await expect(this.logoHeading).toBeVisible();
  }

  /**
   * Assert that the Upload Logo button is visible (no logo uploaded).
   */
  async expectUploadLogoButtonVisible(): Promise<void> {
    await expect(this.uploadLogoButton).toBeVisible();
  }

  /**
   * Assert that the Change Logo button is visible (logo exists).
   */
  async expectChangeLogoButtonVisible(): Promise<void> {
    await expect(this.changeLogoButton).toBeVisible();
  }

  /**
   * Assert that the Remove button is visible.
   */
  async expectRemoveLogoButtonVisible(): Promise<void> {
    await expect(this.removeLogoButton).toBeVisible();
  }

  /**
   * Assert that the Remove button is NOT visible.
   */
  async expectRemoveLogoButtonNotVisible(): Promise<void> {
    await expect(this.removeLogoButton).not.toBeVisible();
  }

  /**
   * Assert that the logo preview image is visible.
   */
  async expectLogoPreviewVisible(): Promise<void> {
    await expect(this.logoPreview).toBeVisible();
  }

  /**
   * Assert that a success toast message is visible.
   */
  async expectToast(message: string, timeout: number = 10000): Promise<void> {
    await expect(this.page.getByText(message)).toBeVisible({ timeout });
  }

  // --- Banner Assertions ---

  /**
   * Assert that the Banner section is visible.
   */
  async expectBannerSectionVisible(): Promise<void> {
    await expect(this.bannerHeading).toBeVisible();
  }

  /**
   * Assert that the Upload Banner button is visible (no banner uploaded).
   */
  async expectUploadBannerButtonVisible(): Promise<void> {
    await expect(this.uploadBannerButton).toBeVisible();
  }

  /**
   * Assert that the Change Banner button is visible (banner exists).
   */
  async expectChangeBannerButtonVisible(): Promise<void> {
    await expect(this.changeBannerButton).toBeVisible();
  }

  /**
   * Assert that the Remove banner button is visible.
   */
  async expectRemoveBannerButtonVisible(): Promise<void> {
    await expect(this.removeBannerButton).toBeVisible();
  }

  /**
   * Assert that the Remove banner button is NOT visible.
   */
  async expectRemoveBannerButtonNotVisible(): Promise<void> {
    await expect(this.removeBannerButton).not.toBeVisible();
  }

  /**
   * Assert that the banner preview image is visible.
   */
  async expectBannerPreviewVisible(): Promise<void> {
    await expect(this.bannerPreview).toBeVisible();
  }

  /**
   * Assert that the banner is visible in the sidebar.
   */
  async expectSidebarBannerVisible(timeout: number = 10000): Promise<void> {
    await expect(this.sidebarBanner).toBeVisible({ timeout });
  }

  /**
   * Assert that the banner is NOT visible in the sidebar.
   */
  async expectSidebarBannerNotVisible(): Promise<void> {
    await expect(this.sidebarBanner).not.toBeVisible();
  }

  // --- Nav Item Visibility Assertions ---

  /**
   * Assert that the default admin nav item is visible in the admin sidebar.
   */
  async expectHomeNavVisible(): Promise<void> {
    await expect(this.homeNavItem).toBeVisible();
  }

  /**
   * Assert that the default admin nav item is NOT visible in the admin sidebar.
   */
  async expectHomeNavNotVisible(): Promise<void> {
    await expect(this.homeNavItem).not.toBeVisible();
  }

  /**
   * Assert that the General nav item is visible in the admin sidebar.
   */
  async expectGeneralNavVisible(): Promise<void> {
    await expect(this.generalNavItem).toBeVisible();
  }

  /**
   * Assert that the General nav item is NOT visible in the admin sidebar.
   */
  async expectGeneralNavNotVisible(): Promise<void> {
    await expect(this.generalNavItem).not.toBeVisible();
  }

  /**
   * Assert that the Members nav item is visible in the admin sidebar.
   */
  async expectMembersNavVisible(): Promise<void> {
    await expect(this.membersNavItem).toBeVisible();
  }

  /**
   * Assert that the Members nav item is NOT visible in the admin sidebar.
   */
  async expectMembersNavNotVisible(): Promise<void> {
    await expect(this.membersNavItem).not.toBeVisible();
  }

  /**
   * Assert that the Roles nav item is visible in the admin sidebar.
   */
  async expectRolesNavVisible(): Promise<void> {
    await expect(this.rolesNavItem).toBeVisible();
  }

  /**
   * Assert that the Roles nav item is NOT visible in the admin sidebar.
   */
  async expectRolesNavNotVisible(): Promise<void> {
    await expect(this.rolesNavItem).not.toBeVisible();
  }

  /**
   * Assert that the Access Denied message is visible.
   */
  async expectAccessDenied(): Promise<void> {
    await expect(this.accessDeniedHeading).toBeVisible();
  }

  /**
   * Assert that the Access Denied message is NOT visible.
   */
  async expectAccessNotDenied(): Promise<void> {
    await expect(this.accessDeniedHeading).not.toBeVisible();
  }

  // --- Direct Navigation Methods ---

  /**
   * Navigate directly to the General admin page URL.
   */
  async gotoGeneralDirectly(spaceId: string): Promise<void> {
    await this.page.goto(routes.serverAdminGeneral);
    await expect(this.generalSettingsHeading).toBeVisible();
  }

  /**
   * Navigate directly to the Members admin page URL.
   */
  async gotoMembersDirectly(spaceId: string): Promise<void> {
    await this.page.goto(routes.serverAdminMembers);
  }

  /**
   * Navigate directly to the Roles admin page URL.
   */
  async gotoRolesDirectly(spaceId: string): Promise<void> {
    await this.page.goto(routes.serverAdminRoles);
  }

  /**
   * Navigate directly to a member's details page.
   */
  async gotoMemberDetails(spaceId: string, userId: string): Promise<void> {
    await this.page.goto(routes.serverAdminMember(userId));
  }

  /**
   * Assert that the General settings page content is visible.
   */
  async expectGeneralSettingsVisible(): Promise<void> {
    await expect(this.generalSettingsHeading).toBeVisible();
  }

  /**
   * Assert that the General settings page content is NOT visible.
   */
  async expectGeneralSettingsNotVisible(): Promise<void> {
    await expect(this.generalSettingsHeading).not.toBeVisible();
  }

  // --- Member Details Page ---

  /** Member Details page heading */
  get memberDetailsHeading(): Locator {
    return this.page.getByRole('heading', { name: 'Member Details' });
  }

  /** User Details panel heading (h2) */
  get userDetailsPanel(): Locator {
    return this.page.locator('h2', { hasText: 'User Details' });
  }

  /** Role Assignments panel heading (h2) */
  get roleAssignmentsPanel(): Locator {
    return this.page.locator('h2', { hasText: 'Role Assignments' });
  }

  /** Back to Members button */
  get backToMembersButton(): Locator {
    return this.page.getByRole('link', { name: 'Back to Members' });
  }

  /**
   * Assert that the Member Details page is visible without errors.
   */
  async expectMemberDetailsVisible(): Promise<void> {
    await expect(this.memberDetailsHeading).toBeVisible({ timeout: 10000 });
    // Wait for loading to complete - the panels appear after data loads
    await expect(this.userDetailsPanel).toBeVisible({ timeout: 10000 });
    await expect(this.roleAssignmentsPanel).toBeVisible({ timeout: 10000 });
  }

  /**
   * Assert that a specific user's details are shown.
   */
  async expectMemberLogin(login: string): Promise<void> {
    await expect(this.page.locator('.font-medium').filter({ hasText: `@${login}` })).toBeVisible();
  }

  /**
   * Assert that a specific display name is shown.
   */
  async expectMemberDisplayName(displayName: string): Promise<void> {
    await expect(this.page.getByText(displayName)).toBeVisible();
  }
}
