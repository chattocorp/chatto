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

// Each permission row has two buttons: Allow and Deny.
function buttonsFor(container: Element): HTMLButtonElement[] {
  return Array.from(
    container.querySelectorAll('button[aria-pressed]')
  ) as HTMLButtonElement[];
}

describe('PermissionGrid', () => {
  describe('rendering', () => {
    it('renders Allow and Deny buttons for each permission', async () => {
      const permissions = ['rooms.create', 'rooms.browse', 'space.manage'];
      const { container } = renderPermissionGrid({ permissions });

      const buttons = buttonsFor(container);
      expect(buttons.length).toBe(6); // 3 permissions × 2 buttons
    });

    it('displays permission names', async () => {
      const permissions = ['rooms.create', 'rooms.browse'];
      const { container } = renderPermissionGrid({ permissions });

      const names = qAll(container, '[data-testid="permission-name"]');
      expect(names.length).toBe(2);
      // Sorted alphabetically
      expect(names[0].textContent).toBe('rooms.browse');
      expect(names[1].textContent).toBe('rooms.create');
    });

    it('exposes permission descriptions via the help tooltip', async () => {
      const { flushSync } = await import('svelte');
      const permissions = ['room.create'];
      const { container } = renderPermissionGrid({ permissions });

      // Description is rendered inside the HelpTooltip popover, which is
      // visible after the trigger button is clicked.
      const trigger = container.querySelector(
        'button[aria-expanded]'
      ) as HTMLButtonElement | null;
      if (!trigger) throw new Error('help tooltip trigger not rendered');
      trigger.click();
      flushSync();
      const tip = container.querySelector('[role="tooltip"]');
      expect(tip?.textContent?.trim()).toBe('Create new rooms');
    });

    it('renders permissions grouped by category, alphabetically within groups', async () => {
      const permissions = ['room.leave', 'room.create', 'room.join'];
      const { container } = renderPermissionGrid({ permissions });

      const names = qAll(container, '[data-testid="permission-name"]');
      expect(names[0].textContent).toBe('room.create');
      expect(names[1].textContent).toBe('room.join');
      expect(names[2].textContent).toBe('room.leave');
    });

    it('groups permissions by category with headers', async () => {
      const permissions = ['space.create', 'room.join', 'message.post'];
      const { container } = renderPermissionGrid({ permissions });

      // Each category renders as its own Panel — h2 carries the title now.
      const headers = qAll(container, 'h2');
      expect(headers.length).toBe(3);
      expect(headers[0].textContent?.trim()).toBe('Space Operations');
      expect(headers[1].textContent?.trim()).toBe('Room Operations');
      expect(headers[2].textContent?.trim()).toBe('Messages');
    });

    it('renders nothing when no permissions', async () => {
      const { container } = renderPermissionGrid({ permissions: [] });
      expect(buttonsFor(container).length).toBe(0);
    });
  });

  describe('three-state permissions', () => {
    it('marks Allow button as pressed for granted permissions', async () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        grantedPermissions: ['rooms.create']
      });

      const [allow, deny] = buttonsFor(container);
      expect(allow.getAttribute('aria-pressed')).toBe('true');
      expect(deny.getAttribute('aria-pressed')).toBe('false');
    });

    it('marks Deny button as pressed for denied permissions', async () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        deniedPermissions: ['rooms.create']
      });

      const [allow, deny] = buttonsFor(container);
      expect(allow.getAttribute('aria-pressed')).toBe('false');
      expect(deny.getAttribute('aria-pressed')).toBe('true');
    });

    it('neither button pressed for neutral permissions', async () => {
      const { container } = renderPermissionGrid({ permissions: ['rooms.create'] });
      const [allow, deny] = buttonsFor(container);
      expect(allow.getAttribute('aria-pressed')).toBe('false');
      expect(deny.getAttribute('aria-pressed')).toBe('false');
    });

    it('shows appropriate styling for allowed state', async () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        grantedPermissions: ['rooms.create']
      });
      const name = container.querySelector(
        '[data-testid="permission-name"].text-success'
      );
      expect(name?.textContent).toBe('rooms.create');
    });

    it('shows appropriate styling for denied state', async () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        deniedPermissions: ['rooms.create']
      });
      const name = container.querySelector(
        '[data-testid="permission-name"].text-danger'
      );
      expect(name?.textContent).toBe('rooms.create');
    });
  });

  describe('disabled state', () => {
    it('disables Allow + Deny buttons when disabled is true', async () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        disabled: true
      });

      const buttons = buttonsFor(container);
      for (const b of buttons) {
        expect(b.disabled).toBe(true);
      }
    });

    it('enables buttons when disabled is false', async () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        disabled: false
      });

      const buttons = buttonsFor(container);
      for (const b of buttons) {
        expect(b.disabled).toBe(false);
      }
    });
  });

  describe('updating state', () => {
    it('disables buttons for the permission being updated', async () => {
      const permissions = ['rooms.browse', 'rooms.create'];
      const { container } = renderPermissionGrid({
        permissions,
        updatingPermission: 'rooms.create'
      });

      // After alphabetical sorting: rooms.browse → buttons[0,1]; rooms.create → buttons[2,3]
      const buttons = buttonsFor(container);
      expect(buttons[0].disabled).toBe(false);
      expect(buttons[1].disabled).toBe(false);
      expect(buttons[2].disabled).toBe(true);
      expect(buttons[3].disabled).toBe(true);
    });

    it('adds pulse animation to row being updated', async () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        updatingPermission: 'rooms.create'
      });
      expect(container.querySelector('.animate-pulse')).not.toBeNull();
    });
  });

  describe('onSetState callback', () => {
    it('calls onSetState with neutral when toggling off Allow', async () => {
      const onSetState = vi.fn();
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        grantedPermissions: ['rooms.create'],
        onSetState
      });

      const [allow] = buttonsFor(container);
      allow.click();
      expect(onSetState).toHaveBeenCalledWith('rooms.create', 'neutral');
    });

    it('calls onSetState with allow when clicking Allow', async () => {
      const onSetState = vi.fn();
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        onSetState
      });

      const [allow] = buttonsFor(container);
      allow.click();
      expect(onSetState).toHaveBeenCalledWith('rooms.create', 'allow');
    });

    it('calls onSetState with deny when clicking Deny', async () => {
      const onSetState = vi.fn();
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        onSetState
      });

      const [, deny] = buttonsFor(container);
      deny.click();
      expect(onSetState).toHaveBeenCalledWith('rooms.create', 'deny');
    });

    it('calls onSetState with neutral when toggling off Deny', async () => {
      const onSetState = vi.fn();
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        deniedPermissions: ['rooms.create'],
        onSetState
      });

      const [, deny] = buttonsFor(container);
      deny.click();
      expect(onSetState).toHaveBeenCalledWith('rooms.create', 'neutral');
    });
  });
});
