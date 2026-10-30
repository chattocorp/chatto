import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { RealtimeProjectionUpdate } from '@chatto/client/realtime/eventBus';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import { ListRoomsResponse, RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { RealtimeResourceUpdate } from '@chatto/client/api/realtimeResources';
import { RoomDeletedEvent } from '@chatto/api-types/realtime/v1/events_pb';
import { RealtimeEvent as PublicRealtimeEvent } from '@chatto/api-types/realtime/v1/realtime_pb';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { queryClient } from '$lib/query/client';
import { adminQueryKeys } from '$lib/query/admin';
import { removeRegisteredAdminQueries } from '$lib/query/cacheRegistry';
import type { AdminManagedRoom } from '$lib/api/adminRoomLayout';
import { createTestServerScope, type TestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  getRoom: vi.fn(),
  goto: vi.fn(),
  beforeNavigate: vi.fn(),
  listRoomMembers: vi.fn(),
  projectionHandlers: [] as Array<(event: RealtimeProjectionUpdate) => void>,
  updateRoom: vi.fn(),
  refreshLayout: vi.fn(),
  success: vi.fn(),
  error: vi.fn()
}));

// Page titles are tested separately from this page's partial route/server fixtures.
// The store mock also carries the frontend UI state of its server.
vi.mock(
  '$lib/state/server/serverUi',
  async () => (await import('$lib/test-utils/serverUiMock')).serverUiIsStore
);

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    isOriginServer: () => false,
    getServer: (serverId: string) => ({ id: serverId, url: `https://${serverId}.example.test` }),
    tryGetStore: () => server.scope.store,
    getStore: () => ({})
  }
}));

vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$app/state', () => ({
  page: {
    get params() {
      return { serverId: server.serverId, roomId: 'shared-room' };
    },
    get route() {
      return { id: routeId };
    }
  }
}));

vi.mock('$app/navigation', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$app/navigation')>()),
  goto: mocks.goto,
  beforeNavigate: mocks.beforeNavigate
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => server.serverId
}));

vi.mock('$lib/hooks', () => ({
  useProjectionEvent: (handler: (event: RealtimeProjectionUpdate) => void) => {
    mocks.projectionHandlers.push(handler);
  }
}));

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);

let server: TestServerScope;
let routeId = '/chat/[serverId]/manage/rooms/[roomId]';

type Section = 'general' | 'members' | 'permissions';

/** Renders the sections on the route of the first one. */
function renderSections(...sections: Section[]) {
  routeId =
    sections[0] === 'general'
      ? '/chat/[serverId]/manage/rooms/[roomId]'
      : `/chat/[serverId]/manage/rooms/[roomId]/${sections[0]}`;
  return render(RoomManagementTestHarness, { props: { sections } });
}

vi.mock('$lib/api/adminRoomLayout', () => ({
  createAdminRoomLayoutAPI: ({ serverId }: { serverId: string }) => ({
    getRoom: (roomId: string, options?: { signal?: AbortSignal }) =>
      mocks.getRoom(serverId, roomId, options)
  })
}));

vi.mock('@chatto/client/api/memberDirectory', () => ({
  createMemberDirectoryAPI: () => ({
    listRoomMembers: mocks.listRoomMembers,
    listUsers: () => Promise.resolve({ members: [], totalCount: 0, hasMore: false }),
    batchGetRoomMembers: () => Promise.resolve([])
  })
}));

vi.mock('@chatto/client/api/rooms', () => ({
  createRoomCommandAPI: () => ({
    updateRoom: mocks.updateRoom,
    addMember: vi.fn(),
    removeMember: vi.fn()
  })
}));

vi.mock('$lib/components/rbac/PermissionMatrix.svelte', async () => ({
  default: (await import('./RoomManagementPagePermissionMatrixMock.svelte')).default
}));

vi.mock('$lib/ui/toast', () => ({
  toast: { success: mocks.success, error: mocks.error }
}));

import RoomManagementTestHarness from './RoomManagementTestHarness.svelte';
import { RoomThreadingMode } from '@chatto/client/util/roomThreading';

function managedRoom(
  name: string,
  overrides: Partial<{
    archived: boolean;
    isUniversal: boolean;
    threadSlowModeSeconds: number;
    threadingMode: RoomThreadingMode;
    showMembershipEvents: boolean;
    canManageRoom: boolean;
    canManagePermissions: boolean;
  }> = {}
) {
  return {
    id: 'shared-room',
    name,
    description: null,
    archived: overrides.archived ?? false,
    isUniversal: overrides.isUniversal ?? false,
    slowModeSeconds: 0,
    threadSlowModeSeconds: overrides.threadSlowModeSeconds,
    threadingMode: overrides.threadingMode ?? RoomThreadingMode.ENABLED,
    showMembershipEvents: overrides.showMembershipEvents,
    canManageRoom: overrides.canManageRoom ?? true,
    canManagePermissions: overrides.canManagePermissions ?? true
  };
}

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  flushSync();
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

