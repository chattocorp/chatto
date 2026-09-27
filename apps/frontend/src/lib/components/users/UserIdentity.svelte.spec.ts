import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync, tick } from 'svelte';
import { userEvent } from 'vitest/browser';
import { q } from '$lib/test-utils';
import UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
import UserIdentity from './UserIdentity.svelte';
import UserMenu from './UserMenu.svelte';
import { UserMenuState } from './UserMenuState.svelte';

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (serverId: string) => serverId
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    serverId: 'server-1',
    store: {
      permissions: {
        loaded: true,
        canAdminViewUsers: false,
        canStartDMs: true
      }
    }
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBio: () => null,
  getLiveTimezone: () => null,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveLogin: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

vi.mock('$lib/utils/inputCapabilities', () => ({
  prefersTouchActions: () => false,
  supportsHoverActions: () => true
}));

const user = {
  id: 'owner-1',
  login: 'alice',
  displayName: 'Alice Example',
  deleted: false,
  avatarUrl: null,
  presenceStatus: PresenceStatus.OFFLINE
};

const userContextMenuLoader = async () => ({ default: UserContextMenu });

let originalShowPopover: typeof HTMLElement.prototype.showPopover;
let originalShowModal: typeof HTMLDialogElement.prototype.showModal;

beforeAll(() => {
  originalShowPopover = HTMLElement.prototype.showPopover;
  originalShowModal = HTMLDialogElement.prototype.showModal;
  HTMLElement.prototype.showPopover = function showPopover() {
    this.setAttribute('popover-open', '');
  };
  HTMLDialogElement.prototype.showModal = function showModal() {
    this.setAttribute('open', '');
  };
});

afterAll(() => {
  HTMLElement.prototype.showPopover = originalShowPopover;
  HTMLDialogElement.prototype.showModal = originalShowModal;
});

describe('UserIdentity', () => {
  it('renders the shared avatar with the display name', () => {
    const { container } = render(UserIdentity, { props: { user, userContextMenuLoader } });

    expect(q(container, '[data-testid="user-identity"]')?.textContent).toContain('Alice Example');
    expect(q(container, '[role="img"][aria-label="alice"]')).toBeTruthy();
  });

  it('opens the shared profile from a clickable identity', async () => {
    const { container } = render(UserIdentity, {
      props: { user, openOnClick: true, size: 'xs', userContextMenuLoader }
    });
    const button = q(container, 'button[data-testid="user-identity"]')!;
    await expect.element(button).toHaveAttribute('aria-haspopup', 'dialog');
    button.click();
    await tick();
    await expect.element(q(container, '[role="dialog"]')).toBeInTheDocument();
    expect(q(container, '[role="dialog"]')?.textContent).toContain('Alice Example');
  });

  it('connects host-provided message and profile actions to the displayed user', async () => {
    const onSendMessage = vi.fn();
    const onOpenProfile = vi.fn();
    const { container } = render(UserIdentity, {
      props: { user, openOnClick: true, onSendMessage, onOpenProfile, userContextMenuLoader }
    });
    const button = q(container, 'button[data-testid="user-identity"]')!;
    button.click();
    await tick();
    const send = [...container.querySelectorAll('button')].find(
      (item) => item.textContent?.trim() === 'Send Message'
    );
    expect(send).toBeTruthy();
    send!.click();
    await tick();
    expect(onSendMessage).toHaveBeenCalledWith(user.id);
    button.click();
    await tick();
    const profile = [...container.querySelectorAll('button')].find(
      (item) => item.textContent?.trim() === 'View profile'
    );
    expect(profile).toBeTruthy();
    profile!.click();
    expect(onOpenProfile).toHaveBeenCalledWith(user.id);
  });

  it('opens the shared user profile on right-click', async () => {
    const { container } = render(UserIdentity, { props: { user, userContextMenuLoader } });
    await tick();
    const identity = q(container, '[data-testid="user-identity"]')!;

    identity.dispatchEvent(
      new MouseEvent('contextmenu', { bubbles: true, clientX: 40, clientY: 60 })
    );
    await tick();

    await expect.element(q(container, '[role="dialog"]')).toBeInTheDocument();
    expect(q(container, '[role="dialog"]')?.textContent).toContain('Alice Example');
  });

  it('opens the shared user profile as a sheet after a touch long-press', async () => {
    const { container } = render(UserIdentity, { props: { user, userContextMenuLoader } });
    await tick();
    vi.useFakeTimers();
    try {
      const identity = q(container, '[data-testid="user-identity"]')!;

      identity.dispatchEvent(
        new PointerEvent('pointerdown', {
          bubbles: true,
          pointerId: 7,
          pointerType: 'touch',
          isPrimary: true,
          clientX: 40,
          clientY: 60
        })
      );
      await vi.advanceTimersByTimeAsync(500);
      flushSync();

      expect(q(container, 'dialog.bottom-sheet[open]')).toBeTruthy();
      expect(q(container, 'dialog')?.textContent).toContain('Alice Example');
    } finally {
      vi.useRealTimers();
    }
  });
});

