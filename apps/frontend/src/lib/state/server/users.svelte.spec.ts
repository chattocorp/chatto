import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DirectoryMember } from '@chatto/api-types/api/v1/member_directory_pb';
import { UserStore, getUserStore, disposeUserStore, resetUserStoresForTests } from './users.svelte';
import { ServerProjectionStore } from './projection.svelte';
import { mapDirectoryMember } from '$lib/api-client/directoryMemberView';
import { Timestamp } from '@bufbuild/protobuf';
import { RoomMembersStore } from '../room/members.svelte';
import type { ServerConnection } from './serverConnection.svelte';

const member = (id: string, name = id) =>
  new DirectoryMember({
    user: { id, login: id, displayName: name, avatarUrl: '/avatar', bot: { ownerUserId: 'owner' } },
    roles: ['everyone']
  });

beforeEach(resetUserStoresForTests);

describe('connection user store', () => {
  it('retains room membership while a profile is invalidated and another user changes', () => {
    const profiles = getUserStore('server', 'session');
    profiles.set('first', member('first'));
    profiles.set('second', member('second'));
    const room = new RoomMembersStore({
      serverId: 'server',
      queryScope: 'session',
      getAPI: () => ({})
    } as unknown as ServerConnection);
    room.members = [...profiles.values()].map(mapDirectoryMember);
    profiles.invalidate('first');
    room.updateUsers([mapDirectoryMember(member('second', 'Updated'))]);
    profiles.set('first', member('first', 'Restored'));
    expect(room.members.map((user) => user.id)).toEqual(['first', 'second']);
    expect(room.members[0].displayName).toBe('Restored');
  });

  it('shares one profile and one pending read across room, timeline, and projection consumers', async () => {
    const store = getUserStore('server', 'session');
    const projection = new ServerProjectionStore(store);
    let finish!: (users: DirectoryMember[]) => void;
    const read = vi.fn(
      () =>
        new Promise<DirectoryMember[]>((resolve) => {
          finish = resolve;
        })
    );
    const roomRead = store.resolve(['bot'], read);
    const timelineRead = vi.fn();
    const authorRead = store.resolve(['bot'], timelineRead);
    await vi.waitFor(() => expect(read).toHaveBeenCalledOnce());
    finish([member('bot')]);
    expect((await roomRead)[0].user?.bot).toBeDefined();
    expect((await authorRead)[0].user?.bot?.ownerUserId).toBe('owner');
    expect(timelineRead).not.toHaveBeenCalled();
    expect(projection.users).toBe(store);
    projection.users.set('bot', member('bot', 'Renamed'));
    expect((await store.resolve(['bot'], read))[0].user?.displayName).toBe('Renamed');
    expect(read).toHaveBeenCalledOnce();
  });

  it('keeps identical user IDs separate across servers and connection sessions', () => {
    getUserStore('a', 'old').set('id', member('id'));
    expect(getUserStore('a', 'new').has('id')).toBe(false);
    expect(getUserStore('b', 'old').has('id')).toBe(false);
  });

  it('fences detail reads after newer profiles, deletion, and reset', async () => {
    const store = new UserStore();
    let finish!: (members: DirectoryMember[]) => void;
    const pending = store.readSnapshot(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    store.set('updated', member('updated', 'New'));
    store.delete('deleted');
    finish([member('updated', 'Old'), member('deleted')]);
    expect((await pending).map((entry) => entry.user?.displayName)).toEqual(['New']);
    const old = store.readSnapshot(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    store.clear();
    finish([member('late')]);
    await expect(old).rejects.toThrow('Response discarded');
    expect(store.size).toBe(0);
  });

  it('prevents incidental timeline data from reviving removed users or replacing newer data', () => {
    const store = new UserStore();
    store.set('updated', member('updated', 'New'));
    store.delete('deleted');
    store.seed(member('updated', 'Old'));
    store.seed(member('deleted'));
    expect(store.get('updated')?.user?.displayName).toBe('New');
    expect(store.has('deleted')).toBe(false);
  });

  it('permanently fences readers retained after connection disposal', async () => {
    const store = getUserStore('server', 'old');
    disposeUserStore('server', 'old');
    const read = vi.fn();
    await expect(store.resolve(['id'], read)).rejects.toThrow('Response discarded');
    store.set('id', member('id'));
    expect(store.get('id')).toBeUndefined();
    expect(getUserStore('server', 'new').size).toBe(0);
    expect(read).not.toHaveBeenCalled();
  });

  it('expires shared custom status and cancels its timer on reset', async () => {
    vi.useFakeTimers();
    try {
      const store = new UserStore();
      const profile = member('id');
      profile.user!.customStatus = new DirectoryMember({
        user: {
          customStatus: {
            emoji: '☕',
            text: 'Away',
            expiresAt: Timestamp.fromDate(new Date(Date.now() + 1000))
          }
        }
      }).user!.customStatus;
      store.set('id', profile);
      await vi.advanceTimersByTimeAsync(1000);
      expect(store.get('id')?.user?.customStatus).toBeUndefined();
      store.set('id', profile);
      store.clear();
      await vi.runAllTimersAsync();
      expect(store.size).toBe(0);
    } finally {
      vi.useRealTimers();
    }
  });
});