/** Finds an action only in the form that owns the given control. */
function formAction(
  container: HTMLElement,
  control: string,
  label = 'Save changes'
): HTMLButtonElement {
  const form = container.querySelector(control)?.closest('form');
  const button = [...(form?.querySelectorAll('button') ?? [])].find(
    (element) => element.textContent?.trim() === label
  );
  if (!button) throw new Error(`Missing ${label} action for ${control}`);
  return button;
}

function submitSection(container: HTMLElement, control: string): void {
  flushSync();
  formAction(container, control).click();
}

function dispatchProjection(event: RealtimeProjectionUpdate): void {
  for (const handler of mocks.projectionHandlers) handler(event);
}

type RoomPatch = {
  name?: string;
  description?: string | null;
  universal?: boolean;
  slowModeSeconds?: number;
  threadSlowModeSeconds?: number;
  threadingMode?: RoomThreadingMode;
  showMembershipEvents?: boolean;
};

/**
 * Serves one room like the server: an update applies its patch, and later
 * reads return the updated room. `hold` delays the response of the next
 * update until the returned function is called.
 */
function serveRoom(initial: ReturnType<typeof managedRoom>) {
  let stored: Omit<typeof initial, 'description'> & { description: string | null } = {
    ...initial
  };
  const held: Array<() => void> = [];
  let holdNext = false;
  mocks.getRoom.mockImplementation(() => Promise.resolve({ ...stored }));
  mocks.updateRoom.mockImplementation((input: RoomPatch & { roomId: string }) => {
    stored = {
      ...stored,
      ...(input.name !== undefined && { name: input.name }),
      ...(input.description !== undefined && { description: input.description }),
      ...(input.universal !== undefined && { isUniversal: input.universal }),
      ...(input.slowModeSeconds !== undefined && { slowModeSeconds: input.slowModeSeconds }),
      ...(input.threadSlowModeSeconds !== undefined && {
        threadSlowModeSeconds: input.threadSlowModeSeconds
      }),
      ...(input.threadingMode !== undefined && { threadingMode: input.threadingMode }),
      ...(input.showMembershipEvents !== undefined && {
        showMembershipEvents: input.showMembershipEvents
      })
    };
    const response = {
      id: stored.id,
      name: stored.name,
      description: stored.description ?? '',
      universal: stored.isUniversal,
      slowModeSeconds: stored.slowModeSeconds,
      threadSlowModeSeconds: stored.threadSlowModeSeconds,
      threadingMode: stored.threadingMode,
      showMembershipEvents: stored.showMembershipEvents,
      archived: stored.archived
    };
    if (!holdNext) return Promise.resolve(response);
    holdNext = false;
    return new Promise((resolve) => held.push(() => resolve(response)));
  });
  return {
    hold() {
      holdNext = true;
      return () => held.shift()?.();
    }
  };
}

function roomSnapshot(present = true): RealtimeProjectionUpdate {
  return new RealtimeProjectionUpdate({
    resource: new RealtimeResourceUpdate({
      resource: {
        case: 'rooms',
        value: new ListRoomsResponse({
          rooms: present
            ? [
                new RoomWithViewerState({
                  room: new Room({ id: 'shared-room', name: 'general' })
                })
              ]
            : []
        })
      }
    })
  });
}

function roomRemoved(): RealtimeProjectionUpdate {
  return new RealtimeProjectionUpdate({
    event: new PublicRealtimeEvent({
      event: {
        case: 'roomDeleted',
        value: new RoomDeletedEvent({ roomId: 'shared-room' })
      }
    })
  });
}

