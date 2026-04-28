import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';

/**
 * Page object for the Space Roles management pages.
 * Handles viewing, creating, editing, and deleting space roles.
 */
export class SpaceRolesPage {
  constructor(readonly page: Page) {}

  // --- Locators ---

  /** The page heading */
  get pageHeading(): Locator {
    return this.page.getByRole('heading', { name: 'Roles', exact: true });
  }

  /** The Create Role button */
  get createRoleButton(): Locator {
    return this.page.getByRole('button', { name: 'Create Role' });
  }

  /** Sidebar navigation item for General settings */
  get generalNavItem(): Locator {
    return this.page.locator('nav a', { hasText: 'General' });
  }

  /** The roles table — the page renders a single DataTable. */
  get rolesTable(): Locator {
    return this.page.locator('table').first();
  }

  /** The role name input (on create/edit page) */
  get nameInput(): Locator {
    return this.page.getByTestId('role-form-name');
  }

  /** The display name input (on create/edit page) */
  get displayNameInput(): Locator {
    return this.page.getByTestId('role-form-display-name');
  }

  /** The description input (on create/edit page) */
  get descriptionInput(): Locator {
    return this.page.getByTestId('role-form-description');
  }

  /** The submit button on create role form */
  get submitButton(): Locator {
    return this.page.getByRole('button', { name: 'Create Role' });
  }

  /** The save changes button on edit role form */
  get saveChangesButton(): Locator {
    return this.page.getByRole('button', { name: 'Save Changes' });
  }

  /** The delete role button */
  get deleteRoleButton(): Locator {
    return this.page.getByRole('button', { name: 'Delete Role' });
  }

  /** The confirm delete button in the modal */
  get confirmDeleteButton(): Locator {
    return this.page.getByRole('button', { name: 'Delete' }).last();
  }

  /** The cancel button */
  get cancelButton(): Locator {
    return this.page.getByRole('button', { name: 'Cancel' });
  }

  /** The Back to Roles arrow link in the pane header */
  get backToRolesButton(): Locator {
    // PaneHeader's backHref renders an <a> with aria-label="Back to roles".
    return this.page.getByRole('link', { name: 'Back to roles' });
  }

  // --- Navigation ---

  /**
   * Navigate to the space roles list page.
   */
  async gotoRolesList(spaceId: string): Promise<void> {
    await this.page.goto(routes.spaceAdminRoles(spaceId));
    await expect(this.pageHeading).toBeVisible();
  }

  /**
   * Navigate to the create role page.
   */
  async gotoCreateRole(spaceId: string): Promise<void> {
    await this.page.goto(routes.spaceAdminRolesNew(spaceId));
    // Wait for either the form (if user has permission) or Access Denied message
    await expect(
      this.nameInput.or(this.page.getByText('Access Denied', { exact: true }))
    ).toBeVisible();
  }

  /**
   * Navigate to a specific role's edit page.
   */
  async gotoEditRole(spaceId: string, roleName: string): Promise<void> {
    await this.page.goto(routes.spaceAdminRole(spaceId, roleName));
    await expect(this.page.getByRole('heading', { name: 'Edit Role' })).toBeVisible();
  }

  // --- Role List Actions ---

  /**
   * Get a row for a specific role by its display name.
   * Finds a td cell that contains exactly the display name text.
   */
  getRoleRow(displayName: string): Locator {
    // The new DataTable row keeps the display name in a <div class="font-medium">
    // alongside the role's internal name and (optionally) a description, so we
    // can no longer match the whole td's text content. Filter against the
    // bold display-name div instead.
    return this.rolesTable.locator('tr').filter({
      has: this.page.locator('td .font-medium').filter({
        hasText: new RegExp(`^${displayName}$`)
      })
    });
  }

  /**
   * Click the Edit button for a specific role.
   */
  async clickEditRole(displayName: string): Promise<void> {
    const row = this.getRoleRow(displayName);
    await row.getByRole('button', { name: 'Edit' }).click();
  }

  // --- Create/Edit Role Form Actions ---

  /**
   * Fill in the role form fields.
   */
  async fillRoleForm(options: {
    name?: string;
    displayName?: string;
    description?: string;
  }): Promise<void> {
    if (options.name !== undefined) {
      await this.nameInput.fill(options.name);
    }
    if (options.displayName !== undefined) {
      await this.displayNameInput.fill(options.displayName);
    }
    if (options.description !== undefined) {
      await this.descriptionInput.fill(options.description);
    }
  }