describe('UserMenu lifecycle', () => {
  it('does not render a menu after it closes while its module loads', async () => {
    let resolveModule!: (module: { default: typeof UserContextMenu }) => void;
    const loader = vi.fn(
      () => new Promise<{ default: typeof UserContextMenu }>((resolve) => (resolveModule = resolve))
    );
    const state = new UserMenuState<string>();
    const { container } = render(UserMenu, { props: { state, user, loader } });
    expect(loader).not.toHaveBeenCalled();

    const trigger = document.createElement('button');
    trigger.onclick = (event) => state.open(user.id, event);
    trigger.click();
    await tick();
    expect(loader).toHaveBeenCalledOnce();
    await userEvent.keyboard('{Escape}');
    await tick();
    expect(state.target).toBeNull();
    resolveModule({ default: UserContextMenu });
    await tick();
    expect(q(container, '[role="dialog"]')).toBeNull();

    // A later open must work after the pending menu was dismissed.
    loader.mockResolvedValue({ default: UserContextMenu });
    trigger.click();
    await tick();
    await expect.element(q(container, '[role="dialog"]')).toBeInTheDocument();
  });

  it('lets the user retry a failed menu load', async () => {
    const loader = vi.fn(userContextMenuLoader).mockRejectedValueOnce(new Error('Load failed'));
    const state = new UserMenuState<string>();
    const screen = render(UserMenu, { props: { state, user, loader } });
    const trigger = document.createElement('button');
    trigger.onclick = (event) => state.open(user.id, event);
    trigger.click();

    await expect.poll(() => q(screen.container, '[role="alertdialog"]')).not.toBeNull();
    q(screen.container, '[role="alertdialog"] button')!.click();
    await expect.element(screen.getByTestId('copy-user-id')).toBeInTheDocument();
    expect(loader).toHaveBeenCalledTimes(2);
  });

  it('uses the latest target and user when a pending menu load completes', async () => {
    let resolveModule!: (module: { default: typeof UserContextMenu }) => void;
    const pending = new Promise<{ default: typeof UserContextMenu }>(
      (resolve) => (resolveModule = resolve)
    );
    const state = new UserMenuState<string>();
    const screen = render(UserMenu, { props: { state, user, loader: () => pending } });
    const trigger = document.createElement('button');
    trigger.onclick = (event) => state.open(user.id, event);
    trigger.click();
    await tick();

    const nextUser = { ...user, id: 'owner-2', login: 'bob', displayName: 'Bob Example' };
    trigger.onclick = (event) => state.open(nextUser.id, event);
    trigger.click();
    await screen.rerender({ user: nextUser });
    resolveModule({ default: UserContextMenu });
    await expect.element(screen.getByTestId('copy-user-id')).toBeInTheDocument();
    expect(state.target).toBe(nextUser.id);
    expect(screen.container.textContent).toContain('Bob Example');
    expect(screen.container.textContent).not.toContain('Alice Example');
  });

  it('replaces point placement with a button anchor and toggles the selected target closed', async () => {
    const state = new UserMenuState<string>();
    const { container } = render(UserMenu, {
      props: { state, user, loader: userContextMenuLoader }
    });
    const trigger = document.createElement('button');
    const cleanup = state.trigger(() => user.id)(trigger);
    try {
      trigger.dispatchEvent(new MouseEvent('contextmenu', { clientX: 40, clientY: 60 }));
      await tick();
      expect(state.selection?.position).toEqual({ x: 40, y: 60 });

      trigger.onclick = (event) => state.open(user.id, event);
      trigger.click();
      await tick();
      expect(state.selection?.position).toBeUndefined();
      expect(state.selection?.anchorRect).toBeDefined();
      await expect.element(q(container, '[role="dialog"]')).toBeInTheDocument();

      trigger.onclick = (event) => state.toggle(user.id, event);
      trigger.click();
      await tick();
      expect(state.target).toBeNull();
      expect(q(container, '[role="dialog"]')).toBeNull();
    } finally {
      cleanup?.();
    }
  });
});
