import { expect, type Locator, type Page } from '@playwright/test';
import * as routes from '../routes';

/**
 * Page object for the Space Roles management pages.
 * Handles viewing, creating, editing, and deleting space roles.
 */
export class SpaceRolesPage {
  constructor(readonly page: Page) {}

  /**
   * The role currently being edited, set by `gotoEditRole`. Subsequent
   * permission interactions resolve the matrix cell against this role.
   */
  private currentRoleName: string | null = null;
  /**
   * The space currently in scope, set by `gotoEditRole`. Used by the
   * permission-interaction helpers to navigate back to the matrix when
   * needed (the role detail page no longer carries the editor).
   */
  private currentSpaceId: string | null = null;

  // --- Locators ---

  /** The page heading */
  get pageHeading(): Locator {
    return this.page.getByRole('heading', { name: 'Roles', exact: true });
  }

  /**
   * The Create Role action. Renders as an `<a>` (with button styling) when
   * the page wires it via `href`, but we keep the method name and match
   * either tag so existing tests keep working.
   */
  get createRoleButton(): Locator {
    return this.page.locator('a, button').filter({ hasText: /^Create Role$/ });
  }

  /** Sidebar navigation item for General settings */
  get generalNavItem(): Locator {
    return this.page.locator('nav a', { hasText: 'General' });
  }

  /**
   * The first matrix table on the page. The matrix renders one `<table>`
   * per permission category (Space Operations, Messages, …); the first one
   * is enough to assert "the matrix rendered".
   */
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
   * Navigate to a specific role's edit page. The role detail page now hosts
   * metadata + assigned-users only; permission editing happens on the matrix
   * at the roles list. We track the role name here so subsequent permission
   * helpers can resolve the matrix cell — they'll auto-navigate to the
   * matrix as needed.
   */
  async gotoEditRole(spaceId: string, roleName: string): Promise<void> {
    this.currentRoleName = roleName;
    this.currentSpaceId = spaceId;
    await this.page.goto(routes.spaceAdminRole(spaceId, roleName));
    await expect(this.page.getByRole('heading', { name: 'Edit Role' })).toBeVisible();
  }

  // --- Role List Actions ---

  /**
   * Resolve the matrix column header for a role by its display name. The
   * matrix renders one table per permission category, so the same role's
   * `<th>` appears once per category — we take the first match to satisfy
   * Playwright's strict mode. The `<th>`'s title attribute carries the
   * displayName + scope marker (e.g. `"Owner (Space role) — click to
   * manage"`), so we anchor the match to the displayName followed by
   * ` (` to avoid partial-string collisions (e.g. "Owner" matching
   * "Instance Owner").
   */
  getRoleRow(displayName: string): Locator {
    const escaped = displayName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    return this.page.locator(`th[title^="${escaped} ("]`).first();
  }

  /**
   * Click into a role's detail page from the matrix by its column header.
   */
  async clickEditRole(displayName: string): Promise<void> {
    await this.getRoleRow(displayName).locator('button').click();
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
   * Create a new role with the given details. Tracks the role so subsequent
   * permission helpers can resolve the matrix cell.
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
    this.currentRoleName = options.name;
    this.currentSpaceId = spaceId;
  }

  // --- Permission Matrix Actions ---

  /**
   * The permission row container for `permission` on the matrix. The
   * sticky-left `<td>` carries `[data-testid="permission-name"]` with the
   * identifier; we filter by it to find the parent `<tr>`.
   */
  getPermissionRow(permission: string): Locator {
    return this.page.locator('tr').filter({
      has: this.page.locator(`[data-testid="permission-name"]:text-is("${permission}")`)
    });
  }

  /**
   * Resolve the matrix cell for `roleName × permission`. The matrix
   * decorates each cell `<td>` with `data-role` and `data-permission`.
   */
  matrixCellFor(roleName: string, permission: string): Locator {
    return this.page.locator(
      `td[data-role="${roleName}"][data-permission="${permission}"] button`
    );
  }