describe('room management page identity and realtime authority', () => {
  it('saves only the membership display setting immediately and blocks repeated clicks', async () => {
    serveRoom(managedRoom('general', { showMembershipEvents: true }));
    const pending = deferred<Room>();
    mocks.updateRoom.mockReturnValueOnce(pending.promise);
    const view = renderSections('general');
    await expect.element(view.getByLabelText('Show join/leave events')).toBeChecked();
    const checkbox = view.container.querySelector(
      '#room-settings-membership-events'
    ) as HTMLInputElement;
    checkbox.click();
    await vi.waitFor(() =>
      expect(mocks.updateRoom).toHaveBeenCalledWith({
        roomId: 'shared-room',
        showMembershipEvents: false
      })
    );
    await expect.element(view.getByLabelText('Show join/leave events')).toBeDisabled();
    checkbox.click();
    expect(mocks.updateRoom).toHaveBeenCalledOnce();
    expect(checkbox.closest('form')).toBeNull();
    const cancel = vi.fn();
    mocks.beforeNavigate.mock.calls.at(-1)![0]({ type: 'leave', cancel });
    expect(cancel).not.toHaveBeenCalled();

    pending.resolve(new Room({ id: 'shared-room', showMembershipEvents: false }));
    await expect.element(view.getByLabelText('Show join/leave events')).not.toBeChecked();
    await expect.element(view.getByLabelText('Show join/leave events')).toBeEnabled();
    expect(mocks.success).toHaveBeenCalledOnce();
  });

  it('keeps the server membership display setting after failure and permits retry', async () => {
    serveRoom(managedRoom('general', { showMembershipEvents: true }));
    mocks.updateRoom.mockRejectedValueOnce(new Error('Save failed'));
    const view = renderSections('general');
    const checkbox = view.getByLabelText('Show join/leave events');
    await checkbox.click();
    await vi.waitFor(() => expect(mocks.error).toHaveBeenCalledOnce());
    await expect.element(checkbox).toBeChecked();
    await expect.element(checkbox).toBeEnabled();
    await checkbox.click();
    await vi.waitFor(() => expect(mocks.updateRoom).toHaveBeenCalledTimes(2));
    await expect.element(checkbox).not.toBeChecked();
    await expect.element(checkbox).toBeEnabled();
    expect(mocks.success).toHaveBeenCalledOnce();
  });

  it('does not restore the room after leaving during a membership display save', async () => {
    serveRoom(managedRoom('general', { showMembershipEvents: true }));
    const pending = deferred<Room>();
    mocks.updateRoom.mockReturnValueOnce(pending.promise);
    const view = renderSections('general');
    await view.getByLabelText('Show join/leave events').click();
    await vi.waitFor(() => expect(mocks.updateRoom).toHaveBeenCalledOnce());
    removeRegisteredAdminQueries('server-a');
    view.unmount();
    pending.resolve(new Room({ id: 'shared-room', showMembershipEvents: false }));
    await settle();
    expect(mocks.success).not.toHaveBeenCalled();
    expect(
      queryClient.getQueryData(
        adminQueryKeys.room('server-a', server.scope.connection, 'shared-room')
      )
    ).toBeUndefined();
  });

  it('disables membership display editing on older servers', async () => {
    serveRoom(managedRoom('general'));
    const view = renderSections('general');
    await expect.element(view.getByLabelText('Show join/leave events')).toBeDisabled();
    await expect
      .element(view.getByText('Update this server to change how membership activity is shown.'))
      .toBeVisible();
  });
  afterEach(() => vi.restoreAllMocks());

  beforeEach(async () => {
    queryClient.clear();
    vi.clearAllMocks();
    mocks.projectionHandlers = [];
    server = createTestServerScope({
      serverId: 'server-a',
      permissions: { canManageRooms: true, canAdminManageRoles: true },
      serverInfo: { limitedFeatures: ['independentSlowMode'] },
      store: { adminRoomLayout: { refresh: mocks.refreshLayout } }
    });
    mocks.refreshLayout.mockResolvedValue(undefined);
    mocks.goto.mockResolvedValue(undefined);
    mocks.listRoomMembers.mockResolvedValue({ members: [], totalCount: 0, hasMore: false });
    mocks.updateRoom.mockResolvedValue({
      id: 'shared-room',
      name: 'general',
      description: '',
      universal: false,
      slowModeSeconds: 0,
      threadingMode: RoomThreadingMode.ENABLED,
      archived: false
    });
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('reconciles room rules and permissions after a realtime room update', async () => {
    mocks.getRoom.mockResolvedValueOnce(managedRoom('general')).mockResolvedValueOnce(
      managedRoom('remote-name', {
        archived: true,
        isUniversal: true
      })
    );
    const { container } = renderSections('general', 'members');
    await settle();
    expect(container.querySelector('#room-member-picker')).not.toBeNull();

    dispatchProjection(roomSnapshot());
    await settle();

    expect(container.querySelector('#room-member-picker')).toBeNull();
    expect(container.textContent).toContain('Membership is automatic in Universal rooms.');
    expect((container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'remote-name'
    );
  });

  it('reuses a fresh room snapshot and preserves a dirty draft across projection refreshes', async () => {
    mocks.getRoom.mockResolvedValueOnce(managedRoom('general')).mockResolvedValueOnce(
      managedRoom('remote-name', {
        isUniversal: true
      })
    );
    const first = renderSections('general', 'members');
    await settle();

    const nameInput = first.container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'local-draft';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    dispatchProjection(roomSnapshot());
    await vi.waitFor(() => expect(mocks.getRoom).toHaveBeenCalledTimes(2));
    await settle();

    expect((first.container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'local-draft'
    );
    expect(first.container.textContent).toContain('Membership is automatic in Universal rooms.');
    first.unmount();

    mocks.getRoom.mockResolvedValue(managedRoom('remote-name', { isUniversal: true }));
    const second = renderSections('general', 'members');
    await settle();
    expect(mocks.getRoom).toHaveBeenCalledTimes(3);
    expect((second.container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'remote-name'
    );
  });

  it('accepts spaces, punctuation, emoji, and normalizes Unicode room names', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const nameInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'Team chat 💬 / Ku\u0308che!';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    const submit = container.querySelector('form button[type="submit"]') as HTMLButtonElement;
    expect(submit.disabled).toBe(false);
    submit.click();

    await vi.waitFor(() => {
      expect(mocks.updateRoom).toHaveBeenCalledWith({
        roomId: 'shared-room',
        name: 'Team chat 💬 / Küche!'
      });
    });
  });

  it('saves a numeric slow-mode draft only on submission as a sparse patch', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();
    expect(container.querySelector('#room-settings-thread-slow-mode')).toBeNull();
    expect(container.querySelector('a[href$="/info#independentSlowMode"]')).not.toBeNull();

    const select = container.querySelector('#room-settings-slow-mode') as HTMLSelectElement;
    select.value = '10';
    select.dispatchEvent(new Event('change', { bubbles: true }));

    await settle();
    expect(mocks.updateRoom).not.toHaveBeenCalled();
    expect(select.value).toBe('10');
    submitSection(container, '#room-settings-slow-mode');

    await vi.waitFor(() => {
      expect(mocks.updateRoom).toHaveBeenCalledWith({
        roomId: 'shared-room',
        slowModeSeconds: 10
      });
    });
  });

  it('saves independent thread slow mode including explicit Off without changing the channel interval', async () => {
    serveRoom(managedRoom('general', { threadSlowModeSeconds: 60 }));
    const { container } = renderSections('general');
    await settle();
    const thread = container.querySelector('#room-settings-thread-slow-mode') as HTMLSelectElement;
    const root = container.querySelector('#room-settings-slow-mode') as HTMLSelectElement;
    expect(container.querySelector('[data-testid="feature-compatibility-notice"]')).toBeNull();
    expect(thread.value).toBe('60');
    thread.value = '0';
    thread.dispatchEvent(new Event('change', { bubbles: true }));
    await settle();
    expect(mocks.updateRoom).not.toHaveBeenCalled();
    submitSection(container, '#room-settings-thread-slow-mode');
    await vi.waitFor(() =>
      expect(mocks.updateRoom).toHaveBeenCalledWith({
        roomId: 'shared-room',
        threadSlowModeSeconds: 0
      })
    );
    await vi.waitFor(() => expect(thread.value).toBe('0'));
    expect(root.value).toBe('0');
  });

  it('discards both slow-mode drafts together and submits both changed intervals in one request', async () => {
    serveRoom(managedRoom('general', { threadSlowModeSeconds: 60 }));
    const { container } = renderSections('general');
    await settle();
    const channel = container.querySelector('#room-settings-slow-mode') as HTMLSelectElement;
    const thread = container.querySelector('#room-settings-thread-slow-mode') as HTMLSelectElement;
    function editIntervals() {
      channel.value = '10';
      channel.dispatchEvent(new Event('change', { bubbles: true }));
      thread.value = '0';
      thread.dispatchEvent(new Event('change', { bubbles: true }));
    }
    editIntervals();
    await settle();
    expect(mocks.updateRoom).not.toHaveBeenCalled();
    formAction(container, '#room-settings-slow-mode', 'Discard changes').click();
    await settle();
    expect(channel.value).toBe('0');
    expect(thread.value).toBe('60');
    editIntervals();
    submitSection(container, '#room-settings-slow-mode');
    await vi.waitFor(() =>
      expect(mocks.updateRoom).toHaveBeenCalledExactlyOnceWith({
        roomId: 'shared-room',
        slowModeSeconds: 10,
        threadSlowModeSeconds: 0
      })
    );
    await vi.waitFor(() =>
      expect(formAction(container, '#room-settings-slow-mode').disabled).toBe(true)
    );
    expect(channel.value).toBe('10');
    expect(thread.value).toBe('0');
  });

  it('saves a threading mode only when its form submits', async () => {
    serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const choices = Array.from(container.querySelectorAll<HTMLButtonElement>('[role="radio"]'));
    expect(choices).toHaveLength(4);
    expect(choices[2]).toHaveAttribute('aria-checked', 'true');
    choices[0].click();

    await settle();
    expect(mocks.updateRoom).not.toHaveBeenCalled();
    expect(choices[0]).toHaveAttribute('aria-checked', 'true');
    submitSection(container, '[role="radio"]');

    await vi.waitFor(() => {
      expect(mocks.updateRoom).toHaveBeenCalledWith({
        roomId: 'shared-room',
        threadingMode: RoomThreadingMode.REQUIRED
      });
    });
    await vi.waitFor(() => expect(choices[0]).toHaveAttribute('aria-checked', 'true'));
  });

  it('rejects invisible-only room names', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const nameInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = '\u200d\u2060';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    expect(
      (container.querySelector('form button[type="submit"]') as HTMLButtonElement).disabled
    ).toBe(true);
    expect(container.textContent).toContain('Room name cannot be empty');
    expect(mocks.updateRoom).not.toHaveBeenCalled();
  });

  it('purges room metadata synchronously when realtime removes access', async () => {
    mocks.getRoom.mockResolvedValueOnce(managedRoom('private-room'));
    const pendingReload = deferred<ReturnType<typeof managedRoom>>();
    const { container } = renderSections('general');
    await settle();
    expect(container.textContent).toContain('#private-room');

    mocks.getRoom.mockReturnValueOnce(pendingReload.promise);
    dispatchProjection(roomRemoved());
    flushSync();

    expect(container.textContent).not.toContain('#private-room');
    expect(container.querySelector('#room-settings-name')).toBeNull();

    pendingReload.resolve(managedRoom('private-room'));
    await settle();
  });

  it('revalidates archived room members only after the admin room reread succeeds', async () => {
    mocks.getRoom
      .mockResolvedValueOnce(managedRoom('general'))
      .mockResolvedValueOnce(managedRoom('general', { archived: true }));
    const { container } = renderSections('members');
    await vi.waitFor(() => expect(mocks.listRoomMembers).toHaveBeenCalledOnce());

    dispatchProjection(roomRemoved());
    flushSync();

    expect(mocks.listRoomMembers).toHaveBeenCalledOnce();
    await vi.waitFor(() => expect(mocks.getRoom).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(mocks.listRoomMembers).toHaveBeenCalledTimes(2));
    expect(container.textContent).toContain(
      'Membership cannot be changed while this room is archived.'
    );
  });

  it('does not reopen member reads when the admin room reread confirms deletion', async () => {
    const deletedRoom = deferred<AdminManagedRoom | null>();
    mocks.getRoom
      .mockResolvedValueOnce(managedRoom('general'))
      .mockReturnValueOnce(deletedRoom.promise);
    const { container } = renderSections('members');
    await vi.waitFor(() => expect(mocks.listRoomMembers).toHaveBeenCalledOnce());

    dispatchProjection(roomRemoved());
    flushSync();
    expect(mocks.listRoomMembers).toHaveBeenCalledOnce();

    deletedRoom.resolve(null);
    await vi.waitFor(() =>
      expect(container.textContent).toContain('You do not have permission to access this page.')
    );
    expect(mocks.listRoomMembers).toHaveBeenCalledOnce();
  });

  it('ignores a stale admin reread superseded by a later room removal', async () => {
    const staleRoom = deferred<AdminManagedRoom | null>();
    const deletedRoom = deferred<AdminManagedRoom | null>();
    mocks.getRoom
      .mockResolvedValueOnce(managedRoom('general'))
      .mockReturnValueOnce(staleRoom.promise)
      .mockReturnValueOnce(deletedRoom.promise);
    renderSections('members');
    await vi.waitFor(() => expect(mocks.listRoomMembers).toHaveBeenCalledOnce());

    const removal = () => dispatchProjection(roomRemoved());
    removal();
    await vi.waitFor(() => expect(mocks.getRoom).toHaveBeenCalledTimes(2));
    removal();
    await vi.waitFor(() => expect(mocks.getRoom).toHaveBeenCalledTimes(3));

    staleRoom.resolve(managedRoom('stale-room', { archived: true }));
    await settle();
    expect(mocks.listRoomMembers).toHaveBeenCalledOnce();

    deletedRoom.resolve(null);
    await settle();
    expect(mocks.listRoomMembers).toHaveBeenCalledOnce();
  });

  it('clears saving after a realtime refresh supersedes the save response', async () => {
    const pendingSave = deferred<{
      id: string;
      name: string;
      description: string;
      universal: boolean;
      archived: boolean;
    }>();
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    mocks.updateRoom.mockReturnValueOnce(pendingSave.promise);
    const { container } = renderSections('general');
    await settle();

    const nameInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'renamed';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (container.querySelector('form button[type="submit"]') as HTMLButtonElement).click();
    await settle();

    dispatchProjection(roomSnapshot());
    await settle();
    pendingSave.resolve({
      id: 'shared-room',
      name: 'renamed',
      description: '',
      universal: false,
      archived: false
    });
    await settle();

    const refreshedInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    await vi.waitFor(() => expect(refreshedInput.disabled).toBe(false));
    refreshedInput.value = 'later';
    refreshedInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    await vi.waitFor(() =>
      expect(
        (container.querySelector('form button[type="submit"]') as HTMLButtonElement).disabled
      ).toBe(false)
    );
  });

  it('does not restore a room snapshot after an admin-cache privacy boundary', async () => {
    const pendingSave = deferred<{
      id: string;
      name: string;
      description: string;
      universal: boolean;
      archived: boolean;
    }>();
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    mocks.updateRoom.mockReturnValueOnce(pendingSave.promise);
    const view = renderSections('general');
    await settle();

    const input = view.container.querySelector('#room-settings-name') as HTMLInputElement;
    input.value = 'private-name';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (view.container.querySelector('form button[type="submit"]') as HTMLButtonElement).click();
    await vi.waitFor(() => expect(mocks.updateRoom).toHaveBeenCalledOnce());

    removeRegisteredAdminQueries('server-a');
    view.unmount();
    pendingSave.resolve({
      id: 'shared-room',
      name: 'private-name',
      description: '',
      universal: false,
      archived: false
    });
    await settle();

    const queryKey = adminQueryKeys.room('server-a', server.scope.connection, 'shared-room');
    expect(queryClient.getQueryData<AdminManagedRoom>(queryKey)?.name).not.toBe('private-name');
    expect(mocks.success).not.toHaveBeenCalled();
  });
  function sectionLinks(root: ParentNode) {
    return [...root.querySelectorAll('nav[aria-label="Room sections"] a')].map((link) => ({
      label: link.textContent?.trim(),
      current: link.getAttribute('aria-current')
    }));
  }

  it('offers every section to a room manager and marks the current one', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    const { container } = renderSections('permissions');
    await settle();

    expect(sectionLinks(container)).toEqual([
      { label: 'General', current: null },
      { label: 'Members', current: null },
      { label: 'Permissions', current: 'page' }
    ]);
    expect(container.querySelector('[data-testid="permission-matrix"]')).not.toBeNull();
  });

  it('sends a viewer who cannot change the settings to the members section', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general', { canManageRoom: false }));
    const { container } = renderSections('general');
    await settle();

    expect(container.querySelector('#room-settings-name')).toBeNull();
    expect(sectionLinks(container).map((link) => link.label)).toEqual(['Members', 'Permissions']);
    await vi.waitFor(() =>
      expect(mocks.goto).toHaveBeenCalledWith(
        '/chat/server-a.example.test/manage/rooms/shared-room/members',
        {
          replaceState: true
        }
      )
    );
  });
  it('submits Universal without resetting a dirty name draft', async () => {
    serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const nameInput = container.querySelector('#room-settings-name') as HTMLInputElement;
    nameInput.value = 'draft-name';
    nameInput.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (container.querySelector('#room-settings-universal') as HTMLInputElement).click();

    await settle();
    expect(mocks.updateRoom).not.toHaveBeenCalled();
    submitSection(container, '#room-settings-universal');

    await vi.waitFor(() =>
      expect(mocks.updateRoom).toHaveBeenCalledWith({ roomId: 'shared-room', universal: true })
    );
    await vi.waitFor(() => expect(mocks.success).toHaveBeenCalledWith('Room updated'));
    expect((container.querySelector('#room-settings-universal') as HTMLInputElement).checked).toBe(
      true
    );
    expect((container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'draft-name'
    );
  });

  it('retains the Universal draft after a failed save and allows retry', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    mocks.updateRoom.mockRejectedValueOnce(new Error('offline'));
    const { container } = renderSections('general');
    await settle();

    const checkbox = container.querySelector('#room-settings-universal') as HTMLInputElement;
    checkbox.click();
    submitSection(container, '#room-settings-universal');

    await vi.waitFor(() => expect(mocks.error).toHaveBeenCalled());
    await vi.waitFor(() => expect(checkbox.disabled).toBe(false));
    expect(checkbox.checked).toBe(true);
    expect(checkbox.disabled).toBe(false);
    serveRoom(managedRoom('general'));
    submitSection(container, '#room-settings-universal');
    await vi.waitFor(() => expect(mocks.updateRoom).toHaveBeenCalledTimes(2));
    await vi.waitFor(() =>
      expect(formAction(container, '#room-settings-universal').disabled).toBe(true)
    );
    expect(checkbox.checked).toBe(true);
  });

  it('does not let an older concurrent save restore another panel value', async () => {
    const room = serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();

    const releaseUniversal = room.hold();
    (container.querySelector('#room-settings-universal') as HTMLInputElement).click();
    submitSection(container, '#room-settings-universal');
    await vi.waitFor(() => expect(mocks.updateRoom).toHaveBeenCalledOnce());
    // The server applied Universal first, so its response still has Slow Mode off.
    const select = container.querySelector('#room-settings-slow-mode') as HTMLSelectElement;
    select.value = '10';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    submitSection(container, '#room-settings-slow-mode');
    await vi.waitFor(() => expect(mocks.updateRoom).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(mocks.success).toHaveBeenCalledOnce());
    // Keep the refetch from reconciling, so only the save responses write the cache.
    mocks.getRoom.mockImplementation(() => new Promise(() => {}));

    releaseUniversal();
    await vi.waitFor(() => expect(mocks.success).toHaveBeenCalledTimes(2));

    const cached = queryClient.getQueryData<AdminManagedRoom>(
      adminQueryKeys.room('server-a', server.scope.connection, 'shared-room')
    );
    expect(cached?.isUniversal).toBe(true);
    expect(cached?.slowModeSeconds).toBe(10);
  });

  it('discards only its section and clears navigation protection after all drafts are discarded', async () => {
    serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();
    const name = container.querySelector('#room-settings-name') as HTMLInputElement;
    const universal = container.querySelector('#room-settings-universal') as HTMLInputElement;
    const interval = container.querySelector('#room-settings-slow-mode') as HTMLSelectElement;
    const choices = [...container.querySelectorAll<HTMLButtonElement>('[role="radio"]')];
    name.value = 'draft-name';
    name.dispatchEvent(new Event('input', { bubbles: true }));
    universal.click();
    interval.value = '10';
    interval.dispatchEvent(new Event('change', { bubbles: true }));
    choices[0].click();
    await settle();
    expect(mocks.updateRoom).not.toHaveBeenCalled();
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const cancel = vi.fn();
    const guard = mocks.beforeNavigate.mock.calls.at(-1)![0];

    formAction(container, '#room-settings-name', 'Discard changes').click();
    await settle();
    expect(name.value).toBe('general');
    expect(universal.checked).toBe(true);
    expect(interval.value).toBe('10');
    expect(choices[0]).toHaveAttribute('aria-checked', 'true');
    guard({ type: 'link', cancel });
    expect(cancel).toHaveBeenCalledOnce();

    for (const control of [
      '#room-settings-universal',
      '#room-settings-slow-mode',
      '[role="radio"]'
    ]) {
      formAction(container, control, 'Discard changes').click();
    }
    await settle();
    expect(universal.checked).toBe(false);
    expect(interval.value).toBe('0');
    expect(choices[2]).toHaveAttribute('aria-checked', 'true');
    guard({ type: 'link', cancel });
    expect(confirm).not.toHaveBeenCalled();
    expect(mocks.updateRoom).not.toHaveBeenCalled();
    for (const button of container.querySelectorAll<HTMLButtonElement>(
      'form button[type="submit"]'
    )) {
      expect(button.disabled).toBe(true);
    }
  });

  it('keeps a pending section disabled and rejects duplicate submissions while other sections stay usable', async () => {
    const room = serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();
    const release = room.hold();
    const checkbox = container.querySelector('#room-settings-universal') as HTMLInputElement;
    checkbox.click();
    submitSection(container, '#room-settings-universal');
    await vi.waitFor(() => expect(mocks.updateRoom).toHaveBeenCalledOnce());
    expect(checkbox.disabled).toBe(true);
    expect(formAction(container, '#room-settings-universal', 'Discard changes').disabled).toBe(
      true
    );
    checkbox
      .closest('form')!
      .dispatchEvent(new SubmitEvent('submit', { bubbles: true, cancelable: true }));
    await settle();
    expect(mocks.updateRoom).toHaveBeenCalledOnce();
    expect((container.querySelector('#room-settings-name') as HTMLInputElement).disabled).toBe(
      false
    );
    release();
    await vi.waitFor(() => expect(checkbox.disabled).toBe(false));
    expect(formAction(container, '#room-settings-universal').disabled).toBe(true);
  });

  it('preserves edited settings across refreshes and discards to the latest server values', async () => {
    mocks.getRoom.mockResolvedValueOnce({ ...managedRoom('general'), slowModeSeconds: 7 });
    const { container } = renderSections('general');
    await settle();
    const universal = container.querySelector('#room-settings-universal') as HTMLInputElement;
    const interval = container.querySelector('#room-settings-slow-mode') as HTMLSelectElement;
    const choices = [...container.querySelectorAll<HTMLButtonElement>('[role="radio"]')];
    expect([...interval.options].some((option) => option.value === '7')).toBe(true);
    universal.click();
    interval.value = '10';
    interval.dispatchEvent(new Event('change', { bubbles: true }));
    choices[0].click();
    await settle();
    mocks.getRoom.mockResolvedValue({
      ...managedRoom('remote-name', { threadingMode: RoomThreadingMode.DISABLED }),
      slowModeSeconds: 13
    });
    dispatchProjection(roomSnapshot());
    await vi.waitFor(() => expect(mocks.getRoom).toHaveBeenCalledTimes(2));
    await settle();
    expect((container.querySelector('#room-settings-name') as HTMLInputElement).value).toBe(
      'remote-name'
    );
    expect(universal.checked).toBe(true);
    expect(interval.value).toBe('10');
    expect(choices[0]).toHaveAttribute('aria-checked', 'true');
    formAction(container, '#room-settings-slow-mode', 'Discard changes').click();
    formAction(container, '[role="radio"]', 'Discard changes').click();
    await settle();
    expect(interval.value).toBe('13');
    expect(choices[3]).toHaveAttribute('aria-checked', 'true');
    expect(mocks.updateRoom).not.toHaveBeenCalled();
  });

  it('keeps invalid drafts on modal cancellation and resumes only the confirmed destination', async () => {
    serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();
    const guard = mocks.beforeNavigate.mock.calls.at(-1)![0];
    const cancel = vi.fn();
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    guard({ type: 'link', cancel });
    expect(confirm).not.toHaveBeenCalled();
    const name = container.querySelector('#room-settings-name') as HTMLInputElement;
    name.value = ' ';
    name.dispatchEvent(new Event('input', { bubbles: true }));
    await settle();
    expect(formAction(container, '#room-settings-name').disabled).toBe(true);
    const destination = new URL(
      'https://server-a.example.test/chat/-/manage/rooms/shared-room/members?filter=active#members'
    );
    const navigation = { type: 'link', cancel, to: { url: destination }, willUnload: false };
    guard(navigation);
    await settle();
    const dialog = document.querySelector('dialog[open]')!;
    expect(dialog.textContent).toContain('You have unsaved changes.');
    const cancelButton = [...dialog.querySelectorAll('button')].find(
      (button) => button.textContent?.trim() === 'Cancel'
    )!;
    cancelButton.click();
    await settle();
    expect(name.value).toBe(' ');
    expect(mocks.goto).not.toHaveBeenCalled();

    guard(navigation);
    await settle();
    // A second attempt while the modal is open must not replace its destination.
    guard({ ...navigation, to: { url: new URL('https://server-a.example.test/other') } });
    const resumedCancel = vi.fn();
    mocks.goto.mockImplementationOnce(async () =>
      guard({ ...navigation, type: 'goto', cancel: resumedCancel })
    );
    (document.querySelector('dialog[open] button[type="submit"]') as HTMLButtonElement).click();
    await settle();
    expect(mocks.goto).toHaveBeenCalledWith(
      `${destination.pathname}${destination.search}${destination.hash}`
    );
    expect(resumedCancel).not.toHaveBeenCalled();
    expect(confirm).not.toHaveBeenCalled();
    expect(mocks.updateRoom).not.toHaveBeenCalled();

    // The one-time approval cannot bypass the guard on a later attempt.
    guard({ type: 'leave', cancel });
    expect(cancel).toHaveBeenCalledTimes(4);
    expect(confirm).not.toHaveBeenCalled();
  });

  it('resumes back navigation with its history delta instead of adding a history entry', async () => {
    serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();
    (container.querySelector('#room-settings-universal') as HTMLInputElement).click();
    await settle();
    const guard = mocks.beforeNavigate.mock.calls.at(-1)![0];
    const cancel = vi.fn();
    const navigation = {
      type: 'popstate',
      delta: -1,
      cancel,
      to: { url: new URL('https://server-a.example.test/chat/-/overview') },
      willUnload: false
    };
    const historyGo = vi.spyOn(window.history, 'go').mockImplementation(() => {});
    guard(navigation);
    await settle();
    (document.querySelector('dialog[open] button[type="submit"]') as HTMLButtonElement).click();
    await settle();
    expect(historyGo).toHaveBeenCalledWith(-1);
    expect(mocks.goto).not.toHaveBeenCalled();
    guard(navigation);
    expect(cancel).toHaveBeenCalledOnce();
    guard(navigation);
    expect(cancel).toHaveBeenCalledTimes(2);
  });

  it('uses the native unload warning without opening a modal for tab close or reload', async () => {
    serveRoom(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();
    (container.querySelector('#room-settings-universal') as HTMLInputElement).click();
    await settle();
    const cancel = vi.fn();
    mocks.beforeNavigate.mock.calls.at(-1)![0]({ type: 'leave', cancel });
    await settle();
    expect(cancel).toHaveBeenCalledOnce();
    expect(document.querySelector('dialog[open]')).toBeNull();
  });

  it('removes dirty drafts on permission loss without blocking the redirect', async () => {
    mocks.getRoom.mockResolvedValue(managedRoom('general'));
    const { container } = renderSections('general');
    await settle();
    (container.querySelector('#room-settings-universal') as HTMLInputElement).click();
    await settle();
    const guard = mocks.beforeNavigate.mock.calls.at(-1)![0];
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    const cancel = vi.fn();
    guard({
      type: 'link',
      cancel: vi.fn(),
      to: { url: new URL('https://server-a.example.test/chat/-/overview') }
    });
    await settle();
    expect(document.querySelector('dialog[open]')).not.toBeNull();
    mocks.getRoom.mockResolvedValue(managedRoom('general', { canManageRoom: false }));
    dispatchProjection(roomSnapshot());
    await vi.waitFor(() => expect(container.querySelector('#room-settings-universal')).toBeNull());
    guard({ type: 'goto', cancel });
    expect(confirm).not.toHaveBeenCalled();
    expect(cancel).not.toHaveBeenCalled();
    expect(mocks.goto).toHaveBeenCalled();
    expect(mocks.updateRoom).not.toHaveBeenCalled();
    expect(document.querySelector('dialog[open]')).toBeNull();
  });
});
