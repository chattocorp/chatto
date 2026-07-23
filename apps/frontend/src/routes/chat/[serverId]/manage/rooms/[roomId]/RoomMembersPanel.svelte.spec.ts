import { beforeEach, describe, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import {
  RealtimeProjectionEvent,
  RealtimeProjectionOperation,
  RealtimeProjectionRoomRemove
} from '@chatto/api-types/realtime/v1/realtime_pb';
import type {
  DirectoryMember,
  MemberDirectoryAPI,
  MemberDirectoryPage
} from '$lib/api-client/memberDirectory';
import { PresenceStatus } from '$lib/api-client/renderTypes';
import type { RoomCommandAPI } from '$lib/api-client/rooms';
import RoomMembersPanel from './RoomMembersPanel.svelte';
import {
  RoomMemberManagementStore,
  type RoomMemberManagementAPIs
} from './RoomMemberManagementStore.svelte';

const mocks = vi.hoisted(() => ({
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  projectionHandler: null as ((event: RealtimeProjectionEvent) => void) | null
}));

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
    presenceStatus: PresenceStatus.Offline,
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
  const api: RoomMemberManagementAPIs = {
    directory: {
      listRoomMembers,
      listUsers,
      batchGetRoomMembers,
      getUser: vi.fn(),
      getUserByLogin: vi.fn(),
      batchGetUsers: vi.fn(),
      getRoomMember: vi.fn()
    } as unknown as MemberDirectoryAPI,
    commands: {
      addMember,
      removeMember
    } as unknown as RoomCommandAPI
  };
  const store = new RoomMemberManagementStore(() => api);
  return { store, listRoomMembers, listUsers, batchGetRoomMembers, addMember, removeMember };
}

function renderPanel(
  store: RoomMemberManagementStore,
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
      canManageMembers: overrides.canManageMembers ?? true,
      store
    }
  });
}

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
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

describe('RoomMembersPanel', () => {
  beforeEach(() => {
    mocks.toastSuccess.mockReset();
    mocks.toastError.mockReset();
    mocks.projectionHandler = null;
  });

  it('searches the directory, excludes existing members, and successfully adds a user', async () => {
    const alice = member('alice', 'Alice');
    const bob = member('bob', 'Bob');
    const { store, addMember } = setup({
      members: [alice],
      directoryUsers: [alice, bob],
      existingSearchMembers: [alice]
    });
    const { container } = renderPanel(store);
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
    const { store, removeMember } = setup();
    const { container } = renderPanel(store);
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
    const { store } = setup();
    const { container } = renderPanel(store, { canManageMembers: false });
    await settle();

    expect(container.textContent).toContain('Alice');
    expect(container.querySelector('#room-member-picker')).toBeNull();
    expect([...container.querySelectorAll('button')]).toHaveLength(0);
  });

  it('explains automatic Universal membership without rendering editing controls', async () => {
    const { store } = setup();
    const { container } = renderPanel(store, { isUniversal: true });
    await settle();

    expect(container.textContent).toContain('Membership is automatic in Universal rooms.');
    expect(container.querySelector('#room-member-picker')).toBeNull();
    expect(container.textContent).not.toContain('Remove member');
  });

  it('keeps archived room membership read-only', async () => {
    const { store } = setup();
    const { container } = renderPanel(store, { archived: true });
    await settle();

    expect(container.textContent).toContain(
      'Membership cannot be changed while this room is archived.'
    );
    expect(container.querySelector('#room-member-picker')).toBeNull();
    expect(container.textContent).not.toContain('Remove member');
  });

  it('immediately clears member identities when realtime removes the room', async () => {
    const { store, listRoomMembers } = setup();
    const { container } = renderPanel(store);
    await settle();
    expect(container.textContent).toContain('Alice');

    listRoomMembers.mockRejectedValueOnce(new Error('permission denied'));
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
    expect(container.textContent).toContain('permission denied');
    expect(container.textContent).not.toContain('Alice');
  });

  it('clears a selected add candidate when the server identity changes', async () => {
    const bob = member('bob', 'Bob');
    const { store, addMember } = setup({ directoryUsers: [bob] });
    const rendered = renderPanel(store);
    await settle();

    const input = rendered.container.querySelector('#room-member-picker') as HTMLInputElement;
    input.value = 'bob';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await settleDirectorySearch();
    (document.querySelector('[role="option"]') as HTMLButtonElement).click();
    flushSync();
    expect(buttonByText(rendered.container, 'Add member').disabled).toBe(false);

    await rendered.rerender({
      serverId: 'server-2',
      roomId: 'room-1',
      roomName: 'general',
      isUniversal: false,
      archived: false,
      canManageMembers: true,
      store
    });
    await settle();

    expect(buttonByText(rendered.container, 'Add member').disabled).toBe(true);
    buttonByText(rendered.container, 'Add member').click();
    await settle();
    expect(addMember).not.toHaveBeenCalled();
  });

  it('closes a removal confirmation when the server identity changes', async () => {
    const { store, removeMember } = setup();
    const rendered = renderPanel(store);
    await settle();

    buttonByText(rendered.container, 'Remove member').click();
    flushSync();
    expect(document.querySelector('dialog')).not.toBeNull();

    await rendered.rerender({
      serverId: 'server-2',
      roomId: 'room-1',
      roomName: 'general',
      isUniversal: false,
      archived: false,
      canManageMembers: true,
      store
    });
    await settle();

    expect(document.querySelector('dialog')).toBeNull();
    expect(removeMember).not.toHaveBeenCalled();
  });

  it('reports add and remove API errors without claiming success', async () => {
    const bob = member('bob', 'Bob');
    const addSetup = setup({
      directoryUsers: [bob],
      addError: new Error('user is banned')
    });
    const addRender = renderPanel(addSetup.store);
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
    const removeSetup = setup({ removeError: new Error('room is archived') });
    const removeRender = renderPanel(removeSetup.store);
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
});
