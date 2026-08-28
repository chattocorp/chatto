import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { RoomKind } from '@chatto/api-types/api/v1/rooms_pb';
import { TimeFormat } from '@chatto/api-types/api/v1/viewer_pb';
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

import { q } from '$lib/test-utils';
import { getToasts, toast } from '$lib/ui/toast';
import UserContextMenu from './UserContextMenu.svelte';

const liveProfileState = { bio: null as string | null, timezone: null as string | null };
const viewerSettingsState = {
  timezone: 'Europe/Berlin',
  timeFormat: TimeFormat.TIME_FORMAT_24_HOUR
};
const serverScopeMock = vi.hoisted(() => ({
  serverId: 'server-1',
  permissions: {
    loaded: true,
    canAdminViewUsers: false,
    canStartDMs: true
  },
  currentUser: { user: { id: 'viewer-1' } },
  projection: { rooms: new Map() }
}));

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (serverId: string) => `${serverId}.example.test`
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
      serverId: serverScopeMock.serverId,
      store: {
        permissions: serverScopeMock.permissions,
        currentUser: serverScopeMock.currentUser,
        projection: serverScopeMock.projection
      }
  })
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveBio: (_userId: string, fallback: string | null) => liveProfileState.bio ?? fallback,
  getLiveTimezone: (_userId: string, fallback: string | null) =>
    liveProfileState.timezone ?? fallback,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback,
  getLiveLogin: (_userId: string, fallback: string) => fallback,
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback
}));

vi.mock('$lib/state/presenceCache.svelte', () => ({
  getPresenceCache: () => ({
    get: (_scope: { serverId: string; userId: string }, fallback: string) => fallback
  })
}));

const user = {
  id: 'user-1',
  login: 'alice',
  displayName: 'Alice Example',
  avatarUrl: null,
  presenceStatus: PresenceStatus.ONLINE,
  customStatus: null
};

let originalShowPopover: typeof HTMLElement.prototype.showPopover;
const writeClipboardText = vi.fn();

function buttonWithText(container: HTMLElement, text: string): HTMLButtonElement | null {
  return (
    Array.from(container.querySelectorAll('button')).find(
      (button) => button.textContent?.trim() === text
    ) ?? null
  );
}

function linkWithText(container: HTMLElement, text: string): HTMLAnchorElement | null {
  return (
    Array.from(container.querySelectorAll('a')).find((link) => link.textContent?.trim() === text) ??
    null
  );
}

function renderMenu(props: Record<string, unknown> = {}) {
  return render(UserContextMenu, {
    props: {
      user,
      anchorRect: { top: 10, bottom: 30, left: 20 },
      viewerSettings: viewerSettingsState,
      onClose: vi.fn(),
      ...props
    }
  });
}

beforeAll(() => {
  originalShowPopover = HTMLElement.prototype.showPopover;
  HTMLElement.prototype.showPopover = function showPopover() {
    this.setAttribute('popover-open', '');
  };
});

afterAll(() => {
  HTMLElement.prototype.showPopover = originalShowPopover;
});

beforeEach(() => {
  vi.restoreAllMocks();
  serverScopeMock.serverId = 'server-1';
  serverScopeMock.permissions.loaded = true;
  serverScopeMock.permissions.canAdminViewUsers = false;
  serverScopeMock.permissions.canStartDMs = true;
  serverScopeMock.currentUser.user = { id: 'viewer-1' };
  serverScopeMock.projection.rooms.clear();
  toast.clear();
  writeClipboardText.mockReset();
  writeClipboardText.mockResolvedValue(undefined);
  liveProfileState.bio = null;
  liveProfileState.timezone = null;
  viewerSettingsState.timeFormat = TimeFormat.TIME_FORMAT_24_HOUR;
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText: writeClipboardText },
    configurable: true
  });
});

