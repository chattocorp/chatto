import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import PermissionGrid from './PermissionGrid.svelte';
import type { PermissionState } from './types';

// Type helper
function renderPermissionGrid(
  props: Partial<{
    permissions: string[];
    grantedPermissions: string[];
    deniedPermissions: string[];
    disabled: boolean;
    updatingPermission: string | null;
    onSetState: (permission: string, state: PermissionState) => void;
  }>
) {
  const defaultProps = {
    permissions: [],
    grantedPermissions: [],
    deniedPermissions: [],
    disabled: false,
    updatingPermission: null,
    onSetState: vi.fn(),
    ...props
  };
  return render(PermissionGrid, { props: defaultProps });
}

const qAll = (container: Element, selector: string) => container.querySelectorAll(selector);

describe('PermissionGrid', () => {
  describe('rendering', () => {
    it('renders two checkboxes (Allow + Deny) for each permission', async () => {
      const permissions = ['rooms.create', 'rooms.browse', 'space.manage'];
      const { container } = renderPermissionGrid({ permissions });

      // Each permission has an Allow checkbox and a Deny checkbox
      const checkboxes = qAll(container, 'input[type="checkbox"]');
      expect(checkboxes.length).toBe(6); // 3 permissions * 2 checkboxes each
    });

    it('displays permission names in code elements', async () => {
      const permissions = ['rooms.create', 'rooms.browse'];
      const { container } = renderPermissionGrid({ permissions });

      const codeElements = qAll(container, 'code');
      expect(codeElements.length).toBe(2);
      // Sorted alphabetically
      expect(codeElements[0].textContent).toBe('rooms.browse');
      expect(codeElements[1].textContent).toBe('rooms.create');
    });

    it('displays permission descriptions from local module', async () => {
      const permissions = ['room.create'];
      const { container } = renderPermissionGrid({ permissions });

      // Description comes from the permissions module
      expect(container.textContent).toContain('Create new rooms');
    });

    it('renders permissions grouped by category, alphabetically within groups', async () => {
      // Use permissions from same category to test alphabetical ordering within group
      const permissions = ['room.leave', 'room.create', 'room.join'];
      const { container } = renderPermissionGrid({ permissions });

      const codeElements = qAll(container, 'code');
      // Should be sorted alphabetically within the 'room' category
      expect(codeElements[0].textContent).toBe('room.create');
      expect(codeElements[1].textContent).toBe('room.join');
      expect(codeElements[2].textContent).toBe('room.leave');
    });

    it('groups permissions by category with headers', async () => {
      const permissions = ['space.create', 'room.join', 'message.post'];
      const { container } = renderPermissionGrid({ permissions });

      // Should have category headers
      const headers = qAll(container, 'h3');
      expect(headers.length).toBe(3); // space, room, message categories

      // Headers should be in the expected order (space, room, message)
      expect(headers[0].textContent).toBe('Space Operations');
      expect(headers[1].textContent).toBe('Room Operations');
      expect(headers[2].textContent).toBe('Messages');
    });

    it('renders empty grid when no permissions', async () => {
      const { container } = renderPermissionGrid({ permissions: [] });
      const checkboxes = qAll(container, 'input[type="checkbox"]');
      expect(checkboxes.length).toBe(0);
    });
  });

  describe('three-state permissions', () => {
    it('checks Allow checkbox for granted permissions', async () => {
      const permissions = ['rooms.create'];
      const grantedPermissions = ['rooms.create'];
      const { container } = renderPermissionGrid({ permissions, grantedPermissions });

      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      // First checkbox is Allow, second is Deny
      expect(checkboxes[0].checked).toBe(true); // Allow is checked
      expect(checkboxes[1].checked).toBe(false); // Deny is unchecked
    });

    it('checks Deny checkbox for denied permissions', async () => {
      const permissions = ['rooms.create'];
      const deniedPermissions = ['rooms.create'];
      const { container } = renderPermissionGrid({ permissions, deniedPermissions });

      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      // First checkbox is Allow, second is Deny
      expect(checkboxes[0].checked).toBe(false); // Allow is unchecked
      expect(checkboxes[1].checked).toBe(true); // Deny is checked
    });

    it('neither checkbox checked for neutral permissions', async () => {
      const permissions = ['rooms.create'];
      const { container } = renderPermissionGrid({ permissions });

      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      expect(checkboxes[0].checked).toBe(false); // Allow is unchecked
      expect(checkboxes[1].checked).toBe(false); // Deny is unchecked
    });

    it('shows appropriate styling for allowed state', async () => {
      const permissions = ['rooms.create'];
      const grantedPermissions = ['rooms.create'];
      const { container } = renderPermissionGrid({ permissions, grantedPermissions });

      // Permission identifier should be tinted with success color.
      const code = container.querySelector('code.text-success');
      expect(code?.textContent).toBe('rooms.create');
    });

    it('shows appropriate styling for denied state', async () => {
      const permissions = ['rooms.create'];
      const deniedPermissions = ['rooms.create'];
      const { container } = renderPermissionGrid({ permissions, deniedPermissions });

      // Permission identifier should be tinted with danger color.
      const code = container.querySelector('code.text-danger');
      expect(code?.textContent).toBe('rooms.create');
    });
  });

  describe('disabled state', () => {
    it('disables all checkboxes when disabled is true', async () => {
      const permissions = ['rooms.create'];
      const { container } = renderPermissionGrid({ permissions, disabled: true });

      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      expect(checkboxes[0].disabled).toBe(true);
      expect(checkboxes[1].disabled).toBe(true);
    });

    it('enables checkboxes when disabled is false', async () => {
      const permissions = ['rooms.create'];
      const { container } = renderPermissionGrid({ permissions, disabled: false });

      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      expect(checkboxes[0].disabled).toBe(false);
      expect(checkboxes[1].disabled).toBe(false);
    });

    it('disables all checkbox inputs when disabled is true', async () => {
      const permissions = ['rooms.create', 'rooms.delete'];
      const { container } = renderPermissionGrid({ permissions, disabled: true });

      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      expect(checkboxes.length).toBeGreaterThan(0);
      for (const cb of checkboxes) {
        expect(cb.disabled).toBe(true);
      }
    });
  });

  describe('updating state', () => {
    it('disables checkboxes for permission being updated', async () => {
      const permissions = ['rooms.browse', 'rooms.create'];
      const { container } = renderPermissionGrid({
        permissions,
        updatingPermission: 'rooms.create'
      });

      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      // After alphabetical sorting: rooms.browse (checkboxes 0,1), rooms.create (checkboxes 2,3)
      expect(checkboxes[0].disabled).toBe(false); // rooms.browse Allow
      expect(checkboxes[1].disabled).toBe(false); // rooms.browse Deny
      expect(checkboxes[2].disabled).toBe(true); // rooms.create Allow - being updated
      expect(checkboxes[3].disabled).toBe(true); // rooms.create Deny - being updated
    });

    it('adds pulse animation to row being updated', async () => {
      const permissions = ['rooms.create'];
      const { container } = renderPermissionGrid({
        permissions,
        updatingPermission: 'rooms.create'
      });

      const row = container.querySelector('.animate-pulse');
      expect(row).not.toBeNull();
    });
  });

  describe('onSetState callback', () => {
    it('calls onSetState with neutral when unchecking Allow', async () => {
      const onSetState = vi.fn();
      const permissions = ['rooms.create'];
      const grantedPermissions = ['rooms.create'];
      const { container } = renderPermissionGrid({
        permissions,
        grantedPermissions,
        onSetState
      });

      // Click Allow checkbox (first checkbox) to uncheck it
      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      checkboxes[0].click();

      expect(onSetState).toHaveBeenCalledWith('rooms.create', 'neutral');
    });

    it('calls onSetState with allow when checking Allow', async () => {
      const onSetState = vi.fn();
      const permissions = ['rooms.create'];
      const { container } = renderPermissionGrid({
        permissions,
        grantedPermissions: [],
        onSetState
      });

      // Click Allow checkbox (first checkbox) to check it
      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      checkboxes[0].click();

      expect(onSetState).toHaveBeenCalledWith('rooms.create', 'allow');
    });

    it('calls onSetState with deny when checking Deny', async () => {
      const onSetState = vi.fn();
      const permissions = ['rooms.create'];
      const { container } = renderPermissionGrid({
        permissions,
        grantedPermissions: [],
        onSetState
      });

      // Click Deny checkbox (second checkbox) to check it
      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      checkboxes[1].click();

      expect(onSetState).toHaveBeenCalledWith('rooms.create', 'deny');
    });

    it('calls onSetState with neutral when unchecking Deny', async () => {
      const onSetState = vi.fn();
      const permissions = ['rooms.create'];
      const deniedPermissions = ['rooms.create'];
      const { container } = renderPermissionGrid({
        permissions,
        deniedPermissions,
        onSetState
      });

      // Click Deny checkbox (second checkbox) to uncheck it
      const checkboxes = qAll(container, 'input[type="checkbox"]') as NodeListOf<HTMLInputElement>;
      checkboxes[1].click();

      expect(onSetState).toHaveBeenCalledWith('rooms.create', 'neutral');
    });
  });
});
