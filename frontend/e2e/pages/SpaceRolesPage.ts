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

  /** The roles table (uses border-collapse class unique to space roles table) */
  get rolesTable(): Locator {
    return this.page.locator('table.border-collapse');
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
    // Find a row that contains a td cell with exactly the display name text.
    // Uses a td selector that matches exact text content (not code elements which contain role names).
    // The column position may vary depending on whether the drag handle column is visible.
    return this.rolesTable.locator('tr').filter({
      has: this.page.locator('td').filter({ hasText: new RegExp(`^${displayName}$`) })
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
   * Get the permission row containing the Allow and Deny checkboxes.
   * The permission grid renders each permission in a div with rounded-lg border classes.
   */
  getPermissionRow(permission: string): Locator {
    // Target the permission row by finding a rounded-lg bordered div containing the specific code element
    return this.page.locator('div.rounded-lg.border').filter({
      has: this.page.locator(`code:text-is("${permission}")`)
    });
  }

  /**
   * Get the Allow checkbox for a specific permission.
   */
  getPermissionCheckbox(permission: string): Locator {
    // Find the row with the permission code, then get the Allow checkbox
    return this.getPermissionRow(permission)
      .locator('label')
      .filter({ hasText: 'Allow' })
      .locator('input[type="checkbox"]');
  }

  /**
   * Get the Deny checkbox for a specific permission.
   */
  getDenyPermissionCheckbox(permission: string): Locator {
    // Find the row with the permission code, then get the Deny checkbox
    return this.getPermissionRow(permission)
      .locator('label')
      .filter({ hasText: 'Deny' })
      .locator('input[type="checkbox"]');
  }

  /**
   * Toggle the Allow state for a permission.
   * If currently allowed, sets to neutral. If neutral, sets to allowed.
   */
  async togglePermission(permission: string): Promise<void> {
    await this.getPermissionCheckbox(permission).click();
  }

  /**
   * Deny a permission.
   */
  async denyPermission(permission: string): Promise<void> {
    await this.getDenyPermissionCheckbox(permission).click();
  }

  /**
   * Check if a permission is granted (Allow checkbox checked).
   */
  async isPermissionGranted(permission: string): Promise<boolean> {
    return this.getPermissionCheckbox(permission).isChecked();
  }

  /**
   * Check if a permission is denied (Deny checkbox checked).
   */
  async isPermissionDenied(permission: string): Promise<boolean> {
    return this.getDenyPermissionCheckbox(permission).isChecked();
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

  /**
   * Assert a permission is checked.
   */
  async expectPermissionGranted(permission: string): Promise<void> {
    await expect(this.getPermissionCheckbox(permission)).toBeChecked();
  }

  /**
   * Assert a permission is NOT checked.
   */
  async expectPermissionNotGranted(permission: string): Promise<void> {
    await expect(this.getPermissionCheckbox(permission)).not.toBeChecked();
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

  /** The Instance Roles panel (find by heading) */
  get instanceRolesPanel(): Locator {
    // Find the div that contains an h2 with "Instance Roles" heading
    return this.page.locator('div.rounded-xl').filter({
      has: this.page.getByRole('heading', { name: 'Instance Roles' })
    });
  }

  /** The instance roles table */
  get instanceRolesTable(): Locator {
    return this.instanceRolesPanel.locator('table');
  }

  /**
   * Get an instance role row by its name.
   */
  getInstanceRoleRow(name: string): Locator {
    return this.instanceRolesTable.locator('tr').filter({
      has: this.page.locator(`code:text-is("${name}")`)
    });
  }

  /**
   * Click the Configure button for a specific instance role.
   */
  async clickConfigureInstanceRole(name: string): Promise<void> {
    const row = this.getInstanceRoleRow(name);
    await row.getByRole('button', { name: 'Configure' }).click();
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
    await expect(this.instanceRolesPanel).toBeVisible();
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

  /**
   * Assert permission is denied (Deny checkbox checked) for an instance role.
   */
  async expectPermissionDenied(permission: string): Promise<void> {
    await expect(this.getDenyPermissionCheckbox(permission)).toBeChecked();
  }

  /**
   * Assert permission is not denied (Deny checkbox unchecked) for an instance role.
   */
  async expectPermissionNotDenied(permission: string): Promise<void> {
    await expect(this.getDenyPermissionCheckbox(permission)).not.toBeChecked();
  }
}
