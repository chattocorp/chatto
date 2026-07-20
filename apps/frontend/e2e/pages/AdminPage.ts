import { expect, type Locator, type Page } from '@playwright/test';
import { TIMEOUTS } from '../constants';
import * as routes from '../routes';

/**
 * Page object for the unified manage/server interface (/chat/-/manage/server).
 * Handles admin navigation, general settings, members, system, runtime, and permissions pages.
 *
 * The legacy /chat/-/admin route tree was folded into manage/server once the
 * old instance-vs-server admin split collapsed; the back-compat aliases in
 * routes.ts make older test names continue to point at the new URLs.
 */
export class AdminPage {
  constructor(readonly page: Page) {}

  // --- Locators ---

  /** The sidebar navigation container (inside ServerSidebar component) */
  get sidebar(): Locator {
    return this.page.locator('nav').first();
  }

  /** The main content area (flex-1 div containing the page content) */
  get mainContent(): Locator {
    // Target the main content area by looking for the flex container after the sidebar
    return this.page.locator('div.flex-1.flex-col').first();
  }

  /** Gear entry point in the server header. */
  get adminGearLink(): Locator {
    return this.page.getByRole('link', { name: 'Server administration' });
  }

  /** General settings link inside the dedicated admin sidebar. */
  get generalLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'General' });
  }

  /** Normal chat overview link in the main server sidebar. */
  get overviewLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'Overview' });
  }

  /** Members link inside the dedicated admin sidebar. */
  get usersLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'Members' });
  }

  /** Rooms link inside the dedicated admin sidebar. */
  get roomsLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'Rooms' });
  }

  /** System link inside the dedicated admin sidebar. */
  get systemLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'System' });
  }

  /** Permissions link inside the dedicated admin sidebar. */
  get rolesLink(): Locator {
    return this.sidebar.getByRole('link', { name: 'Permissions' });
  }

  /** Back-to-chat affordance in the admin sidebar header. */
  get backToChatLink(): Locator {
    return this.page.getByRole('link', { name: 'Back to Server' });
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
    return this.page.getByRole('link', { name: /return to (chat|space|server)/i });
  }

  // --- Navigation Methods ---

  /**
   * Navigate to the default admin destination.
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
   * Navigate to the admin system page.
   */
  async gotoSystem(): Promise<void> {
    await this.page.goto(routes.adminSystem);
  }

  /**
   * Navigate to the admin permissions page.
   */
  async gotoRoles(): Promise<void> {
    await this.page.goto(routes.adminRoles);
  }

  /**
   * Navigate to the instance settings page. Post-merge, the legacy
   * /admin/settings/instance is split across /manage/server/general (name,
   * description, motd, welcome) and /manage/server/security (blocked
   * usernames). Default to /general — the smart fill/expect methods
   * below switch to /security as needed.
   */
  async gotoServerSettings(): Promise<void> {
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

  async navigateToGeneral(): Promise<void> {
    await this.generalLink.click();
    await this.page.waitForURL(routes.admin);
  }

  async navigateToUsers(): Promise<void> {
    await this.usersLink.click();
    await this.page.waitForURL(routes.adminUsers);
  }

  async navigateToSystem(): Promise<void> {
    await this.systemLink.click();
    await this.page.waitForURL(routes.adminSystem);
  }

  async navigateToAdminViaGear(): Promise<void> {
    await this.adminGearLink.click();
    await this.page.waitForURL(routes.admin);
  }

  async navigateBackToChat(): Promise<void> {
    await this.backToChatLink.click();
    // The /chat page may redirect to the overview, the last visited room,
    // or another chat page. Accept any non-admin chat route.
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
    return this.getRoleOption(roleDisplayName).getByRole('checkbox');
  }

  /** Get the visible option label that owns the role checkbox hit area. */
  getRoleOption(roleDisplayName: string): Locator {
    return this.page.locator('label').filter({ hasText: roleDisplayName });
  }

  /**
   * Assign a role to the current user.
   */
  async assignRole(roleDisplayName: string): Promise<void> {
    const checkbox = this.getRoleCheckbox(roleDisplayName);
    await this.getRoleOption(roleDisplayName).click();
    await expect(checkbox).toBeChecked({ timeout: TIMEOUTS.UI_STANDARD });
  }

  /**
   * Revoke a role from the current user.
   */
  async revokeRole(roleDisplayName: string): Promise<void> {
    const checkbox = this.getRoleCheckbox(roleDisplayName);
    await this.getRoleOption(roleDisplayName).click();
    await expect(checkbox).not.toBeChecked({ timeout: TIMEOUTS.UI_STANDARD });
  }

  // --- Permissions Page ---

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
   * Assert that the General settings page is visible.
   */
  async expectGeneralPageVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'General', level: 1 })).toBeVisible();
  }

  /**
   * Assert that the members page is visible. (Was: "Users" before the
   * instance-admin → manage/server merge.)
   */
  async expectUsersPageVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'Members' })).toBeVisible();
  }

  /**
   * Assert that the system page is visible.
   */
  async expectSystemPageVisible(): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'System' })).toBeVisible();
    await expect(
      this.page.getByText('Broker, JetStream storage, consumers, and projection health')
    ).toBeVisible();
  }

  /**
   * Assert that the permissions page is visible.
   */
  async expectRolesPageVisible(): Promise<void> {
    await expect(
      this.page.getByRole('heading', { name: 'Permissions', exact: true, level: 1 })
    ).toBeVisible();
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
   * Assert that access is denied. The merged manage/server layout shows a
   * generic "You do not have permission..." message rather than naming the
   * specific permission like the legacy /admin layout did, so this method
   * now ignores the permission argument — kept for back-compat.
   */
  async expectAccessDeniedForPermission(_permission: string): Promise<void> {
    await expect(this.accessDeniedMessage).toBeVisible();
  }

  /** Assert that the dedicated manage/server sidebar exposes admin navigation. */
  async expectSidebarNavVisible(): Promise<void> {
    await expect(this.generalLink).toBeVisible();
    await expect(this.usersLink).toBeVisible();
    await expect(this.systemLink).toBeVisible();
  }

  async expectAdminGearVisible(): Promise<void> {
    await expect(this.adminGearLink).toBeVisible();
  }

  async expectAdminGearNotVisible(): Promise<void> {
    await expect(this.adminGearLink).not.toBeVisible();
  }

  /**
   * Assert that a sidebar link is NOT visible (limited permissions).
   */
  async expectSidebarLinkNotVisible(
    linkName: 'General' | 'Users' | 'Rooms' | 'System' | 'Roles' | 'Permissions'
  ): Promise<void> {
    const linkMap = {
      General: this.generalLink,
      Users: this.usersLink,
      Rooms: this.roomsLink,
      System: this.systemLink,
      Roles: this.rolesLink,
      Permissions: this.rolesLink
    };
    await expect(linkMap[linkName]).not.toBeVisible();
  }

  /**
   * Assert that a sidebar link IS visible.
   */
  async expectSidebarLinkVisible(
    linkName: 'General' | 'Users' | 'Rooms' | 'System' | 'Roles' | 'Permissions'
  ): Promise<void> {
    const linkMap = {
      General: this.generalLink,
      Users: this.usersLink,
      Rooms: this.roomsLink,
      System: this.systemLink,
      Roles: this.rolesLink,
      Permissions: this.rolesLink
    };
    await expect(linkMap[linkName]).toBeVisible();
  }

  async expectSidebarLinkActive(
    linkName: 'General' | 'Users' | 'Rooms' | 'System' | 'Roles' | 'Permissions'
  ): Promise<void> {
    const linkMap = {
      General: this.generalLink,
      Users: this.usersLink,
      Rooms: this.roomsLink,
      System: this.systemLink,
      Roles: this.rolesLink,
      Permissions: this.rolesLink
    };
    await expect(linkMap[linkName]).toHaveAttribute('aria-current', 'page');
  }

  /** Assert that the normal server sidebar offers a way back to chat. */
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
   * Assert that system connection status is connected.
   */
  async expectSystemConnected(): Promise<void> {
    await expect(this.page.getByText('NATS Broker')).toBeVisible();
    await expect(this.page.getByText('Connected')).toBeVisible({
      timeout: TIMEOUTS.REALTIME_EVENT
    });
  }

  /**
   * Assert that system stat cards are visible.
   */
  async expectSystemStatsVisible(): Promise<void> {
    await expect(this.mainContent.getByText('JetStream Account')).toBeVisible();
    await expect(this.mainContent.getByText('File Storage')).toBeVisible();
    await expect(this.mainContent.getByText('Memory Storage')).toBeVisible();
    await expect(this.mainContent.getByText('Stream Capacity')).toBeVisible();
    await expect(this.mainContent.getByText('Consumer Capacity')).toBeVisible();
    await expect(this.mainContent.getByText('Stream Activity')).toBeVisible();
    await expect(this.mainContent.getByText('Stream Summary')).toBeVisible();
    await expect(this.mainContent.getByText('Consumer Summary')).toBeVisible();
    await expect(this.mainContent.getByText('Projection Summary')).toBeVisible();
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
    await expect(this.page.getByText(/all.*members.*everyone.*role/i)).toBeVisible();
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

  /** Instance Name input — lives on /manage/server/general (label "Name", suffixed
   * with the required-marker asterisk so the accessible name is "Name*"). */
  get instanceNameInput(): Locator {
    return this.page.getByLabel(/^Name\*?$/);
  }

  /** MOTD input — on /manage/server/general. */
  get motdInput(): Locator {
    return this.page.getByLabel('Message of the Day');
  }

  /** Welcome Message textarea — on /manage/server/general. */
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
   * Fill manage/server settings on /general. serverName, description,
   * motd, and welcomeMessage all live in one ServerSettings form now;
   * a single "Save Changes" click persists everything via Mutation.updateInstance.
   */
  async fillServerSettings(options: {
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
   * Save the active manage/server form. Kept as a no-op for back-compat —
   * fillServerSettings now persists each field group as it goes.
   */
  async saveServerSettings(): Promise<void> {
    // No-op — fills auto-save.
  }

  /**
   * Assert that the manage/server settings landing page (General) is visible.
   * The page-level H1 ("General") and a FormSection H2 ("General") share the
   * label, so scope to the page header explicitly.
   */
  async expectServerSettingsVisible(): Promise<void> {
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
   * /manage/server/general (Messages panel).
   */
  async expectMotd(value: string): Promise<void> {
    await this.ensureOn(routes.serverAdminGeneral);
    await expect(this.motdInput).toHaveValue(value);
  }

  /**
   * Assert that the welcome message input has a specific value. The field
   * lives on /manage/server/general (Messages panel).
   */
  async expectWelcomeMessage(value: string): Promise<void> {
    await this.ensureOn(routes.serverAdminGeneral);
    await expect(this.welcomeMessageInput).toHaveValue(value);
  }
}
