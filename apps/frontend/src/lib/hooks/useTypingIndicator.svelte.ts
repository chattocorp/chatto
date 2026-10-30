import { onDestroy } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { useServerScope } from '$lib/state/server/scope.svelte';
import { createRoomCommandAPI } from '@chatto/client/api/rooms';
import { createMemberDirectoryAPI } from '@chatto/client/api/memberDirectory';
import { useTypingEvent, type TypingEventData } from './useEvent.svelte';

/** How long to display typing indicator after receiving an event (ms) */
export const TYPING_TIMEOUT_MS = 6000;

/** Minimum interval between sending typing indicator events (ms) */
const SEND_DEBOUNCE_MS = 2000;

interface TypingIndicatorConfig {
  roomId: string;
  threadRootEventId: string | null;
  currentUserId: string | null;
}

/**
 * Typing indicator hook for a room or thread.
 * MUST be called during component initialization (uses getContext).
 *
 * Each conversation owns its typing bursts and send throttle. Changing the
 * room or thread replaces that state; resolving the viewer does not.
 */
export function createTypingIndicator(getConfig: () => TypingIndicatorConfig) {
  const serverScope = useServerScope();

  const roomId = $derived(getConfig().roomId);
  const threadRootEventId = $derived(getConfig().threadRootEventId);
  const conversation = $derived({
    roomId,
    threadRootEventId,
    users: new SvelteMap<string, { lastTypingAt: number; profileReadAttempted: boolean }>(),
    lastSentAt: -Infinity
  });

  function handleTypingEvent(data: TypingEventData) {
    const currentUserId = getConfig().currentUserId;
    if (!conversation.roomId || !currentUserId || data.userId === currentUserId) return;
    if (
      data.roomId !== conversation.roomId ||
      data.threadRootEventId !== conversation.threadRootEventId
    )
      return;

    const previous = conversation.users.get(data.userId);
    const profiles = serverScope.store.projection.users;
    const needsProfile =
      !profiles.has(data.userId) &&
      !profiles.isDeleted(data.userId) &&
      !previous?.profileReadAttempted;
    conversation.users.set(data.userId, {
      lastTypingAt: Date.now(),
      profileReadAttempted: !!previous?.profileReadAttempted || needsProfile
    });
    if (needsProfile) {
      void serverScope.connection
        .getAPI(createMemberDirectoryAPI)
        .batchGetUsers([data.userId])
        // Typing is transient. A later burst can retry without failing this one.
        .catch(() => undefined);
    }
  }

  function cleanupExpired() {
    const now = Date.now();
    for (const [userId, user] of conversation.users) {
      if (now - user.lastTypingAt >= TYPING_TIMEOUT_MS) {
        conversation.users.delete(userId);
      }
    }
  }

  useTypingEvent(handleTypingEvent);
  const cleanupInterval = setInterval(cleanupExpired, 1000);

  onDestroy(() => clearInterval(cleanupInterval));

  return {
    /** Reactive list of user IDs currently typing (excludes current user) */
    get userIds(): string[] {
      const currentUserId = getConfig().currentUserId;
      return currentUserId
        ? Array.from(conversation.users.keys()).filter((id) => id !== currentUserId)
        : [];
    },

    /** Remove a user from the typing list (e.g. when they post a message) */
    removeTypingUser(userId: string) {
      conversation.users.delete(userId);
    },

    /** Reset send debounce (call after sending a message) */
    resetDebounce() {
      conversation.lastSentAt = -Infinity;
    },

    /** Send typing indicator to other users (debounced) */
    async sendTypingIndicator(): Promise<void> {
      if (!conversation.roomId) return;

      const now = Date.now();
      if (now - conversation.lastSentAt < SEND_DEBOUNCE_MS) return;
      conversation.lastSentAt = now;

      try {
        await serverScope.connection
          .getAPI(createRoomCommandAPI)
          .refreshTypingIndicator(conversation.roomId, conversation.threadRootEventId);
      } catch (err) {
        console.debug('Failed to send typing indicator:', err);
      }
    }
  };
}

export type TypingIndicator = ReturnType<typeof createTypingIndicator>;
