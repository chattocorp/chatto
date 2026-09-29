import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RealtimeProjectionUpdate } from '@chatto/client/realtime/eventBus';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { ListRoomGroupsResponse, RoomGroup } from '@chatto/api-types/api/v1/room_directory_pb';
import { RealtimeResourceUpdate } from '@chatto/client/api/realtimeResources';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import { queryClient } from '$lib/query/client';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';

const mocks = vi.hoisted(() => ({
  getRoomGroup: vi.fn(),
  updateRoomGroup: vi.fn(),
  refreshLayout: vi.fn(),
  projectionHandlers: [] as Array<(event: RealtimeProjectionUpdate) => void>,
  success: vi.fn(),
  error: vi.fn()
}));

// Page titles are tested separately from this page's partial route/server fixtures.
// The store mock also carries the frontend UI state of its server.
vi.mock(
  '$lib/state/server/serverUi',
  async () => (await import('$lib/test-utils/serverUiMock')).serverUiIsStore
);

vi.mock('$lib/render/pageTitle', () => ({ formatPageTitle: () => 'Chatto' }));

vi.mock('$app/state', () => ({
  page: {
    get params() {
      return { groupId: routeGroupId };
    }
  }
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

vi.mock('$lib/api/adminRoomLayout', () => ({
  createAdminRoomLayoutAPI: ({ serverId }: { serverId: string }) => ({
    getRoomGroup: (groupId: string, options?: { signal?: AbortSignal }) =>
      mocks.getRoomGroup(serverId, groupId, options),
    updateRoomGroup: (input: unknown) => mocks.updateRoomGroup(serverId, input)
  })
}));

vi.mock('$lib/components/rbac/PermissionMatrix.svelte', async () => ({
  default: (await import('../../rooms/[roomId]/RoomManagementPagePermissionMatrixMock.svelte'))
    .default
}));

vi.mock('$lib/ui/toast', () => ({
  toast: { success: mocks.success, error: mocks.error }
}));

import RoomGroupPage from './+page.svelte';

let routeGroupId = $state('group-a');

function managedGroup(name: string, groupId = routeGroupId) {
  return {
    group: {
      id: groupId,
      name,
      description: 'Description',
      canCreateRoom: true,
      rooms: [],
      items: []
    },
    canManageGroup: true,
    canManagePermissions: true
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

function dispatchGroups(groupIds: string[]): void {
  const event = new RealtimeProjectionUpdate({
    resource: new RealtimeResourceUpdate({
      resource: {
        case: 'roomGroups',
        value: new ListRoomGroupsResponse({
          groups: groupIds.map((id) => new RoomGroup({ id, name: id }))
        })
      }
    })
  });
  for (const handler of mocks.projectionHandlers) handler(event);
}

describe('room-group management query lifecycle', () => {
  beforeEach(async () => {
    queryClient.clear();
    vi.clearAllMocks();
    mocks.projectionHandlers = [];
    routeGroupId = 'group-a';
    createTestServerScope({
      serverId: 'server-a',
      store: { adminRoomLayout: { refresh: mocks.refreshLayout } }
    });
    mocks.getRoomGroup.mockResolvedValue(managedGroup('Lobby'));
    mocks.updateRoomGroup.mockResolvedValue(managedGroup('Projects').group);
    mocks.refreshLayout.mockResolvedValue(undefined);
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('passes cancellation through and revalidates a cached group snapshot on remount', async () => {
    const first = render(RoomGroupPage);
    await settle();

    expect(mocks.getRoomGroup).toHaveBeenCalledWith(
      'server-a',
      'group-a',
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    );
    expect(first.container.textContent).toContain('Lobby');
    first.unmount();

    const second = render(RoomGroupPage);
    await settle();
    expect(second.container.textContent).toContain('Lobby');
    expect(mocks.getRoomGroup).toHaveBeenCalledTimes(2);
  });

  it('switches query identity when the route group changes', async () => {
    mocks.getRoomGroup.mockImplementation((_serverId: string, groupId: string) =>
      Promise.resolve(managedGroup(groupId === 'group-a' ? 'Lobby' : 'Projects', groupId))
    );
    const { container } = render(RoomGroupPage);
    await settle();

    routeGroupId = 'group-b';
    flushSync();
    await settle();

    expect(container.textContent).toContain('Projects');
    expect((container.querySelector('#room-group-settings-name') as HTMLInputElement).value).toBe(
      'Projects'
    );
  });

  it('preserves a dirty draft while a group projection refreshes the snapshot', async () => {
    mocks.getRoomGroup
      .mockResolvedValueOnce(managedGroup('Lobby'))
      .mockResolvedValueOnce(managedGroup('Remote name'));
    const { container } = render(RoomGroupPage);
    await settle();

    const input = container.querySelector('#room-group-settings-name') as HTMLInputElement;
    input.value = 'Local draft';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    dispatchGroups(['group-a']);
    await vi.waitFor(() => expect(mocks.getRoomGroup).toHaveBeenCalledTimes(2));
    await settle();

    expect((container.querySelector('#room-group-settings-name') as HTMLInputElement).value).toBe(
      'Local draft'
    );
    expect(container.textContent).toContain('Remote name');
  });

  it('adopts refreshed group fields while the form is pristine', async () => {
    mocks.getRoomGroup
      .mockResolvedValueOnce(managedGroup('Lobby'))
      .mockResolvedValueOnce(managedGroup('Remote name'));
    const { container } = render(RoomGroupPage);
    await settle();

    dispatchGroups(['group-a']);
    await vi.waitFor(() => expect(mocks.getRoomGroup).toHaveBeenCalledTimes(2));
    await settle();

    expect((container.querySelector('#room-group-settings-name') as HTMLInputElement).value).toBe(
      'Remote name'
    );
  });

  it('updates the exact snapshot after a settings mutation', async () => {
    const { container } = render(RoomGroupPage);
    await settle();
    mocks.getRoomGroup.mockResolvedValue(managedGroup('Projects'));

    const input = container.querySelector('#room-group-settings-name') as HTMLInputElement;
    input.value = 'Projects';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (container.querySelector('form button[type="submit"]') as HTMLButtonElement).click();

    await vi.waitFor(() => expect(mocks.updateRoomGroup).toHaveBeenCalledOnce());
    await settle();
    expect(mocks.success).toHaveBeenCalledOnce();
    expect(mocks.refreshLayout).toHaveBeenCalledOnce();
    expect((container.querySelector('#room-group-settings-name') as HTMLInputElement).value).toBe(
      'Projects'
    );
  });

  it('acknowledges a save without overwriting a newer projected snapshot', async () => {
    const pendingSave = deferred<ReturnType<typeof managedGroup>['group']>();
    mocks.updateRoomGroup.mockReturnValueOnce(pendingSave.promise);
    const { container } = render(RoomGroupPage);
    await settle();

    const input = container.querySelector('#room-group-settings-name') as HTMLInputElement;
    input.value = 'Projects';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (container.querySelector('form button[type="submit"]') as HTMLButtonElement).click();
    await vi.waitFor(() => expect(mocks.updateRoomGroup).toHaveBeenCalledOnce());

    mocks.getRoomGroup.mockResolvedValue(managedGroup('Projected name'));
    dispatchGroups(['group-a']);
    await vi.waitFor(() => expect(container.textContent).toContain('Projected name'));

    pendingSave.resolve(managedGroup('Mutation response').group);
    await vi.waitFor(() => expect(mocks.success).toHaveBeenCalledOnce());
    await settle();

    expect(container.textContent).toContain('Projected name');
    expect(container.textContent).not.toContain('Mutation response');
  });

  it('fences a late save response after projected group removal', async () => {
    const pendingSave = deferred<ReturnType<typeof managedGroup>['group']>();
    mocks.updateRoomGroup.mockReturnValueOnce(pendingSave.promise);
    const { container } = render(RoomGroupPage);
    await settle();

    const input = container.querySelector('#room-group-settings-name') as HTMLInputElement;
    input.value = 'Private draft';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
    (container.querySelector('form button[type="submit"]') as HTMLButtonElement).click();
    await vi.waitFor(() => expect(mocks.updateRoomGroup).toHaveBeenCalledOnce());

    mocks.getRoomGroup.mockResolvedValue(null);
    dispatchGroups([]);
    pendingSave.resolve(managedGroup('Private draft').group);
    await settle();

    expect(container.textContent).not.toContain('Private draft');
    expect(mocks.success).not.toHaveBeenCalled();
  });

  it('purges the visible group immediately when projection access disappears', async () => {
    const { container } = render(RoomGroupPage);
    await settle();
    expect(container.textContent).toContain('Lobby');
    mocks.getRoomGroup.mockResolvedValueOnce(null);

    dispatchGroups([]);
    flushSync();

    expect(container.textContent).not.toContain('Lobby');
    expect(container.querySelector('#room-group-settings-name')).toBeNull();
  });
});
