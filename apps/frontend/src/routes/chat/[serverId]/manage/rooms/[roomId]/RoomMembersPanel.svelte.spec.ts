import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { RoomViewerState, RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { Room } from '@chatto/api-types/api/v1/rooms_pb';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import {
  RealtimeProjectionEvent,
  RealtimeProjectionOperation,
  RealtimeProjectionRoom,
  RealtimeProjectionRoomRemove,
  RealtimeProjectionUserRemove
} from '@chatto/api-types/realtime/v1/realtime_pb';
import type {
  DirectoryMember,
  MemberDirectoryAPI,
  MemberDirectoryPage
} from '$lib/api-client/memberDirectory';

import type { RoomCommandAPI } from '$lib/api-client/rooms';
import { queryClient } from '$lib/query/client';
import RoomMembersPanel from './RoomMembersPanel.svelte';

const mocks = vi.hoisted(() => ({
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  projectionHandler: null as ((event: RealtimeProjectionEvent) => void) | null,
  directoryAPI: null as MemberDirectoryAPI | null,
  commandAPI: null as RoomCommandAPI | null,
  queryScope: 'session-1',
  scopeCurrent: true
}));

vi.mock('$lib/state/server/scope.svelte', () => ({
  useServerScope: () => ({
    get connection() {
      return {
        queryScope: mocks.queryScope,
        getAPI: (factory: (config: never) => unknown) => factory({} as never)
      };
    },
    isCurrent: () => mocks.scopeCurrent
  })
}));

vi.mock('$lib/api-client/memberDirectory', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api-client/memberDirectory')>();
  return { ...actual, createMemberDirectoryAPI: () => mocks.directoryAPI };
});

vi.mock('$lib/api-client/rooms', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/api-client/rooms')>();
  return { ...actual, createRoomCommandAPI: () => mocks.commandAPI };
});

vi.mock('$lib/ui/toast', () => ({
  toast: {
    success: mocks.toastSuccess,
    error: mocks.toastError
  }
}));

vi.mock('$lib/ui/ConfirmDialog.svelte', async () => ({
  default: (await import('./RoomMembersConfirmDialogMock.svelte')).default
}));

vi.mock('$lib/hooks', () => ({
  useProjectionEvent: (handler: (event: RealtimeProjectionEvent) => void) => {
    mocks.projectionHandler = handler;
  }
}));

function member(id: string, displayName = id.toUpperCase()): DirectoryMember {
  return {
    id,
    login: id,
    displayName,
    deleted: false,
    avatarUrl: null,
    presenceStatus: PresenceStatus.OFFLINE,
    customStatus: null,
    roles: ['everyone'],
    createdAt: null
  };
}

function page(members: DirectoryMember[]): MemberDirectoryPage {
  return { members, totalCount: members.length, hasMore: false };
}

function setup(
  overrides: {
    members?: DirectoryMember[];
    directoryUsers?: DirectoryMember[];
    existingSearchMembers?: DirectoryMember[];
    addError?: Error;
    removeError?: Error;
  } = {}
) {
  let current = overrides.members ?? [member('alice', 'Alice')];
  const listRoomMembers = vi.fn().mockImplementation(() => Promise.resolve(page(current)));
  const listUsers = vi.fn().mockResolvedValue(page(overrides.directoryUsers ?? []));
  const batchGetRoomMembers = vi.fn().mockResolvedValue(overrides.existingSearchMembers ?? []);
  const addMember = vi.fn().mockImplementation(async ({ userId }: { userId: string }) => {
    if (overrides.addError) throw overrides.addError;
    const added = (overrides.directoryUsers ?? []).find((user) => user.id === userId) ?? null;
    if (added) current = [...current, added];
    return added;
  });
  const removeMember = vi.fn().mockImplementation(async ({ userId }: { userId: string }) => {
    if (overrides.removeError) throw overrides.removeError;
    current = current.filter((candidate) => candidate.id !== userId);
    return true;
  });
  const directory = {
    listRoomMembers,
    listUsers,
    batchGetRoomMembers,
    getUser: vi.fn(),
    getUserByLogin: vi.fn(),
    batchGetUsers: vi.fn(),
    getRoomMember: vi.fn()
  } as unknown as MemberDirectoryAPI;
  const commands = { addMember, removeMember } as unknown as RoomCommandAPI;
  mocks.directoryAPI = directory;
  mocks.commandAPI = commands;
  return { listRoomMembers, listUsers, batchGetRoomMembers, addMember, removeMember };
}

