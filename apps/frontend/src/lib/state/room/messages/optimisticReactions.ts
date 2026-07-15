import type { RoomEventView } from '$lib/render/types';
import { RoomEventKind } from '$lib/render/eventKinds';
import type { OptimisticMutationRegistry } from '$lib/state/optimisticMutations';

type RoomEventPayload = NonNullable<RoomEventView['event']>;
type MessagePostedPayload = Extract<RoomEventPayload, { kind: typeof RoomEventKind.MessagePosted }>;
type MessageReactionSummary = MessagePostedPayload['reactions'][number];

export type OptimisticReactionAction = 'add' | 'remove';

export type OptimisticReactionServerSummary = {
  emoji: string;
  count: number;
  hasReacted: boolean;
} | null;

export type OptimisticReactionHandle = {
  applyServerReaction(reaction: OptimisticReactionServerSummary): void;
  rollback(): void;
};

type OptimisticReactionSnapshot = {
  key: string;
  emoji: string;
  previousReaction: MessageReactionSummary | null;
  source: 'events' | 'preview';
  eventId?: string;
  previewKey?: string;
};

type BeginOptimisticReactionInput = {
  messageEventId: string;
  emoji: string;
  action: OptimisticReactionAction;
  getEvents(): readonly RoomEventView[];
  previews: Iterable<readonly [string, RoomEventView | null]>;
  registry: OptimisticMutationRegistry;
  setEvent(eventId: string, event: RoomEventView): void;
  setPreview(key: string, event: RoomEventView): void;
};

export function beginOptimisticReaction(
  input: BeginOptimisticReactionInput
): OptimisticReactionHandle {
  const token = input.registry.createToken();
  const events = input.getEvents();
  const targetIds = optimisticReactionTargetIds(input.messageEventId, events, input.previews);
  const snapshots: OptimisticReactionSnapshot[] = [];

  const record = (snapshot: OptimisticReactionSnapshot) => {
    snapshots.push(snapshot);
    input.registry.mark(snapshot.key, token);
  };

  events.forEach((event) => {
    if (!isReactionTarget(event, targetIds)) return;
    const updated = eventWithOptimisticReaction(event, input.emoji, input.action);
    if (!updated) return;

    record({
      key: optimisticReactionEventKey(event.id, input.emoji),
      emoji: input.emoji,
      previousReaction: reactionSummary(event, input.emoji),
      source: 'events',
      eventId: event.id
    });
    input.setEvent(event.id, updated);
  });

  for (const [previewKey, event] of input.previews) {
    if (!event || !isReactionTarget(event, targetIds)) continue;
    const updated = eventWithOptimisticReaction(event, input.emoji, input.action);
    if (!updated) continue;

    record({
      key: optimisticReactionPreviewKey(previewKey, input.emoji),
      emoji: input.emoji,
      previousReaction: reactionSummary(event, input.emoji),
      source: 'preview',
      previewKey
    });
    input.setPreview(previewKey, updated);
  }

  return {
    applyServerReaction: (reaction) => {
      for (const snapshot of snapshots) {
        if (!input.registry.isCurrent(snapshot.key, token)) continue;
        applyServerReactionSnapshot(snapshot, input.emoji, reaction, input);
        input.registry.clear(snapshot.key);
      }
    },
    rollback: () => {
      for (const snapshot of snapshots) {
        if (!input.registry.isCurrent(snapshot.key, token)) continue;
        restoreOptimisticReactionSnapshot(snapshot, input);
        input.registry.clear(snapshot.key);
      }
    }
  };
}

export function clearOptimisticReactionsForEvent(
  registry: OptimisticMutationRegistry,
  eventId: string,
  previewKey: string
): void {
  registry.clearPrefixes([
    optimisticReactionEventKeyPrefix(eventId),
    optimisticReactionPreviewKeyPrefix(previewKey)
  ]);
}

