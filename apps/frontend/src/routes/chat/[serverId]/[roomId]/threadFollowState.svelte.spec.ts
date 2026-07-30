import { flushSync } from 'svelte';
import { describe, expect, it, onTestFinished, vi } from 'vitest';
import type { ServerConnection } from '$lib/state/server/serverConnection.svelte';
import { ThreadFollowState, type ThreadFollowSnapshot } from './threadFollowState.svelte';

type Deferred<T> = {
  promise: Promise<T>;
  resolve(value: T): void;
  reject(error: Error): void;
};

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });
  return { promise, resolve, reject };
}

function setup(
  initial: ThreadFollowSnapshot = {
    roomId: 'room-1',
    threadRootEventId: 'thread-1',
    following: false
  }
) {
  const followRequest = deferred<{ following: boolean }>();
  const unfollowRequest = deferred<{ following: boolean }>();
  const api = {
    followThread: vi.fn(() => followRequest.promise),
    unfollowThread: vi.fn(() => unfollowRequest.promise)
  };
  const rollback = vi.fn();
  const beginOptimistic = vi.fn(() => ({ rollback }));
  const commit = vi.fn();
  const connection = {
    getAPI: vi.fn(() => api)
  } as unknown as ServerConnection;
  let snapshot = $state(initial);
  let state!: ThreadFollowState;
  const dispose = $effect.root(() => {
    state = new ThreadFollowState({
      getConnection: () => connection,
      getSnapshot: () => snapshot,
      beginOptimistic,
      commit
    });
  });
  onTestFinished(dispose);
  flushSync();

  return {
    api,
    beginOptimistic,
    commit,
    followRequest,
    rollback,
    setSnapshot(next: ThreadFollowSnapshot) {
      snapshot = next;
      flushSync();
    },
    state,
    unfollowRequest
  };
}

describe('ThreadFollowState', () => {
  it('optimistically follows and commits the server result', async () => {
    const { api, beginOptimistic, commit, followRequest, state } = setup();

    const request = state.toggle();

    expect(state.following).toBe(true);
    expect(state.pending).toBe(true);
    expect(beginOptimistic).toHaveBeenCalledWith(
      { roomId: 'room-1', threadRootEventId: 'thread-1' },
      true
    );
    expect(api.followThread).toHaveBeenCalledWith({
      roomId: 'room-1',
      threadRootEventId: 'thread-1'
    });

    followRequest.resolve({ following: true });
    await request;

    expect(state.following).toBe(true);
    expect(state.pending).toBe(false);
    expect(commit).toHaveBeenCalledWith({ roomId: 'room-1', threadRootEventId: 'thread-1' }, true);
  });

  it('rolls an optimistic update back when the request fails', async () => {
    const { followRequest, rollback, state } = setup();

    const request = state.toggle();
    followRequest.reject(new Error('request failed'));
    await request;

    expect(state.following).toBe(false);
    expect(state.pending).toBe(false);
    expect(rollback).toHaveBeenCalledOnce();
  });

  it('adopts an authoritative update after a concurrent request fails', async () => {
    const { followRequest, setSnapshot, state } = setup();

    const request = state.toggle();
    setSnapshot({
      roomId: 'room-1',
      threadRootEventId: 'thread-1',
      following: true
    });
    expect(state.pending).toBe(true);

    followRequest.reject(new Error('response lost'));
    await request;
    flushSync();

    expect(state.following).toBe(true);
    expect(state.pending).toBe(false);
  });

  it('unfollows from an authoritative followed state', async () => {
    const { api, state, unfollowRequest } = setup({
      roomId: 'room-1',
      threadRootEventId: 'thread-1',
      following: true
    });

    const request = state.toggle();
    expect(state.following).toBe(false);
    expect(api.unfollowThread).toHaveBeenCalledOnce();

    unfollowRequest.resolve({ following: false });
    await request;
    expect(state.following).toBe(false);
    expect(state.pending).toBe(false);
  });

  it('reconciles distinct authoritative updates without replaying stale input', async () => {
    const { followRequest, setSnapshot, state } = setup();

    const request = state.toggle();
    followRequest.resolve({ following: true });
    await request;

    setSnapshot({
      roomId: 'room-1',
      threadRootEventId: 'thread-1',
      following: false
    });
    expect(state.following).toBe(true);

    setSnapshot({
      roomId: 'room-1',
      threadRootEventId: 'thread-1',
      following: true
    });
    expect(state.following).toBe(true);

    setSnapshot({
      roomId: 'room-1',
      threadRootEventId: 'thread-1',
      following: false
    });
    expect(state.following).toBe(false);
  });

  it('fences an in-flight request when the component switches threads', async () => {
    const { commit, followRequest, rollback, setSnapshot, state } = setup();

    const request = state.toggle();
    setSnapshot({
      roomId: 'room-1',
      threadRootEventId: 'thread-2',
      following: true
    });

    expect(state.following).toBe(true);
    expect(state.pending).toBe(false);
    expect(rollback).toHaveBeenCalledOnce();

    followRequest.resolve({ following: true });
    await request;

    expect(state.following).toBe(true);
    expect(commit).not.toHaveBeenCalled();
  });
});