  /**
   * Create a new role with the given details.
   */
  async createRole(
    spaceId: string,
    options: { name: string; displayName: string; description?: string }
  ): Promise<void> {
    await this.gotoCreateRole(spaceId);
    await this.fillRoleForm(options);
    await this.submitButton.click();
    // Wait for navigation to the role detail page
    await expect(this.page.getByRole('heading', { name: 'Edit Role' })).toBeVisible();
  }

  // --- Permission Grid Actions ---

  /**
   * Get the permission row by the permission identifier. The grid is now a
   * DataTable; each row exposes the identifier via
   * `[data-testid="permission-name"]`.
   */
  getPermissionRow(permission: string): Locator {
    return this.page.locator('tr').filter({
      has: this.page.locator(`[data-testid="permission-name"]:text-is("${permission}")`)
    });
  }

  /**
   * Get the Allow ToggleChip button for a specific permission. Despite the
   * historical "checkbox" name, this is now an aria-pressed button — keep
   * the method name so test cases that already call `toBeEnabled()` etc.
   * continue to work.
   */
  getPermissionCheckbox(permission: string): Locator {
    return this.getPermissionRow(permission).getByRole('button', { name: 'Allow' });
  }

  /** Get the Deny ToggleChip button for a specific permission. */
  getDenyPermissionCheckbox(permission: string): Locator {
    return this.getPermissionRow(permission).getByRole('button', { name: 'Deny' });
  }

  /**
   * Toggle the Allow state for a permission.
   * If currently allowed, sets to neutral. If neutral, sets to allowed.
   */
  async togglePermission(permission: string): Promise<void> {
    await this.getPermissionCheckbox(permission).click();
  }

  /** Deny a permission. */
  async denyPermission(permission: string): Promise<void> {
    await this.getDenyPermissionCheckbox(permission).click();
  }

  /** Whether a permission is currently granted (Allow pill is pressed). */
  async isPermissionGranted(permission: string): Promise<boolean> {
    return (
      (await this.getPermissionCheckbox(permission).getAttribute('aria-pressed')) === 'true'
    );
  }

  /** Whether a permission is currently denied (Deny pill is pressed). */
  async isPermissionDenied(permission: string): Promise<boolean> {
    return (
      (await this.getDenyPermissionCheckbox(permission).getAttribute('aria-pressed')) === 'true'
    );
  }

  // --- Delete Role Actions ---

  /**
   * Delete the currently viewed role.
   */
  async deleteCurrentRole(): Promise<void> {
    await this.deleteRoleButton.click();
    await this.confirmDeleteButton.click();
  }

  // --- Assertions ---

  /**
   * Assert the roles list page is visible.
   */
  async expectRolesListVisible(): Promise<void> {
    await expect(this.pageHeading).toBeVisible();
    await expect(this.rolesTable).toBeVisible();
  }

  /**
   * Assert a role is listed with the given display name.
   */
  async expectRoleInList(displayName: string): Promise<void> {
    await expect(this.getRoleRow(displayName)).toBeVisible();
  }

  /**
   * Assert a role is NOT in the list.
   */
  async expectRoleNotInList(displayName: string): Promise<void> {
    await expect(this.getRoleRow(displayName)).not.toBeVisible();
  }

  /**
   * Assert the Create Role button is visible.
   */
  async expectCreateRoleButtonVisible(): Promise<void> {
    await expect(this.createRoleButton).toBeVisible();
  }

  /**
   * Assert the Create Role button is NOT visible.
   */
  async expectCreateRoleButtonNotVisible(): Promise<void> {
    await expect(this.createRoleButton).not.toBeVisible();
  }

  /** Assert a permission's Allow pill is in the pressed state. */
  async expectPermissionGranted(permission: string): Promise<void> {
    await expect(this.getPermissionCheckbox(permission)).toHaveAttribute(
      'aria-pressed',
      'true'
    );
  }

  /** Assert a permission's Allow pill is NOT in the pressed state. */
  async expectPermissionNotGranted(permission: string): Promise<void> {
    await expect(this.getPermissionCheckbox(permission)).toHaveAttribute(
      'aria-pressed',
      'false'
    );
  }