  /** Internal helper — uses the role tracked by `gotoEditRole`. */
  private currentCell(permission: string): Locator {
    if (!this.currentRoleName) {
      throw new Error(
        'SpaceRolesPage permission helpers require a current role — call gotoEditRole(...) first.'
      );
    }
    return this.matrixCellFor(this.currentRoleName, permission);
  }

  /**
   * Compatibility shim — exposes the matrix cell so older tests that drove
   * an Allow ToggleChip via `getPermissionCheckbox` keep working. The
   * "Allow" semantic is folded into the cell now (allow when its aria-label
   * matches `Override allow`). NOTE: this is a sync getter, so callers
   * must already be on the matrix; use `expectPermissionEditable` if you
   * need an async navigation guard.
   */
  getPermissionCheckbox(permission: string): Locator {
    return this.currentCell(permission);
  }

  /**
   * Assert the matrix cell for the current role × permission is editable
   * (i.e. its button is in the DOM and enabled). Auto-navigates to the
   * matrix first.
   */
  async expectPermissionEditable(permission: string): Promise<void> {
    await this.ensureOnMatrix();
    await expect(this.currentCell(permission)).toBeEnabled();
  }

  /**
   * Ensure we're on the matrix (the role detail page no longer carries the
   * permission editor). Navigates to the listing if we're elsewhere.
   */
  private async ensureOnMatrix(): Promise<void> {
    if (!this.currentSpaceId) {
      throw new Error(
        'SpaceRolesPage permission helpers require a current space — call gotoEditRole(...) first.'
      );
    }
    if (!this.page.url().endsWith(`/admin/roles`)) {
      await this.page.goto(routes.spaceAdminRoles(this.currentSpaceId));
      await expect(this.pageHeading).toBeVisible();
    }
  }

  /**
   * Drive the matrix cell for the current role × permission to a target
   * state (`allow`, `deny`, or `neutral`). The cell cycles
   * `neutral → allow → deny → neutral` on each click; we click up to three
   * times until the state lands.
   */
  async setPermissionState(
    permission: string,
    target: 'allow' | 'deny' | 'neutral'
  ): Promise<void> {
    await this.ensureOnMatrix();
    const cell = this.currentCell(permission);
    for (let i = 0; i < 3; i++) {
      const label = (await cell.getAttribute('aria-label')) ?? '';
      if (target === 'allow' && /Override allow/.test(label)) return;
      if (target === 'deny' && /Override deny/.test(label)) return;
      if (target === 'neutral' && /No override/.test(label)) return;
      await cell.click();
      // Optimistic UI update is synchronous after the GraphQL mutation
      // resolves; one tick is enough.
      await this.page.waitForFunction(
        ({ from, prev }) => {
          const el = document.querySelector(from) as HTMLElement | null;
          return el ? (el.getAttribute('aria-label') ?? '') !== prev : false;
        },
        {
          from: `td[data-role="${this.currentRoleName}"][data-permission="${permission}"] button`,
          prev: label
        }
      );
    }
  }

  /**
   * Cycle a permission once on the matrix. Used by tests that specifically
   * want to exercise click semantics (e.g. "from neutral, one click lands
   * on allow"). Waits for the optimistic update to land in the DOM so a
   * subsequent `page.reload()` doesn't race the mutation.
   */
  async togglePermission(permission: string): Promise<void> {
    await this.ensureOnMatrix();
    const cell = this.currentCell(permission);
    const before = (await cell.getAttribute('aria-label')) ?? '';
    await cell.click();
    await this.page.waitForFunction(
      ({ selector, prev }) => {
        const el = document.querySelector(selector) as HTMLElement | null;
        return el ? (el.getAttribute('aria-label') ?? '') !== prev : false;
      },
      {
        selector: `td[data-role="${this.currentRoleName}"][data-permission="${permission}"] button`,
        prev: before
      }
    );
  }

  /** Drive the cell to the deny state. */
  async denyPermission(permission: string): Promise<void> {
    await this.setPermissionState(permission, 'deny');
  }

  /** Whether the cell currently shows an allow override. */
  async isPermissionGranted(permission: string): Promise<boolean> {
    const label = (await this.currentCell(permission).getAttribute('aria-label')) ?? '';
    return /Override allow/.test(label);
  }