function optimisticReactionEventKey(eventId: string, emoji: string): string {
  return `${optimisticReactionEventKeyPrefix(eventId)}${emoji}`;
}

function optimisticReactionEventKeyPrefix(eventId: string): string {
  return `events:${eventId}\u0000`;
}

function optimisticReactionPreviewKey(previewKey: string, emoji: string): string {
  return `${optimisticReactionPreviewKeyPrefix(previewKey)}${emoji}`;
}

function optimisticReactionPreviewKeyPrefix(previewKey: string): string {
  return `preview:${previewKey}\u0000`;
}

function optimisticReactionTargetIds(
  messageEventId: string,
  events: readonly RoomEventView[],
  previews: Iterable<readonly [string, RoomEventView | null]>
): Set<string> {
  const targetIds = new Set([messageEventId]);
  let changed = true;

  while (changed) {
    changed = false;
    for (const event of events) {
      changed = addLinkedReactionTargetIds(event, targetIds) || changed;
    }
    for (const [, event] of previews) {
      if (event) changed = addLinkedReactionTargetIds(event, targetIds) || changed;
    }
  }

  return targetIds;
}

function addLinkedReactionTargetIds(event: RoomEventView, targetIds: Set<string>): boolean {
  const payload = event.event;
  if (!isMessagePostedPayload(payload)) return false;

  const before = targetIds.size;
  if (targetIds.has(event.id)) {
    if (payload.echoOfEventId) targetIds.add(payload.echoOfEventId);
    if (payload.channelEchoEventId) targetIds.add(payload.channelEchoEventId);
  }
  if (payload.echoOfEventId && targetIds.has(payload.echoOfEventId)) targetIds.add(event.id);
  if (payload.channelEchoEventId && targetIds.has(payload.channelEchoEventId)) {
    targetIds.add(event.id);
  }
  return targetIds.size !== before;
}

function isReactionTarget(event: RoomEventView, targetIds: Set<string>): boolean {
  if (targetIds.has(event.id)) return true;
  const payload = event.event;
  return (
    isMessagePostedPayload(payload) &&
    Boolean(
      (payload.echoOfEventId && targetIds.has(payload.echoOfEventId)) ||
      (payload.channelEchoEventId && targetIds.has(payload.channelEchoEventId))
    )
  );
}

function isMessagePostedPayload(
  event: RoomEventView['event'] | null | undefined
): event is MessagePostedPayload {
  return event?.kind === RoomEventKind.MessagePosted;
}

function eventWithOptimisticReaction(
  event: RoomEventView,
  emoji: string,
  action: OptimisticReactionAction
): RoomEventView | null {
  const payload = event.event;
  if (!isMessagePostedPayload(payload)) return null;
  return {
    ...event,
    event: {
      ...payload,
      reactions: optimisticReactions(payload.reactions, emoji, action)
    }
  };
}

function reactionSummary(event: RoomEventView, emoji: string): MessageReactionSummary | null {
  const payload = event.event;
  if (!isMessagePostedPayload(payload)) return null;
  return payload.reactions.find((reaction) => reaction.emoji === emoji) ?? null;
}

function eventWithReactionSummary(
  event: RoomEventView,
  emoji: string,
  reaction: MessageReactionSummary | null
): RoomEventView | null {
  const payload = event.event;
  if (!isMessagePostedPayload(payload)) return null;
  return {
    ...event,
    event: {
      ...payload,
      reactions: reactionsWithSummary(payload.reactions, emoji, reaction)
    }
  };
}