describe('UserContextMenu', () => {
  it('renders the user profile content', async () => {
    const { container } = renderMenu();

    await expect.element(q(container, '[role="dialog"]')).toBeInTheDocument();
    expect(container.textContent).toContain('Alice Example');
    expect(container.textContent).toContain('@alice');
  });

  it('shows a bio and local time when the user shares them', () => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2025-04-27T14:30:00Z'));
    liveProfileState.bio = 'I build chat software.';
    liveProfileState.timezone = 'Europe/Berlin';
    const { container } = renderMenu();
    const dialog = q(container, '[role="dialog"]');

    expect(q(dialog ?? document.body, '[data-testid="user-bio"]')?.textContent).toBe(
      'I build chat software.'
    );
    expect(dialog?.textContent).toContain('Europe/Berlin');
    expect(dialog?.textContent).toContain('16:30');
  });

  it('uses the viewer preferred 12-hour format for another user local time', () => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2025-04-27T14:30:00Z'));
    viewerSettingsState.timeFormat = TimeFormat.TIME_FORMAT_12_HOUR;
    liveProfileState.timezone = 'Europe/Berlin';
    const { container } = renderMenu();

    expect(q(container, '[role="dialog"]')?.textContent).toMatch(/04:30\s*pm/i);
  });

  it('uses profile fields from the supplied user before a live update arrives', () => {
    vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2025-04-27T14:30:00Z'));
    const { container } = renderMenu({
      user: {
        ...user,
        bio: 'Cached profile bio',
        timezone: 'Europe/Berlin'
      }
    });

    expect(q(container, '[role="dialog"]')?.textContent).toContain('Cached profile bio');
    expect(q(container, '[role="dialog"]')?.textContent).toContain('Europe/Berlin');
    expect(q(container, '[role="dialog"]')?.textContent).toContain('16:30');
  });

  it('calls the supplied profile callback and closes the menu', () => {
    const onClose = vi.fn();
    const onOpenProfile = vi.fn();
    const { container } = renderMenu({ onClose, onOpenProfile });
    const viewProfile = buttonWithText(container, 'View profile');

    viewProfile?.click();

    expect(onOpenProfile).toHaveBeenCalledExactlyOnceWith('user-1');
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('disables profile navigation when no direct message exists and the viewer cannot create one', () => {
    serverScopeMock.permissions.canStartDMs = false;
    const onClose = vi.fn();
    const onOpenProfile = vi.fn();
    const { container } = renderMenu({ onClose, onOpenProfile });
    const viewProfile = buttonWithText(container, 'View profile');

    expect(viewProfile?.disabled).toBe(true);
    expect(viewProfile?.title).toBe('A direct message is required to view this profile.');
    viewProfile?.click();

    expect(onOpenProfile).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it('keeps profile navigation available for an existing direct message without DM-create permission', () => {
    serverScopeMock.permissions.canStartDMs = false;
    serverScopeMock.projection.rooms.set('dm-1', {
      room: { room: { kind: RoomKind.DM }, viewerState: { isMember: true } },
      memberUserIds: ['viewer-1', 'user-1']
    });
    const onOpenProfile = vi.fn();
    const { container } = renderMenu({ onOpenProfile });
    const viewProfile = buttonWithText(container, 'View profile');

    expect(viewProfile?.disabled).toBe(false);
    viewProfile?.click();

    expect(onOpenProfile).toHaveBeenCalledExactlyOnceWith('user-1');
  });

  it('omits the profile action outside a room', () => {
    const { container } = renderMenu();

    expect(buttonWithText(container, 'View profile')).toBeNull();
  });

  it('renders custom status as its own profile line', async () => {
    const { container } = renderMenu({
      user: {
        ...user,
        customStatus: {
          emoji: '🍜',
          text: 'chatto:status:out_for_lunch',
          expiresAt: null
        }
      }
    });

    await expect.element(q(container, '[role="dialog"]')).toBeInTheDocument();
    expect(container.querySelector('[role="dialog"] .flex-1 > .font-semibold')?.textContent).toBe(
      'Alice Example'
    );
    expect(q(container, '[aria-label="🍜 Out for lunch"]')).toBeTruthy();
    expect(container.textContent).toContain('Out for lunch');
  });

  it('shows Send Message only when allowed', async () => {
    const hidden = renderMenu({ canSendMessage: false });
    expect(hidden.container.textContent).not.toContain('Send Message');
    hidden.unmount();

    const visible = renderMenu({ canSendMessage: true });
    await expect.element(buttonWithText(visible.container, 'Send Message')).toBeInTheDocument();
  });

  it('shows the selected user admin page only when its permission is loaded and granted', async () => {
    serverScopeMock.permissions.loaded = false;
    serverScopeMock.permissions.canAdminViewUsers = true;
    const loading = renderMenu();
    expect(linkWithText(loading.container, 'View in Server Admin')).toBeNull();
    loading.unmount();

    serverScopeMock.permissions.loaded = true;
    serverScopeMock.permissions.canAdminViewUsers = false;
    const denied = renderMenu();
    expect(linkWithText(denied.container, 'View in Server Admin')).toBeNull();
    denied.unmount();

    serverScopeMock.permissions.canAdminViewUsers = true;
    const allowed = renderMenu();
    const adminLink = linkWithText(allowed.container, 'View in Server Admin');

    await expect.element(adminLink).toBeInTheDocument();
    expect(adminLink?.getAttribute('href')).toBe(
      '/chat/server-1.example.test/manage/server/members/user-1'
    );
  });

  it('closes when opening the selected user in Server Admin', () => {
    serverScopeMock.permissions.canAdminViewUsers = true;
    const onClose = vi.fn();
    const { container } = renderMenu({ onClose });
    const adminLink = linkWithText(container, 'View in Server Admin')!;
    adminLink.addEventListener('click', (event) => event.preventDefault());

    adminLink.click();

    expect(onClose).toHaveBeenCalledOnce();
  });

  it('separates the profile and actions with sibling menu surfaces', () => {
    serverScopeMock.permissions.canAdminViewUsers = true;
    const { container } = renderMenu({
      canSendMessage: true,
      canBanFromRoom: true,
      onOpenProfile: vi.fn()
    });
    const dialog = q(container, '[role="dialog"]')!;
    const sections = dialog.querySelectorAll('.menu-section');
    const actionLabels = Array.from(sections[1]!.querySelectorAll('button, a')).map((action) =>
      action.textContent?.trim()
    );

    expect(sections).toHaveLength(3);
    expect(sections[0]?.textContent).toContain('Alice Example');
    expect(actionLabels).toEqual([
      'Send Message',
      'View profile',
      'View in Server Admin',
      'Ban from room'
    ]);
    expect(sections[2]?.textContent).toContain('Copy User ID');
    expect(sections[0]?.parentElement).toBe(sections[1]?.parentElement);
    expect(sections[1]?.parentElement).toBe(sections[2]?.parentElement);
    expect(dialog.querySelector('[class~="border-t"], [role="separator"]')).toBeNull();
  });

  it('shows Copy User ID last and copies the exact user ID', async () => {
    const onClose = vi.fn();
    const { container } = renderMenu({ onClose });
    const copyUserId = q(container, '[data-testid="copy-user-id"]') as HTMLButtonElement;
    const dialogButtons = q(container, '[role="dialog"]')?.querySelectorAll('button');

    await expect.element(copyUserId).toHaveTextContent('Copy User ID');
    expect(dialogButtons?.item((dialogButtons?.length ?? 0) - 1)).toBe(copyUserId);

    copyUserId.click();

    await vi.waitFor(() => expect(writeClipboardText).toHaveBeenCalledWith('user-1'));
    expect(onClose).toHaveBeenCalledOnce();
    expect(writeClipboardText.mock.invocationCallOrder[0]).toBeLessThan(
      onClose.mock.invocationCallOrder[0]!
    );
    expect(getToasts().map((item) => item.message)).toContain('Copied to clipboard');
  });

  it('calls send and close callbacks when sending a message', () => {
    const onSendMessage = vi.fn();
    const onClose = vi.fn();
    const { container } = renderMenu({ canSendMessage: true, onSendMessage, onClose });

    buttonWithText(container, 'Send Message')?.click();

    expect(onSendMessage).toHaveBeenCalledOnce();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('closes on Escape', () => {
    const onClose = vi.fn();
    renderMenu({ onClose });

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));

    expect(onClose).toHaveBeenCalledOnce();
  });
});
