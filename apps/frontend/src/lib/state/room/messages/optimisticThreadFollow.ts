import type { RoomEventView } from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { OptimisticMutationRegistry } from '$lib/state/optimisticMutations';

type MessagePostedPayload = Extract<
  NonNullable<RoomEventView['event']>,
  { kind: typeof RoomEventKind.MessagePosted }
>;

export type OptimisticThreadFollowHandle = {
  applyServerState(isFollowing: boolean): void;
  rollback(): void;
};

type BeginOptimisticThreadFollowInput = {
  threadRootEventId: string;
  isFollowing: boolean;
  events: readonly RoomEventView[];
  registry: OptimisticMutationRegistry;
  setEvent(index: number, event: RoomEventView): void;
};

export function beginOptimisticThreadFollow(
  input: BeginOptimisticThreadFollowInput
): OptimisticThreadFollowHandle {
  const token = input.registry.createToken();
  const key = optimisticThreadFollowKey(input.threadRootEventId);
  const index = input.events.findIndex((event) => event.id === input.threadRootEventId);
  const event = index === -1 ? null : input.events[index];
  const previousState = isMessagePostedPayload(event?.event)
    ? (event.event.viewerIsFollowingThread ?? null)
    : null;

  if (event) {
    const updated = eventWithThreadFollowState(event, input.isFollowing);
    if (updated) {
      input.registry.mark(key, token);
      input.setEvent(index, updated);
    }
  }

  return {
    applyServerState: (isFollowing) => {
      if (!input.registry.isCurrent(key, token)) return;
      const currentIndex = input.events.findIndex((event) => event.id === input.threadRootEventId);
      if (currentIndex !== -1) {
        const updated = eventWithThreadFollowState(input.events[currentIndex], isFollowing);
        if (updated) input.setEvent(currentIndex, updated);
      }
      input.registry.clear(key);
    },
    rollback: () => {
      if (!input.registry.isCurrent(key, token)) return;
      const currentIndex = input.events.findIndex((event) => event.id === input.threadRootEventId);
      if (currentIndex !== -1) {
        const updated = eventWithThreadFollowState(input.events[currentIndex], previousState);
        if (updated) input.setEvent(currentIndex, updated);
      }
      input.registry.clear(key);
    }
  };
}

export function clearOptimisticThreadFollowForEvent(
  registry: OptimisticMutationRegistry,
  threadRootEventId: string
): void {
  registry.clear(optimisticThreadFollowKey(threadRootEventId));
}

function optimisticThreadFollowKey(threadRootEventId: string): string {
  return `thread-follow:${threadRootEventId}`;
}

function isMessagePostedPayload(
  event: RoomEventView['event'] | null | undefined
): event is MessagePostedPayload {
  return event?.kind === RoomEventKind.MessagePosted;
}

function eventWithThreadFollowState(
  event: RoomEventView,
  isFollowing: boolean | null
): RoomEventView | null {
  const payload = event.event;
  if (!isMessagePostedPayload(payload)) return null;
  return {
    ...event,
    event: {
      ...payload,
      viewerIsFollowingThread: isFollowing
    }
  };
}
