import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import RoleTable from './RoleTable.svelte';
import type { Role } from './types';

// Type helper
function renderRoleTable(
  props: Partial<{
    roles: Role[];
    canManage: boolean;
    adminRoleName: string;
    onEdit: ((role: Role) => void) | undefined;
  }>
) {
  const defaultProps = {
    roles: [],
    canManage: true,
    adminRoleName: 'admin',
    onEdit: undefined,
    ...props
  };
  return render(RoleTable, { props: defaultProps });
}

import { q } from '$lib/test-utils';

const qAll = (container: Element, selector: string) => container.querySelectorAll(selector);

const createTestRole = (overrides: Partial<Role> = {}): Role => ({
  name: 'test',
  displayName: 'Test Role',
  description: 'A test role',
  permissions: [],
  permissionDenials: [],
  isSystem: false,
  position: 1,
  ...overrides
});

describe('RoleTable', () => {
  describe('rendering', () => {
    it('renders table headers', async () => {
      const { container } = renderRoleTable({ roles: [] });

      expect(container.textContent).toContain('Name');
      expect(container.textContent).toContain('Display Name');
      expect(container.textContent).toContain('Description');
      expect(container.textContent).toContain('Permissions');
      expect(container.textContent).toContain('Type');
    });

    it('renders a row for each role', async () => {
      const roles = [
        createTestRole({ name: 'role1', displayName: 'Role 1' }),
        createTestRole({ name: 'role2', displayName: 'Role 2' }),
        createTestRole({ name: 'role3', displayName: 'Role 3' })
      ];
      const { container } = renderRoleTable({ roles });

      const rows = qAll(container, 'tbody tr');
      expect(rows.length).toBe(3);
    });

    it('displays role display name', async () => {
      const roles = [createTestRole({ displayName: 'Custom Role' })];
      const { container } = renderRoleTable({ roles });

      expect(container.textContent).toContain('Custom Role');
    });

    it('displays role description', async () => {
      const roles = [createTestRole({ description: 'My custom description' })];
      const { container } = renderRoleTable({ roles });

      expect(container.textContent).toContain('My custom description');
    });

    it('displays role name in code element', async () => {
      const roles = [createTestRole({ name: 'custom_role' })];
      const { container } = renderRoleTable({ roles });

      const codeElement = q(container, 'code');
      await expect.element(codeElement).toHaveTextContent('custom_role');
    });
  });

  describe('system role badge', () => {
    it('shows System badge for system roles', async () => {
      const roles = [createTestRole({ isSystem: true })];
      const { container } = renderRoleTable({ roles });

      expect(container.textContent).toContain('System');
    });

    it('shows Custom badge for non-system roles', async () => {
      const roles = [createTestRole({ isSystem: false })];
      const { container } = renderRoleTable({ roles });

      expect(container.textContent).toContain('Custom');
      expect(container.textContent).not.toMatch(/\bSystem\b/);
    });
  });

  describe('edit button', () => {
    it('renders Edit button when canManage is true and onEdit is provided', async () => {
      const roles = [createTestRole()];
      const onEdit = vi.fn();
      const { container } = renderRoleTable({ roles, canManage: true, onEdit });

      await expect.element(q(container, 'button')).toHaveTextContent('Edit');
    });

    it('does not render Edit button when canManage is false', async () => {
      const roles = [createTestRole()];
      const { container } = renderRoleTable({ roles, canManage: false });

      const editButton = q(container, 'button');
      expect(editButton).toBeNull();
    });

    it('does not render Edit button when onEdit is not provided', async () => {
      const roles = [createTestRole()];
      const { container } = renderRoleTable({ roles, canManage: true, onEdit: undefined });

      const editButton = q(container, 'button');
      expect(editButton).toBeNull();
    });

    it('calls onEdit with role when Edit button is clicked', async () => {
      const role = createTestRole({ name: 'testrole' });
      const onEdit = vi.fn();
      const { container } = renderRoleTable({ roles: [role], canManage: true, onEdit });

      const editButton = q(container, 'button');
      editButton?.click();

      // The role object passed to onEdit includes an `id` property added by the dnd library
      expect(onEdit).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'testrole',
          displayName: role.displayName,
          description: role.description,
          permissions: role.permissions,
          isSystem: role.isSystem,
          position: role.position
        })
      );
    });
  });

  describe('admin role display', () => {
    it('shows "All" for admin role permissions count', async () => {
      const roles = [createTestRole({ name: 'admin', displayName: 'Administrator' })];
      const { container } = renderRoleTable({ roles, adminRoleName: 'admin' });

      // Admin role shows "All" instead of a permission count
      expect(container.textContent).toContain('All');
    });

    it('shows permission count for non-admin roles', async () => {
      const roles = [
        createTestRole({ name: 'member', permissions: ['rooms.create', 'rooms.browse'] })
      ];
      const { container } = renderRoleTable({ roles, adminRoleName: 'admin' });

      // Non-admin roles show the actual permission count
      expect(container.textContent).toContain('2');
    });
  });

  describe('empty state', () => {
    it('shows "No roles found" message when no roles', async () => {
      const { container } = renderRoleTable({ roles: [] });

      expect(container.textContent).toContain('No roles found');
    });
  });
});