function optimisticReactions(
  reactions: readonly MessageReactionSummary[],
  emoji: string,
  action: OptimisticReactionAction
): MessageReactionSummary[] {
  const existingIndex = reactions.findIndex((reaction) => reaction.emoji === emoji);
  if (action === 'add') {
    if (existingIndex === -1) {
      return [...reactions, { emoji, count: 1, hasReacted: true, users: [] }];
    }

    return reactions.map((reaction, index) =>
      index === existingIndex
        ? {
            ...reaction,
            count: reaction.hasReacted ? reaction.count : reaction.count + 1,
            hasReacted: true
          }
        : reaction
    );
  }

  if (existingIndex === -1) return [...reactions];

  return reactions.flatMap((reaction, index) => {
    if (index !== existingIndex) return [reaction];
    const count = reaction.hasReacted ? Math.max(0, reaction.count - 1) : reaction.count;
    if (count === 0) return [];
    return [{ ...reaction, count, hasReacted: false }];
  });
}

function reactionsWithSummary(
  reactions: readonly MessageReactionSummary[],
  emoji: string,
  reaction: MessageReactionSummary | null
): MessageReactionSummary[] {
  if (!reaction || reaction.count <= 0) {
    return reactions.filter((existing) => existing.emoji !== emoji);
  }

  const existingIndex = reactions.findIndex((existing) => existing.emoji === emoji);
  const nextReaction = {
    ...reaction,
    users: [...reaction.users]
  };
  if (existingIndex === -1) return [...reactions, nextReaction];
  return reactions.map((existing, index) => (index === existingIndex ? nextReaction : existing));
}

function serverReactions(
  reactions: readonly MessageReactionSummary[],
  emoji: string,
  reaction: OptimisticReactionServerSummary
): MessageReactionSummary[] {
  if (!reaction || reaction.count <= 0) {
    return reactions.filter((existing) => existing.emoji !== emoji);
  }

  const existingIndex = reactions.findIndex((existing) => existing.emoji === emoji);
  const nextReaction = (existing?: MessageReactionSummary): MessageReactionSummary => ({
    emoji: reaction.emoji,
    count: reaction.count,
    hasReacted: reaction.hasReacted,
    users: existing?.users ?? []
  });

  if (existingIndex === -1) return [...reactions, nextReaction()];
  return reactions.map((existing, index) =>
    index === existingIndex ? nextReaction(existing) : existing
  );
}

function applyServerReactionSnapshot(
  snapshot: OptimisticReactionSnapshot,
  emoji: string,
  reaction: OptimisticReactionServerSummary,
  input: BeginOptimisticReactionInput
): void {
  const apply = (event: RoomEventView): RoomEventView | null => {
    const payload = event.event;
    if (!isMessagePostedPayload(payload)) return null;
    return {
      ...event,
      event: {
        ...payload,
        reactions: serverReactions(payload.reactions, emoji, reaction)
      }
    };
  };

  if (snapshot.source === 'events') {
    const event = input.getEvents().find((event) => event.id === snapshot.eventId);
    if (!event) return;
    const updated = apply(event);
    if (updated) input.setEvent(event.id, updated);
    return;
  }

  if (!snapshot.previewKey) return;
  const event = previewEvent(input.previews, snapshot.previewKey);
  if (!event) return;
  const updated = apply(event);
  if (updated) input.setPreview(snapshot.previewKey, updated);
}

function restoreOptimisticReactionSnapshot(
  snapshot: OptimisticReactionSnapshot,
  input: BeginOptimisticReactionInput
): void {
  if (snapshot.source === 'events') {
    const event = input.getEvents().find((event) => event.id === snapshot.eventId);
    if (!event) return;
    const updated = eventWithReactionSummary(event, snapshot.emoji, snapshot.previousReaction);
    if (updated) input.setEvent(event.id, updated);
    return;
  }

  if (!snapshot.previewKey) return;
  const event = previewEvent(input.previews, snapshot.previewKey);
  if (!event) return;
  const updated = eventWithReactionSummary(event, snapshot.emoji, snapshot.previousReaction);
  if (updated) input.setPreview(snapshot.previewKey, updated);
}

function previewEvent(
  previews: Iterable<readonly [string, RoomEventView | null]>,
  previewKey: string
): RoomEventView | null {
  for (const [key, event] of previews) {
    if (key === previewKey) return event;
  }
  return null;
}
