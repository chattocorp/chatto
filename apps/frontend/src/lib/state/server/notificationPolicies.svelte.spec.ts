import { describe, expect, it, vi } from 'vitest';
import {
  NotificationDeliveryMode,
  type NotificationAPI,
  type NotificationPolicyScope,
  type ScopedNotificationPolicy
} from '$lib/api-client/notifications';
import { NotificationPolicyMatrixState } from './notificationPolicies.svelte';

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function policy(
  scope: NotificationPolicyScope,
  directMessages: NotificationDeliveryMode = NotificationDeliveryMode.PUSH_NOTIFICATION
): ScopedNotificationPolicy {
  const effective = {
    directMessages,
    roomMessages: NotificationDeliveryMode.UNREAD_BADGE,
    directMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
    replies: NotificationDeliveryMode.PUSH_NOTIFICATION,
    roleMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
    hereMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
    allMentions: NotificationDeliveryMode.PUSH_NOTIFICATION,
    followedThreads: NotificationDeliveryMode.IN_APP_NOTIFICATION,
    followedRooms: NotificationDeliveryMode.OFF,
    reactions: NotificationDeliveryMode.IN_APP_NOTIFICATION
  };
  return {
    scope,
    overrides: {
      directMessages: null,
      roomMessages: null,
      directMentions: null,
      replies: null,
      roleMentions: null,
      hereMentions: null,
      allMentions: null,
      followedThreads: null,
      followedRooms: null,
      reactions: null
    },
    effective
  };
}

function api(
  overrides: Partial<
    Pick<NotificationAPI, 'batchGetNotificationPolicies' | 'updateScopedNotificationPolicy'>
  >
): NotificationAPI {
  return overrides as NotificationAPI;
}