  /** Whether the cell currently shows a deny override. */
  async isPermissionDenied(permission: string): Promise<boolean> {
    const label = (await this.currentCell(permission).getAttribute('aria-label')) ?? '';
    return /Override deny/.test(label);
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

  /** Assert the matrix cell for the current role × permission is set to allow. */
  async expectPermissionGranted(permission: string): Promise<void> {
    await this.ensureOnMatrix();
    await expect(this.currentCell(permission)).toHaveAttribute(
      'aria-label',
      /Override allow/
    );
  }

  /**
   * Assert the matrix cell for the current role × permission is NOT set to
   * allow at this scope (it might be deny or neutral).
   */
  async expectPermissionNotGranted(permission: string): Promise<void> {
    await this.ensureOnMatrix();
    const cell = this.currentCell(permission);
    const label = (await cell.getAttribute('aria-label')) ?? '';
    expect(label).not.toMatch(/Override allow/);
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
   * No-op shim. The matrix doesn't toast on success — the cell's aria
   * state changes synchronously after the mutation resolves and the
   * subsequent assertions (e.g. `expectPermissionGranted`) verify the
   * effect. Kept so existing tests don't have to be rewritten.
   */
  async expectToast(_message: string): Promise<void> {
    // intentionally empty
  }

  // --- Instance Roles ---

  /**
   * Get an instance-role row in the unified "Roles applicable in this space"
   * table by its internal name (e.g. "instance-admin"). Instance and space
   * roles share one table now, with a Scope pill on each row.
   */
  /**
   * Resolve the matrix column header for an instance role by its slug
   * (e.g. "instance-admin"). The header text is `@${roleName}` and the
   * `<th>` carries `data-role`. Same per-category duplication as
   * `getRoleRow` — take the first match.
   */
  getInstanceRoleRow(name: string): Locator {
    return this.page.locator(`th[data-role="${name}"]`).first();
  }

  /**
   * Click into the per-space configuration of an instance role. The merged
   * roles table dispatches between space role and instance-role detail pages
   * based on the row, so we click the row's Edit button (or the row itself).
   */
  async clickConfigureInstanceRole(name: string): Promise<void> {
    // The matrix's column header itself is the configure affordance.
    // Clicking it routes to the instance role's detail page.
    await this.getInstanceRoleRow(name).locator('button').click();
  }

  /**
   * Navigate to instance role permission editing. The dedicated
   * /admin/roles/instance/[name] page no longer exists — instance role
   * permissions are now configured at space scope via the matrix on the
   * roles list. We navigate there and remember the role so subsequent
   * permission helpers target the right column.
   */
  async gotoInstanceRoleDetail(spaceId: string, roleName: string): Promise<void> {
    this.currentRoleName = roleName;
    this.currentSpaceId = spaceId;
    await this.gotoRolesList(spaceId);
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
   * Clicking an instance-role column header at space scope routes to the
   * instance-scope role detail page (`/admin/roles/[name]`), which carries
   * "Edit Role" + the role slug as a `<code>` value.
   */
  async expectInstanceRoleDetailPage(roleName: string): Promise<void> {
    await expect(this.page.getByRole('heading', { name: 'Edit Role' })).toBeVisible();
    await expect(this.page.locator(`code:text-is("${roleName}")`)).toBeVisible();
  }

  /** Assert the matrix cell for the current role × permission is set to deny. */
  async expectPermissionDenied(permission: string): Promise<void> {
    await this.ensureOnMatrix();
    await expect(this.currentCell(permission)).toHaveAttribute(
      'aria-label',
      /Override deny/
    );
  }

  /**
   * Assert the matrix cell for the current role × permission is NOT set
   * to deny at this scope (it might be allow or neutral).
   */
  async expectPermissionNotDenied(permission: string): Promise<void> {
    await this.ensureOnMatrix();
    const label = (await this.currentCell(permission).getAttribute('aria-label')) ?? '';
    expect(label).not.toMatch(/Override deny/);
  }
}
