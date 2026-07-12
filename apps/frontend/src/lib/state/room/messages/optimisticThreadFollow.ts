import type { RoomEventView } from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { OptimisticMutationRegistry } from '$lib/state/optimisticMutations';

type MessagePostedPayload = Extract<
  NonNullable<RoomEventView['event']>,
  { kind: typeof RoomEventKind.MessagePosted }
>;

export type OptimisticThreadFollowHandle = {
  rollback(): void;
};

type BeginOptimisticThreadFollowInput = {
  threadRootEventId: string;
  isFollowing: boolean;
  getEvents(): readonly RoomEventView[];
  registry: OptimisticMutationRegistry;
  setEvent(eventId: string, event: RoomEventView): void;
};

export function beginOptimisticThreadFollow(
  input: BeginOptimisticThreadFollowInput
): OptimisticThreadFollowHandle {
  const token = input.registry.createToken();
  const key = optimisticThreadFollowKey(input.threadRootEventId);
  const event = input.getEvents().find((event) => event.id === input.threadRootEventId) ?? null;
  const previousState = isMessagePostedPayload(event?.event)
    ? (event.event.viewerIsFollowingThread ?? null)
    : null;

  if (event) {
    const updated = eventWithThreadFollowState(event, input.isFollowing);
    if (updated) {
      input.registry.mark(key, token);
      input.setEvent(event.id, updated);
    }
  }

  return {
    rollback: () => {
      if (!input.registry.isCurrent(key, token)) return;
      const event = input.getEvents().find((event) => event.id === input.threadRootEventId);
      if (event) {
        const updated = eventWithThreadFollowState(event, previousState);
        if (updated) input.setEvent(event.id, updated);
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
