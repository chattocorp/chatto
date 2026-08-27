import {
  TimelineEventKind,
  type TimelineEventPayload,
  type TimelineEventView
} from '$lib/render/timelineEvents';
import type { OptimisticMutationRegistry } from '$lib/state/optimisticMutations';

type MessagePostedPayload = Extract<
  TimelineEventPayload,
  { kind: typeof TimelineEventKind.MessagePosted }
>;

export type OptimisticThreadFollowHandle = {
  rollback(): void;
};

type BeginOptimisticThreadFollowInput = {
  threadRootEventId: string;
  isFollowing: boolean;
  getEvents(): readonly TimelineEventView[];
  registry: OptimisticMutationRegistry;
  setEvent(eventId: string, event: TimelineEventView): void;
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
  event: TimelineEventView['event'] | null | undefined
): event is MessagePostedPayload {
  return event?.kind === TimelineEventKind.MessagePosted;
}

function eventWithThreadFollowState(
  event: TimelineEventView,
  isFollowing: boolean | null
): TimelineEventView | null {
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
