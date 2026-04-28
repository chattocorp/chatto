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
    inheritedPermissions: string[];
    inheritedDenials: string[];
    inheritedFromLabel: string | undefined;
    disabled: boolean;
    updatingPermission: string | null;
    onSetState: (permission: string, state: PermissionState) => void;
  }>
) {
  const defaultProps = {
    permissions: [],
    grantedPermissions: [],
    deniedPermissions: [],
    inheritedPermissions: [],
    inheritedDenials: [],
    inheritedFromLabel: undefined,
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
      // visible after the trigger button is clicked. The trigger has
      // aria-label="What does room.create do?" (set by PermissionGrid).
      const trigger = container.querySelector(
        'button[aria-label^="What does"]'
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

  describe('inheritance', () => {
    function pill(container: Element): HTMLElement | null {
      // The inheritance badge is the only Pill rendered in PermissionGrid rows.
      return container.querySelector('span[class*="rounded"][class*="px-2"][class*="text-success"], span[class*="rounded"][class*="px-2"][class*="text-danger"]');
    }

    it('shows "Allow from <label>" when no override and inherited=allow', () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        inheritedPermissions: ['rooms.create'],
        inheritedFromLabel: 'space'
      });

      const badge = pill(container);
      expect(badge?.textContent).toContain('Allow from space');
      expect(badge?.classList.contains('line-through')).toBe(false);
    });

    it('shows "Deny from <label>" when no override and inherited=deny', () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        inheritedDenials: ['rooms.create'],
        inheritedFromLabel: 'instance'
      });

      const badge = pill(container);
      expect(badge?.textContent).toContain('Deny from instance');
    });

    it('renders the badge dimmed when an override at this scope is set', () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        // Override deny on top of inherited allow.
        deniedPermissions: ['rooms.create'],
        inheritedPermissions: ['rooms.create'],
        inheritedFromLabel: 'space'
      });

      const badge = pill(container);
      expect(badge).not.toBeNull();
      expect(badge?.classList.contains('line-through')).toBe(true);
      expect(badge?.classList.contains('opacity-50')).toBe(true);
    });

    it('hides the inherited badge when inheritedFromLabel is not provided', () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        inheritedPermissions: ['rooms.create']
        // inheritedFromLabel intentionally omitted
      });

      expect(pill(container)).toBeNull();
    });

    it('colors the identifier by inherited state when no override is set', () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        inheritedPermissions: ['rooms.create'],
        inheritedFromLabel: 'space'
      });

      // Effective = inherited 'allow' even though there's no override at this scope.
      const name = container.querySelector(
        '[data-testid="permission-name"].text-success'
      );
      expect(name?.textContent).toBe('rooms.create');
    });

    it('override wins over inherited when coloring the identifier', () => {
      const { container } = renderPermissionGrid({
        permissions: ['rooms.create'],
        deniedPermissions: ['rooms.create'],
        inheritedPermissions: ['rooms.create'],
        inheritedFromLabel: 'space'
      });

      // Override deny ⇒ identifier turns red, not green.
      const name = container.querySelector(
        '[data-testid="permission-name"].text-danger'
      );
      expect(name?.textContent).toBe('rooms.create');
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