  /**
   * Assert the delete role button is visible.
   */
  async expectDeleteRoleButtonVisible(): Promise<void> {
    await expect(this.deleteRoleButton).toBeVisible();
  }

  /**
   * Assert the delete role button is NOT visible.
   */
  async expectDeleteRoleButtonNotVisible(): Promise<void> {
    await expect(this.deleteRoleButton).not.toBeVisible();
  }

  /**
   * Assert an access denied message is shown.
   * Note: Since authorization is now handled at the settings layout level,
   * this checks for the layout's Access Denied component, not a page-specific message.
   */
  async expectAccessDenied(): Promise<void> {
    await expect(this.page.getByText('Access Denied', { exact: true })).toBeVisible();
  }

  /**
   * Assert a validation error message is shown.
   */
  async expectValidationError(message: string): Promise<void> {
    await expect(this.page.getByText(message)).toBeVisible();
  }

  /**
   * Assert the role name field shows the correct value.
   */
  async expectRoleName(name: string): Promise<void> {
    await expect(this.page.locator(`code:text-is("${name}")`)).toBeVisible();
  }

  /**
   * Assert the read-only message is shown (for non-admin users).
   */
  async expectReadOnlyMessage(): Promise<void> {
    await expect(
      this.page.getByText('You need the roles.manage permission to make changes')
    ).toBeVisible();
  }

  /**
   * Assert a toast message is visible.
   */
  async expectToast(message: string): Promise<void> {
    await expect(this.page.getByText(message)).toBeVisible();
  }

  // --- Instance Roles ---

  /**
   * Get an instance-role row in the unified "Roles applicable in this space"
   * table by its internal name (e.g. "instance-admin"). Instance and space
   * roles share one table now, with a Scope pill on each row.
   */
  getInstanceRoleRow(name: string): Locator {
    return this.rolesTable.locator('tr').filter({
      has: this.page.locator(`code:text-is("${name}")`)
    });
  }

  /**
   * Click into the per-space configuration of an instance role. The merged
   * roles table dispatches between space role and instance-role detail pages
   * based on the row, so we click the row's Edit button (or the row itself).
   */
  async clickConfigureInstanceRole(name: string): Promise<void> {
    const row = this.getInstanceRoleRow(name);
    await row.getByRole('button', { name: 'Edit' }).click();
  }

  /**
   * Navigate to instance role detail page.
   */
  async gotoInstanceRoleDetail(spaceId: string, roleName: string): Promise<void> {
    await this.page.goto(routes.spaceAdminInstanceRole(spaceId, roleName));
    await expect(
      this.page.getByRole('heading', { name: 'Instance Role Permissions' })
    ).toBeVisible();
  }

  /**
   * Assert the Instance Roles panel is visible.
   */
  async expectInstanceRolesPanelVisible(): Promise<void> {
    // Instance roles now share the unified "Roles applicable in this space"
    // table; there's no separate panel. Asserting that the always-present
    // instance-admin row exists is a strict superset of the original check.
    await expect(this.getInstanceRoleRow('instance-admin')).toBeVisible();
  }

  /**
   * Assert an instance role is listed.
   */
  async expectInstanceRoleInList(name: string): Promise<void> {
    await expect(this.getInstanceRoleRow(name)).toBeVisible();
  }

  /**
   * Assert instance role detail page is shown with correct role.
   */
  async expectInstanceRoleDetailPage(roleName: string): Promise<void> {
    await expect(
      this.page.getByRole('heading', { name: 'Instance Role Permissions' })
    ).toBeVisible();
    // The role name is shown in the subtitle with instance: prefix
    await expect(this.page.locator(`code:text-is("instance:${roleName}")`)).toBeVisible();
  }

  /** Assert a permission's Deny pill is in the pressed state. */
  async expectPermissionDenied(permission: string): Promise<void> {
    await expect(this.getDenyPermissionCheckbox(permission)).toHaveAttribute(
      'aria-pressed',
      'true'
    );
  }

  /** Assert a permission's Deny pill is NOT in the pressed state. */
  async expectPermissionNotDenied(permission: string): Promise<void> {
    await expect(this.getDenyPermissionCheckbox(permission)).toHaveAttribute(
      'aria-pressed',
      'false'
    );
  }
}
