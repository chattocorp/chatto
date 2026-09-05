import { afterEach, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { notifyRoomMessageMutated } from '$lib/state/room/messageMutationEvents';
import { useTimelineMutations } from './useTimelineMutations.svelte';

let dispose: (() => void) | undefined;
afterEach(() => dispose?.());

function timeline() {
  return {
    applyLocalMessageDeletion: vi.fn(),
    refreshAnchorForMessageMutation: vi.fn().mockReturnValue('visible-echo'),
    refreshCurrentWindow: vi.fn().mockResolvedValue(undefined)
  };
}

it('filters scope, removes deleted rows, and refreshes only visible mutation anchors', () => {
  const target = timeline();
  dispose = $effect.root(() => {
    useTimelineMutations(() => ({ serverId: 'server-a', roomId: 'room-a', timeline: target }));
  });
  flushSync();
  const mutation = {
    serverId: 'server-a',
    roomId: 'room-a',
    eventId: 'original-message',
    reason: 'attachment-deleted' as const
  };
  notifyRoomMessageMutated({ ...mutation, serverId: 'server-b' });
  notifyRoomMessageMutated({ ...mutation, roomId: 'room-b' });
  expect(target.refreshAnchorForMessageMutation).not.toHaveBeenCalled();

  notifyRoomMessageMutated(mutation);
  expect(target.refreshCurrentWindow).toHaveBeenCalledWith('visible-echo');
  target.refreshCurrentWindow.mockClear();
  target.refreshAnchorForMessageMutation.mockReturnValue(null);
  notifyRoomMessageMutated({ ...mutation, reason: 'link-preview-deleted' });
  expect(target.refreshCurrentWindow).not.toHaveBeenCalled();

  target.refreshAnchorForMessageMutation.mockClear();
  notifyRoomMessageMutated({ ...mutation, reason: 'message-deleted' });
  expect(target.applyLocalMessageDeletion).toHaveBeenCalledWith('original-message');
  expect(target.refreshAnchorForMessageMutation).not.toHaveBeenCalled();
});

it('uses the current server, room, and thread target and unsubscribes on disposal', () => {
  const first = timeline();
  const second = timeline();
  const scope = $state({ serverId: 'server-a', roomId: 'room-a' });
  let selected = $state.raw(first);
  dispose = $effect.root(() => {
    useTimelineMutations(() => ({ ...scope, timeline: selected }));
  });
  flushSync();
  const mutation = { ...scope, eventId: 'message', reason: 'message-deleted' as const };
  Object.assign(scope, { serverId: 'server-b', roomId: 'room-b' });
  selected = second;
  // A DOM event can arrive before the next effect flush after route reuse.
  notifyRoomMessageMutated(mutation);
  notifyRoomMessageMutated({ ...mutation, ...scope });
  expect(first.applyLocalMessageDeletion).not.toHaveBeenCalled();
  expect(second.applyLocalMessageDeletion).toHaveBeenCalledOnce();

  selected = first;
  notifyRoomMessageMutated({ ...mutation, ...scope });
  expect(first.applyLocalMessageDeletion).toHaveBeenCalledOnce();
  dispose();
  dispose = undefined;
  notifyRoomMessageMutated({ ...mutation, ...scope });
  expect(first.applyLocalMessageDeletion).toHaveBeenCalledOnce();
});