describe('NotificationPolicyMatrixState', () => {
  it('fences a stale load with its scope generation and signature', async () => {
    const first = deferred<ScopedNotificationPolicy[]>();
    const second = deferred<ScopedNotificationPolicy[]>();
    const batch = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const state = new NotificationPolicyMatrixState(api({ batchGetNotificationPolicies: batch }));

    const staleLoad = state.load([{ kind: 'server' }]);
    const currentLoad = state.load([{ kind: 'room', id: 'r1' }]);
    first.resolve([policy({ kind: 'server' })]);
    await staleLoad;
    expect(state.policy({ kind: 'server' })).toBeUndefined();
    expect(state.loading).toBe(true);

    second.resolve([policy({ kind: 'room', id: 'r1' })]);
    await currentLoad;
    expect(state.policy({ kind: 'room', id: 'r1' })).toBeDefined();
    expect(state.loading).toBe(false);
  });

  it('tracks independent pending cells and merges canonical responses', async () => {
    const first = deferred<ScopedNotificationPolicy>();
    const second = deferred<ScopedNotificationPolicy>();
    const update = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const state = new NotificationPolicyMatrixState(
      api({
        batchGetNotificationPolicies: vi.fn().mockResolvedValue([]),
        updateScopedNotificationPolicy: update
      })
    );
    const server = { kind: 'server' } as const;

    const one = state.update(server, 'directMessages', NotificationDeliveryMode.OFF);
    const two = state.update(server, 'reactions', NotificationDeliveryMode.PUSH_NOTIFICATION);
    expect(state.isPending(server, 'directMessages')).toBe(true);
    expect(state.isPending(server, 'reactions')).toBe(true);

    first.resolve(policy(server, NotificationDeliveryMode.OFF));
    await one;
    expect(state.isPending(server, 'directMessages')).toBe(false);
    expect(state.isPending(server, 'reactions')).toBe(true);
    expect(state.policy(server)?.effective.directMessages).toBe(NotificationDeliveryMode.OFF);

    second.resolve(policy(server));
    await two;
    expect(state.isPending(server, 'reactions')).toBe(false);
  });

  it('refreshes loaded descendants after an ancestor update', async () => {
    const server = { kind: 'server' } as const;
    const group = { kind: 'roomGroup', id: 'g1' } as const;
    const room = { kind: 'room', id: 'r1' } as const;
    const initial = [policy(server), policy(group), policy(room)];
    const refreshed = [
      policy(server, NotificationDeliveryMode.IN_APP_NOTIFICATION),
      policy(group, NotificationDeliveryMode.IN_APP_NOTIFICATION),
      policy(room, NotificationDeliveryMode.IN_APP_NOTIFICATION)
    ];
    const batch = vi.fn().mockResolvedValueOnce(initial).mockResolvedValueOnce(refreshed);
    const state = new NotificationPolicyMatrixState(
      api({
        batchGetNotificationPolicies: batch,
        updateScopedNotificationPolicy: vi
          .fn()
          .mockResolvedValue(policy(server, NotificationDeliveryMode.IN_APP_NOTIFICATION))
      })
    );

    await state.load([server, group, room]);
    await state.update(server, 'directMessages', NotificationDeliveryMode.IN_APP_NOTIFICATION);

    expect(batch).toHaveBeenCalledTimes(2);
    expect(state.policy(group)?.effective.directMessages).toBe(
      NotificationDeliveryMode.IN_APP_NOTIFICATION
    );
    expect(state.policy(room)?.effective.directMessages).toBe(
      NotificationDeliveryMode.IN_APP_NOTIFICATION
    );
  });

  it('retains previous policy data on a failed save and clears on reset', async () => {
    const server = { kind: 'server' } as const;
    const state = new NotificationPolicyMatrixState(
      api({
        batchGetNotificationPolicies: vi.fn().mockResolvedValue([policy(server)]),
        updateScopedNotificationPolicy: vi.fn().mockRejectedValue(new Error('rejected'))
      })
    );
    await state.load([server]);
    const previous = state.policy(server);

    await state.update(server, 'directMessages', NotificationDeliveryMode.OFF);
    expect(state.policy(server)).toBe(previous);
    expect(state.error).toBe('rejected');
    expect(state.errorKind).toBe('save');

    state.reset();
    expect(state.policy(server)).toBeUndefined();
    expect(state.pendingKeys.size).toBe(0);
    expect(state.error).toBeNull();
  });

  it('does not restore policy data when a pending save finishes after reset', async () => {
    const server = { kind: 'server' } as const;
    const save = deferred<ScopedNotificationPolicy>();
    const state = new NotificationPolicyMatrixState(
      api({
        batchGetNotificationPolicies: vi.fn().mockResolvedValue([policy(server)]),
        updateScopedNotificationPolicy: vi.fn().mockReturnValue(save.promise)
      })
    );
    await state.load([server]);

    const pending = state.update(server, 'directMessages', NotificationDeliveryMode.OFF);
    state.reset();
    save.resolve(policy(server, NotificationDeliveryMode.OFF));
    await pending;

    expect(state.policy(server)).toBeUndefined();
    expect(state.pendingKeys.size).toBe(0);
    expect(state.error).toBeNull();
  });

  it('does not clear a new pending save when a pre-reset save finishes', async () => {
    const server = { kind: 'server' } as const;
    const oldSave = deferred<ScopedNotificationPolicy>();
    const newSave = deferred<ScopedNotificationPolicy>();
    const update = vi
      .fn()
      .mockReturnValueOnce(oldSave.promise)
      .mockReturnValueOnce(newSave.promise);
    const state = new NotificationPolicyMatrixState(
      api({
        batchGetNotificationPolicies: vi.fn().mockResolvedValue([policy(server)]),
        updateScopedNotificationPolicy: update
      })
    );
    await state.load([server]);

    const beforeReset = state.update(server, 'directMessages', NotificationDeliveryMode.OFF);
    state.reset();
    const afterReset = state.update(
      server,
      'directMessages',
      NotificationDeliveryMode.IN_APP_NOTIFICATION
    );

    oldSave.resolve(policy(server, NotificationDeliveryMode.OFF));
    await beforeReset;
    expect(state.isPending(server, 'directMessages')).toBe(true);

    newSave.resolve(policy(server, NotificationDeliveryMode.IN_APP_NOTIFICATION));
    await afterReset;
    expect(state.isPending(server, 'directMessages')).toBe(false);
    expect(state.policy(server)?.effective.directMessages).toBe(
      NotificationDeliveryMode.IN_APP_NOTIFICATION
    );
  });
});
