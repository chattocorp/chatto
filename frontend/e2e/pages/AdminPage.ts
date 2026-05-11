import { expect, type Locator, type Page } from '@playwright/test';
import { TIMEOUTS } from '../constants';
import * as routes from '../routes';

/**
 * Page object for the unified server-admin interface (/chat/-/server-admin).
 * Handles admin navigation, dashboard, members, system, runtime, and roles pages.
 *
 * The legacy /chat/-/admin route tree was folded into server-admin once the
 * "instance" vs "space" distinction collapsed; the back-compat aliases in
 * routes.ts make older test names continue to point at the new URLs.
 */
export class AdminPage {
  constructor(readonly page: Page) {}

  // --- Locators ---

  /** The sidebar navigation container (inside SecondarySidebar component) */
  get sidebar(): Locator {
    return this.page.locator('nav').first();
  }

  /** The main content area (flex-1 div containing the page content) */
  get mainContent(): Locator {
    // Target the main content area by looking for the flex container after the sidebar
    return this.page.locator('div.flex-1.flex-col').first();
  }

  /** Dashboard link in sidebar */
  get dashboardLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'Dashboard' });
  }

  /** Members link in sidebar. */
  get usersLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'Members' });
  }

  /** Back-compat alias for tests that pre-date the rename. */
  get spacesLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'Members' });
  }

  /** System link in sidebar */
  get systemLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'System' });
  }

  /** Roles link in sidebar */
  get rolesLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'Roles' });
  }

  /** Back-to-chat link (chrome admin nav uses the label "Back to Server"). */
  get backToChatLink(): Locator {
    return this.page.getByRole('link', { name: /back to (chat|space|server)/i });
  }

  /** Access denied message */
  get accessDeniedMessage(): Locator {
    return this.page.getByText('Access Denied', { exact: true });
  }

  /**
   * Return-to-chat link on the access-denied page. Post-merge the chrome
   * AccessDenied uses the label "Return to Server".
   */
  get returnToChatLink(): Locator {
    return this.page.getByRole('link', { name: /return to (chat|space|server|dashboard)/i });
  }

  // --- Navigation Methods ---

  /**
   * Navigate to the admin dashboard.
   */
  async goto(): Promise<void> {
    await this.page.goto(routes.admin);
  }

  /**
   * Navigate to the admin users page.
   */
  async gotoUsers(): Promise<void> {
    await this.page.goto(routes.adminUsers);
  }

  /**
   * Navigate to the admin spaces page.
   */
  async gotoSpaces(): Promise<void> {
    await this.page.goto(routes.adminSpaces);
  }

  /**
   * Navigate to the admin system page.
   */
  async gotoSystem(): Promise<void> {
    await this.page.goto(routes.adminSystem);
  }

  /**
   * Navigate to the admin roles page.
   */
  async gotoRoles(): Promise<void> {
    await this.page.goto(routes.adminRoles);
  }

  /**
   * Navigate to the instance settings page. Post-merge, the legacy
   * /admin/settings/instance is split across /server-admin/general (name,
   * description, motd, welcome) and /server-admin/security (blocked
   * usernames). Default to /general — the smart fill/expect methods
   * below switch to /security as needed.
   */
  async gotoInstanceSettings(): Promise<void> {
    await this.page.goto(routes.serverAdminGeneral);
  }

  async gotoServerAdminGeneral(): Promise<void> {
    await this.page.goto(routes.serverAdminGeneral);
  }

  async gotoServerAdminSecurity(): Promise<void> {
    await this.page.goto(routes.serverAdminSecurity);
  }

  /**
   * Navigate to a specific user's management page.
   */
  async gotoUserManagement(userId: string): Promise<void> {
    await this.page.goto(routes.adminUser(userId));
  }

  /**
   * Navigate to a specific role's page.
   */
  async gotoRole(roleName: string): Promise<void> {
    await this.page.goto(routes.adminRole(roleName));
  }

  /**
   * Navigate using sidebar links.
   */
  async navigateToDashboard(): Promise<void> {
    await this.dashboardLink.click();
    await this.page.waitForURL(routes.admin);
  }

  async navigateToUsers(): Promise<void> {
    await this.usersLink.click();
    await this.page.waitForURL(routes.adminUsers);
  }

  async navigateToSpaces(): Promise<void> {
    await this.spacesLink.click();
    await this.page.waitForURL(routes.adminSpaces);
  }

  async navigateToSystem(): Promise<void> {
    await this.systemLink.click();
    await this.page.waitForURL(routes.adminSystem);
  }

  async navigateBackToChat(): Promise<void> {
    await this.backToChatLink.click();
    // The /chat page may redirect to /chat/spaces for users with no joined spaces,
    // or to their last visited space. Accept any non-admin chat route.
    await this.page.waitForURL(routes.patterns.nonAdmin);
  }

  // --- Users Page ---

  /**
   * Get a user row by login name.
   */
  getUserRow(login: string): Locator {
    return this.page.locator('tr').filter({ hasText: login });
  }

  /**
   * Click on a user row to navigate to their management page.
   */
  async clickUser(login: string): Promise<void> {
    await this.getUserRow(login).click();
    await expect(this.page).toHaveURL(routes.patterns.anyAdminUser);
  }

  // --- User Management Page ---

  /**
   * Find a permission row in the user management page.
   * Permission rows are div containers with the permission name and Grant/Deny checkboxes.
   */
  getPermissionRow(permission: string): Locator {
    // Escape dots for regex and use anchors for exact match
    const escapedPermission = permission.replace(/\./g, '\\.');
    return this.page
      .locator('.font-medium')
      .filter({ hasText: new RegExp(`^${escapedPermission}$`) })
      .locator('xpath=ancestor::div[contains(@class,"rounded-lg")]');
  }

  /**
   * Get the Grant checkbox for a permission.
   */
  getGrantCheckbox(permission: string): Locator {
    return this.getPermissionRow(permission).getByLabel('Grant');
  }

  /**
   * Get the Deny checkbox for a permission.
   */
  getDenyCheckbox(permission: string): Locator {
    return this.getPermissionRow(permission).getByLabel('Deny');
  }

  /**
   * Grant a permission to the current user.
   */
  async grantPermission(permission: string): Promise<void> {
    const checkbox = this.getGrantCheckbox(permission);
    await checkbox.click();
    await expect(checkbox).toBeChecked({ timeout: TIMEOUTS.UI_STANDARD });
  }

  /**
   * Deny a permission for the current user.
   */
  async denyPermission(permission: string): Promise<void> {
    const checkbox = this.getDenyCheckbox(permission);
    await checkbox.click();
    await expect(checkbox).toBeChecked({ timeout: TIMEOUTS.UI_STANDARD });
  }

  /**
   * Clear a permission override by unchecking Grant or Deny.
   */
  async clearGrantOverride(permission: string): Promise<void> {
    const checkbox = this.getGrantCheckbox(permission);
    if (await checkbox.isChecked()) {
      await checkbox.click();
      await expect(checkbox).not.toBeChecked({ timeout: TIMEOUTS.UI_STANDARD });
    }
  }

  async clearDenyOverride(permission: string): Promise<void> {
    const checkbox = this.getDenyCheckbox(permission);
    if (await checkbox.isChecked()) {
      await checkbox.click();
      await expect(checkbox).not.toBeChecked({ timeout: TIMEOUTS.UI_STANDARD });
    }
  }

  /**
   * Get a role checkbox in the Role Assignments section.
   */
  getRoleCheckbox(roleDisplayName: string): Locator {
    return this.page.locator('label').filter({ hasText: roleDisplayName }).getByRole('checkbox');
  }

  /**
   * Assign a role to the current user.
   */
  async assignRole(roleDisplayName: string): Promise<void> {
    const checkbox = this.getRoleCheckbox(roleDisplayName);
    await checkbox.click();
    await expect(checkbox).toBeChecked({ timeout: TIMEOUTS.UI_STANDARD });
  }

  /**
   * Revoke a role from the current user.
   */
  async revokeRole(roleDisplayName: string): Promise<void> {
    const checkbox = this.getRoleCheckbox(roleDisplayName);
    await checkbox.click();
    await expect(checkbox).not.toBeChecked({ timeout: TIMEOUTS.UI_STANDARD });
  }

  // --- Roles Page ---

  /**
   * Get the Create Role button.
   */
  get createRoleButton(): Locator {
    return this.page.getByRole('button', { name: 'Create Role' });
  }

  /**
   * Get an Edit button (first one).
   */
  get editButton(): Locator {
    return this.page.getByRole('button', { name: 'Edit' }).first();
  }

  // --- Assertions ---

  /**
   * Assert that the dashboard page is visible.
   */
  async expectDashboardVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
    await expect(this.page.getByText('Server overview and statistics')).toBeVisible();
  }

  /**
   * Assert that the members page is visible. (Was: "Users" before the
   * instance-admin → server-admin merge.)
   */
  async expectUsersPageVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'Members' })).toBeVisible();
  }

  /**
   * Assert that the spaces page is visible. Spaces no longer exist
   * post-#330; the Members page is the closest equivalent.
   */
  async expectSpacesPageVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'Members' })).toBeVisible();
  }

  /**
   * Assert that the system page is visible.
   */
  async expectSystemPageVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'System' })).toBeVisible();
    await expect(this.page.getByText('NATS/JetStream status and aggregate usage')).toBeVisible();
  }

  /**
   * Assert that the roles page is visible.
   */
  async expectRolesPageVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'Roles', exact: true })).toBeVisible();
  }

  /**
   * Assert that the user management page is visible.
   */
  async expectUserManagementVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'Member Details' })).toBeVisible();
  }

  /**
   * Assert that access is denied.
   */
  async expectAccessDenied(): Promise<void> {
    await expect(this.accessDeniedMessage).toBeVisible();
  }

  /**
   * Assert that access is denied. The merged server-admin layout shows a
   * generic "You do not have permission..." message rather than naming the
   * specific permission like the legacy /admin layout did, so this method
   * now ignores the permission argument — kept for back-compat.
   */
  async expectAccessDeniedForPermission(_permission: string): Promise<void> {
    await expect(this.accessDeniedMessage).toBeVisible();
  }

  /**
   * Assert that the sidebar navigation items are visible.
   */
  async expectSidebarNavVisible(): Promise<void> {
    await expect(this.dashboardLink).toBeVisible();
    await expect(this.usersLink).toBeVisible();
    await expect(this.systemLink).toBeVisible();
  }

  /**
   * Assert that a sidebar link is NOT visible (limited permissions).
   */
  async expectSidebarLinkNotVisible(
    linkName: 'Dashboard' | 'Users' | 'Spaces' | 'System' | 'Roles'
  ): Promise<void> {
    const linkMap = {
      Dashboard: this.dashboardLink,
      Users: this.usersLink,
      Spaces: this.spacesLink,
      System: this.systemLink,
      Roles: this.rolesLink
    };
    await expect(linkMap[linkName]).not.toBeVisible();
  }

  /**
   * Assert that a sidebar link IS visible.
   */
  async expectSidebarLinkVisible(
    linkName: 'Dashboard' | 'Users' | 'Spaces' | 'System' | 'Roles'
  ): Promise<void> {
    const linkMap = {
      Dashboard: this.dashboardLink,
      Users: this.usersLink,
      Spaces: this.spacesLink,
      System: this.systemLink,
      Roles: this.rolesLink
    };
    await expect(linkMap[linkName]).toBeVisible();
  }

  /**
   * Assert that the back to chat link is visible.
   */
  async expectBackToChatVisible(): Promise<void> {
    await expect(this.backToChatLink).toBeVisible();
  }

  /**
   * Assert that the return to chat link is visible (on access denied page).
   */
  async expectReturnToChatVisible(): Promise<void> {
    await expect(this.returnToChatLink).toBeVisible();
  }

  /**
   * Assert dashboard stats are visible.
   */
  async expectDashboardStatsVisible(): Promise<void> {
    await expect(this.mainContent.getByText('Registered Users')).toBeVisible();
  }

  /**
   * Assert that the users table headers are visible. (Was: legacy /admin/users
   * table — Login/Display Name/ID — before instance admin folded into
   * server admin.)
   */
  async expectUsersTableHeadersVisible(): Promise<void> {
    await expect(this.page.getByRole('columnheader', { name: 'User' })).toBeVisible();
    await expect(this.page.getByRole('columnheader', { name: 'Login' })).toBeVisible();
    await expect(this.page.getByRole('columnheader', { name: 'Roles' })).toBeVisible();
  }

  /**
   * Assert the user/member count is visible. The members page shows it
   * implicitly via the table-row count rather than a "N user(s) total"
   * string, so wait for at least one populated row instead.
   */
  async expectUserCountVisible(): Promise<void> {
    await expect(this.page.locator('tbody tr').first()).toBeVisible();
  }

  /**
   * Assert that spaces table headers are visible.
   */
  async expectSpacesTableHeadersVisible(): Promise<void> {
    await expect(this.page.getByRole('columnheader', { name: 'Name' })).toBeVisible();
    await expect(this.page.getByRole('columnheader', { name: 'Description' })).toBeVisible();
    await expect(this.page.getByRole('columnheader', { name: 'Members' })).toBeVisible();
    await expect(this.page.getByRole('columnheader', { name: 'Rooms' })).toBeVisible();
    await expect(this.page.getByRole('columnheader', { name: 'Assets' })).toBeVisible();
  }

  /**
   * Assert that the space count is visible.
   */
  async expectSpaceCountVisible(): Promise<void> {
    await expect(this.page.getByText(/\d+ space\(s\) total/)).toBeVisible();
  }

  /**
   * Assert that system connection status is connected.
   */
  async expectSystemConnected(): Promise<void> {
    await expect(this.page.getByText('Connection')).toBeVisible();
    await expect(this.page.getByText('Connected')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  }

  /**
   * Assert that system stat cards are visible.
   */
  async expectSystemStatsVisible(): Promise<void> {
    await expect(this.mainContent.getByText('Storage')).toBeVisible();
    await expect(this.mainContent.getByText('Memory')).toBeVisible();
    await expect(
      this.mainContent.locator('.text-sm', { hasText: /^Streams$/ }).first()
    ).toBeVisible();
    await expect(
      this.mainContent.locator('.text-sm', { hasText: /^Consumers$/ }).first()
    ).toBeVisible();
  }

  /**
   * Assert that the roles page shows read-only message.
   */
  async expectRolesReadOnlyMessage(): Promise<void> {
    await expect(
      this.page.getByText('You need the admin.manage-roles permission to make changes')
    ).toBeVisible();
  }

  /**
   * Assert that the Create Role button is NOT visible.
   */
  async expectCreateRoleNotVisible(): Promise<void> {
    await expect(this.createRoleButton).not.toBeVisible();
  }

  /**
   * Assert that Edit buttons are NOT visible.
   */
  async expectEditButtonNotVisible(): Promise<void> {
    await expect(this.editButton).not.toBeVisible();
  }

  /**
   * Assert that the Role Assignments section is visible.
   */
  async expectRoleAssignmentsVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'Role Assignments' })).toBeVisible();
  }

  /**
   * Assert that Users with this Role section is visible.
   */
  async expectUsersWithRoleVisible(): Promise<void> {
    await expect(this.page.getByText('Users with this Role')).toBeVisible();
  }

  /**
   * Assert that a user login is visible on the page.
   */
  async expectUserLoginVisible(login: string): Promise<void> {
    await expect(this.page.getByText(login)).toBeVisible();
  }

  /**
   * Assert that a verified email is visible in the users list.
   */
  async expectEmailVisible(email: string): Promise<void> {
    await expect(this.page.getByText(email)).toBeVisible();
  }

  /**
   * Assert that the member role shows the implicit membership message.
   */
  async expectMemberRoleMessage(): Promise<void> {
    await expect(this.page.getByText(/all.*users.*members/i)).toBeVisible();
  }

  /**
   * Assert that the permission row shows a role indicator (checkmark).
   */
  async expectPermissionFromRole(permission: string): Promise<void> {
    const permRow = this.getPermissionRow(permission);
    const rolesIndicator = permRow.locator('.uil--check-circle');
    await expect(rolesIndicator).toBeVisible();
  }

  // --- Instance Settings Page ---

  /** Instance Name input — lives on /server-admin/general (label "Name", suffixed
   * with the required-marker asterisk so the accessible name is "Name*"). */
  get instanceNameInput(): Locator {
    return this.page.getByLabel(/^Name\*?$/);
  }

  /** MOTD input — on /server-admin/general. */
  get motdInput(): Locator {
    return this.page.getByLabel('Message of the Day');
  }

  /** Welcome Message textarea — on /server-admin/general. */
  get welcomeMessageInput(): Locator {
    return this.page.getByLabel('Welcome Message');
  }

  /** Save button — copy varies by page. */
  get saveButton(): Locator {
    return this.page.getByRole('button', { name: 'Save', exact: true });
  }

  /** /general's primary submit button. */
  get saveChangesButton(): Locator {
    return this.page.getByRole('button', { name: 'Save Changes' });
  }

  /**
   * Navigate to the given fully-qualified URL if not already there. Used by
   * the smart fill/expect helpers below.
   */
  private async ensureOn(routeUrl: string): Promise<void> {
    if (!this.page.url().includes(routeUrl)) {
      await this.page.goto(routeUrl);
    }
  }

  /**
   * Fill server-admin settings on /general. serverName, description,
   * motd, and welcomeMessage all live in one InstanceSettings form now;
   * a single "Save Changes" click persists everything via Mutation.updateInstance.
   */
  async fillInstanceSettings(options: {
    serverName?: string;
    motd?: string;
    welcomeMessage?: string;
  }): Promise<void> {
    await this.ensureOn(routes.serverAdminGeneral);

    let dirty = false;
    if (options.serverName !== undefined) {
      await this.instanceNameInput.fill(options.serverName);
      dirty = true;
    }
    if (options.motd !== undefined) {
      await this.motdInput.fill(options.motd);
      dirty = true;
    }
    if (options.welcomeMessage !== undefined) {
      await this.welcomeMessageInput.fill(options.welcomeMessage);
      dirty = true;
    }
    if (dirty) {
      await this.saveChangesButton.click();
      await expect(this.page.getByText('Saved!')).toBeVisible({
        timeout: TIMEOUTS.UI_STANDARD
      });
    }
  }

  /**
   * Save the active server-admin form. Kept as a no-op for back-compat —
   * fillInstanceSettings now persists each field group as it goes.
   */
  async saveInstanceSettings(): Promise<void> {
    // No-op — fills auto-save.
  }

  /**
   * Assert that the server-admin settings landing page (General) is visible.
   * The page-level H1 ("General") and a FormSection H2 ("General") share the
   * label, so scope to the page header explicitly.
   */
  async expectInstanceSettingsVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'General', level: 1 })).toBeVisible();
  }

  /**
   * Assert that the instance name input has a specific value. Navigates to
   * /general first since that's where the field lives.
   */
  async expectInstanceName(value: string): Promise<void> {
    await this.ensureOn(routes.serverAdminGeneral);
    await expect(this.instanceNameInput).toHaveValue(value);
  }

  /**
   * Assert that the MOTD input has a specific value. The field lives on
   * /server-admin/general (Messages panel).
   */
  async expectMotd(value: string): Promise<void> {
    await this.ensureOn(routes.serverAdminGeneral);
    await expect(this.motdInput).toHaveValue(value);
  }

  /**
   * Assert that the welcome message input has a specific value. The field
   * lives on /server-admin/general (Messages panel).
   */
  async expectWelcomeMessage(value: string): Promise<void> {
    await this.ensureOn(routes.serverAdminGeneral);
    await expect(this.welcomeMessageInput).toHaveValue(value);
  }

}