function renderPanel(
  overrides: Partial<{
    serverId: string;
    roomId: string;
    isUniversal: boolean;
    archived: boolean;
    canManageMembers: boolean;
  }> = {}
) {
  return render(RoomMembersPanel, {
    props: {
      serverId: overrides.serverId ?? 'server-1',
      roomId: overrides.roomId ?? 'room-1',
      roomName: 'general',
      isUniversal: overrides.isUniversal ?? false,
      archived: overrides.archived ?? false,
      canManageMembers: overrides.canManageMembers ?? true
    }
  });
}

async function settle(): Promise<void> {
  await vi.waitFor(() => {
    expect(queryClient.isFetching()).toBe(0);
    expect(queryClient.isMutating()).toBe(0);
  });
  flushSync();
}

async function settleDirectorySearch(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 220));
  await settle();
}

function buttonByText(root: ParentNode, text: string): HTMLButtonElement {
  const button = [...root.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === text
  );
  if (!(button instanceof HTMLButtonElement)) throw new Error(`Button not found: ${text}`);
  return button;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe('RoomMembersPanel', () => {
  beforeEach(() => {
    queryClient.clear();
    mocks.toastSuccess.mockReset();
    mocks.toastError.mockReset();
    mocks.projectionHandler = null;
    mocks.directoryAPI = null;
    mocks.commandAPI = null;
    mocks.queryScope = 'session-1';
    mocks.scopeCurrent = true;
  });

  it('searches the directory, excludes existing members, and successfully adds a user', async () => {
    const alice = member('alice', 'Alice');
    const bob = member('bob', 'Bob');
    const { addMember } = setup({
      members: [alice],
      directoryUsers: [alice, bob],
      existingSearchMembers: [alice]
    });
    const { container } = renderPanel();
    await settle();

    const input = container.querySelector('#room-member-picker') as HTMLInputElement;
    input.value = 'bo';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settleDirectorySearch();

    const options = [...document.querySelectorAll('[role="option"]')];
    expect(options).toHaveLength(1);
    expect(options[0].textContent).toContain('Bob');
    expect(options[0].textContent).not.toContain('Alice');
    (options[0] as HTMLButtonElement).click();
    flushSync();
    buttonByText(container, 'Add member').click();
    await settle();

    expect(addMember).toHaveBeenCalledWith({ roomId: 'room-1', userId: 'bob' });
    expect(container.textContent).toContain('Bob');
    await vi.waitFor(() =>
      expect(mocks.toastSuccess).toHaveBeenCalledWith('Added Bob to the room')
    );
  });

  it('requires confirmation before removing a member', async () => {
    const { removeMember } = setup();
    const { container } = renderPanel();
    await settle();

    buttonByText(container, 'Remove member').click();
    flushSync();
    expect(removeMember).not.toHaveBeenCalled();
    expect(document.querySelector('dialog')?.textContent).toContain('Remove Alice from #general?');

    buttonByText(document.querySelector('dialog')!, 'Remove member').click();
    await settle();

    expect(removeMember).toHaveBeenCalledWith({ roomId: 'room-1', userId: 'alice' });
    await vi.waitFor(() =>
      expect(mocks.toastSuccess).toHaveBeenCalledWith('Removed Alice from the room')
    );
  });

  it('hides editing controls without room.manage permission', async () => {
    setup();
    const { container } = renderPanel({ canManageMembers: false });
    await settle();

    expect(container.textContent).toContain('Alice');
    expect(container.querySelector('#room-member-picker')).toBeNull();
    expect([...container.querySelectorAll('button')]).toHaveLength(0);
  });

  it('explains automatic Universal membership without rendering editing controls', async () => {
    setup();
    const { container } = renderPanel({ isUniversal: true });
    await settle();

    expect(container.textContent).toContain('Membership is automatic in Universal rooms.');
    expect(container.querySelector('#room-member-picker')).toBeNull();
    expect(container.textContent).not.toContain('Remove member');
  });

  it('keeps archived room membership read-only', async () => {
    setup();
    const { container } = renderPanel({ archived: true });
    await settle();

    expect(container.textContent).toContain(
      'Membership cannot be changed while this room is archived.'
    );
    expect(container.querySelector('#room-member-picker')).toBeNull();
    expect(container.textContent).not.toContain('Remove member');
  });

  it('keeps identities dormant after room removal until an authorized remount', async () => {
    const { listRoomMembers } = setup();
    const rendered = renderPanel();
    const { container } = rendered;
    await settle();
    expect(container.textContent).toContain('Alice');

    mocks.projectionHandler?.(
      new RealtimeProjectionEvent({
        operations: [
          new RealtimeProjectionOperation({
            operation: {
              case: 'roomRemove',
              value: new RealtimeProjectionRoomRemove({ roomId: 'room-1' })
            }
          })
        ]
      })
    );
    flushSync();

    expect(container.textContent).not.toContain('Alice');
    await settle();
    expect(listRoomMembers).toHaveBeenCalledTimes(1);
    expect(container.textContent).not.toContain('Alice');

    await rendered.unmount();
    const remounted = renderPanel();
    await settle();
    expect(listRoomMembers).toHaveBeenCalledTimes(2);
    expect(remounted.container.textContent).toContain('Alice');
  });

  it('keeps nonmember managers current after room membership updates', async () => {
    const { listRoomMembers } = setup();
    renderPanel();
    await settle();

    mocks.projectionHandler?.(
      new RealtimeProjectionEvent({
        operations: [
          new RealtimeProjectionOperation({
            operation: {
              case: 'roomUpsert',
              value: new RealtimeProjectionRoom({
                room: new RoomWithViewerState({
                  room: new Room({ id: 'room-1' }),
                  viewerState: new RoomViewerState({ isMember: false })
                })
              })
            }
          })
        ]
      })
    );
    await settle();

    expect(listRoomMembers).toHaveBeenCalledTimes(2);
  });

  it('clears a selected add candidate when the server identity changes', async () => {
    const bob = member('bob', 'Bob');
    const { addMember } = setup({ directoryUsers: [bob] });
    const rendered = renderPanel();
    await settle();

    const input = rendered.container.querySelector('#room-member-picker') as HTMLInputElement;
    input.value = 'bob';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settleDirectorySearch();
    (document.querySelector('[role="option"]') as HTMLButtonElement).click();
    flushSync();
    expect(buttonByText(rendered.container, 'Add member').disabled).toBe(false);

    await rendered.unmount();
    renderPanel({
      serverId: 'server-2',
      roomId: 'room-1',
      isUniversal: false,
      archived: false,
      canManageMembers: true
    });
    await settle();

    expect(document.querySelector('#room-member-picker')).not.toBeNull();
    expect(addMember).not.toHaveBeenCalled();
  });

  it('closes a removal confirmation when the server identity changes', async () => {
    const { removeMember } = setup();
    const rendered = renderPanel();
    await settle();

    buttonByText(rendered.container, 'Remove member').click();
    flushSync();
    expect(document.querySelector('dialog')).not.toBeNull();

    await rendered.unmount();
    renderPanel({
      serverId: 'server-2',
      roomId: 'room-1',
      isUniversal: false,
      archived: false,
      canManageMembers: true
    });
    await settle();

    expect(document.querySelector('dialog')).toBeNull();
    expect(removeMember).not.toHaveBeenCalled();
  });

  it('clears a selected add candidate when realtime removes the user', async () => {
    const bob = member('bob', 'Bob');
    const { addMember } = setup({ directoryUsers: [bob] });
    const rendered = renderPanel();
    await settle();

    const input = rendered.container.querySelector('#room-member-picker') as HTMLInputElement;
    input.value = 'bob';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settleDirectorySearch();
    (document.querySelector('[role="option"]') as HTMLButtonElement).click();
    flushSync();
    expect(buttonByText(rendered.container, 'Add member').disabled).toBe(false);

    mocks.projectionHandler?.(
      new RealtimeProjectionEvent({
        operations: [
          new RealtimeProjectionOperation({
            operation: {
              case: 'userRemove',
              value: new RealtimeProjectionUserRemove({ userId: 'bob' })
            }
          })
        ]
      })
    );
    flushSync();

    expect(buttonByText(rendered.container, 'Add member').disabled).toBe(true);
    expect(input.value).toBe('');
    expect(addMember).not.toHaveBeenCalled();
  });

  it('closes a removal confirmation when realtime removes the user', async () => {
    const { removeMember } = setup();
    const rendered = renderPanel();
    await settle();
    buttonByText(rendered.container, 'Remove member').click();
    flushSync();
    expect(document.querySelector('dialog')).not.toBeNull();

    mocks.projectionHandler?.(
      new RealtimeProjectionEvent({
        operations: [
          new RealtimeProjectionOperation({
            operation: {
              case: 'userRemove',
              value: new RealtimeProjectionUserRemove({ userId: 'alice' })
            }
          })
        ]
      })
    );
    flushSync();

    expect(document.querySelector('dialog')).toBeNull();
    expect(removeMember).not.toHaveBeenCalled();
  });

  it('reports add and remove API errors without claiming success', async () => {
    const bob = member('bob', 'Bob');
    setup({
      directoryUsers: [bob],
      addError: new Error('user is banned')
    });
    const addRender = renderPanel();
    await settle();
    const input = addRender.container.querySelector('#room-member-picker') as HTMLInputElement;
    input.value = 'bob';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settleDirectorySearch();
    (document.querySelector('[role="option"]') as HTMLButtonElement).click();
    flushSync();
    buttonByText(addRender.container, 'Add member').click();
    await settle();

    await vi.waitFor(() =>
      expect(mocks.toastError).toHaveBeenCalledWith('Failed to add member: user is banned')
    );
    await addRender.unmount();
    queryClient.clear();
    setup({ removeError: new Error('room is archived') });
    const removeRender = renderPanel();
    await settle();
    buttonByText(removeRender.container, 'Remove member').click();
    flushSync();
    const dialogs = document.querySelectorAll('dialog');
    buttonByText(dialogs[dialogs.length - 1], 'Remove member').click();
    await settle();

    await vi.waitFor(() =>
      expect(mocks.toastError).toHaveBeenCalledWith('Failed to remove member: room is archived')
    );
  });

  it('keeps command success when the canonical membership reread fails', async () => {
    const bob = member('bob', 'Bob');
    const { listRoomMembers } = setup({ directoryUsers: [bob] });
    const rendered = renderPanel();
    await settle();
    listRoomMembers.mockRejectedValueOnce(
      new ConnectError('projection temporarily unavailable', Code.PermissionDenied)
    );

    const input = rendered.container.querySelector('#room-member-picker') as HTMLInputElement;
    input.value = 'bob';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settleDirectorySearch();
    (document.querySelector('[role="option"]') as HTMLButtonElement).click();
    flushSync();
    buttonByText(rendered.container, 'Add member').click();
    await settle();

    expect(mocks.toastSuccess).toHaveBeenCalledWith('Added Bob to the room');
    expect(rendered.container.textContent).toContain('projection temporarily unavailable');
    expect(rendered.container.textContent).not.toContain('Bob');
  });

  it('suppresses a mutation error that settles after the server scope is destroyed', async () => {
    const bob = member('bob', 'Bob');
    const pending = deferred<null>();
    const { addMember } = setup({ directoryUsers: [bob] });
    addMember.mockReturnValueOnce(pending.promise);
    const rendered = renderPanel();
    await settle();

    const input = rendered.container.querySelector('#room-member-picker') as HTMLInputElement;
    input.value = 'bob';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settleDirectorySearch();
    (document.querySelector('[role="option"]') as HTMLButtonElement).click();
    flushSync();
    buttonByText(rendered.container, 'Add member').click();
    await vi.waitFor(() => expect(addMember).toHaveBeenCalled());

    mocks.scopeCurrent = false;
    await rendered.unmount();
    pending.reject(new Error('old server unavailable'));
    await vi.waitFor(() => expect(queryClient.isMutating()).toBe(0));

    expect(mocks.toastError).not.toHaveBeenCalled();
    expect(mocks.toastSuccess).not.toHaveBeenCalled();
  });
});
