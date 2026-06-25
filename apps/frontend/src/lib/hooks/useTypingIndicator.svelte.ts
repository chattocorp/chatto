import { SvelteMap } from 'svelte/reactivity';
import { onTypingEvent, type TypingEventData } from '$lib/eventBus.svelte';
import { useConnection } from '$lib/state/server/connection.svelte';
import { graphql } from '$lib/gql';

const SendTypingIndicatorMutation = graphql(`
  mutation SendTypingIndicator($input: SendTypingIndicatorInput!) {
    sendTypingIndicator(input: $input)
  }
`);

/** How long to display typing indicator after receiving an event (ms) */
export const TYPING_TIMEOUT_MS = 6000;

/** Minimum interval between sending typing indicator events (ms) */
const SEND_DEBOUNCE_MS = 2000;

export interface TypingUser {
  userId: string;
  /** When we last received a typing event from this user */
  lastTypingAt: number;
}

interface TypingIndicatorConfig {
  roomId: string;
  threadRootEventId: string | null;
  currentUserId: string | null;
}

/**
 * Typing indicator hook for a room or thread.
 * MUST be called during component initialization (uses getContext).
 *
 * Accepts a getter that returns the current config. The getter is called
 * inside an $effect, so reactive values read within it are automatically
 * tracked.
 */
export function createTypingIndicator(getConfig: () => TypingIndicatorConfig) {
  const connection = useConnection();

  /** Current configuration snapshot */
  let configRoomId: string | null = null;
  let configThreadRootEventId: string | null = null;
  let configCurrentUserId: string | null = null;

  /** Map of userId -> TypingUser for users currently typing */
  const typingUsers = new SvelteMap<string, TypingUser>();

  /** Version counter to force reactivity updates */
  const state = $state({ version: 0 });

  /** Timestamp of last sent typing indicator */
  let lastSentAt = 0;

  function handleTypingEvent(data: TypingEventData) {
    if (!configRoomId || !configCurrentUserId) return;

    if (data.roomId !== configRoomId) return;

    // Check thread context matches
    const eventThread = data.threadRootEventId;
    if (configThreadRootEventId !== null) {
      if (eventThread !== configThreadRootEventId) return;
    } else {
      if (eventThread !== null) return;
    }

    if (data.userId === configCurrentUserId) return;

    typingUsers.set(data.userId, {
      userId: data.userId,
      lastTypingAt: Date.now()
    });
    state.version++;
  }

  function cleanupExpired() {
    const now = Date.now();
    let changed = false;
    for (const [userId, user] of typingUsers) {
      if (now - user.lastTypingAt >= TYPING_TIMEOUT_MS) {
        typingUsers.delete(userId);
        changed = true;
      }
    }
    if (changed) {
      state.version++;
    }
  }

  // Subscribe to typing events
  const unsubscribe = onTypingEvent(handleTypingEvent);
  const cleanupInterval = setInterval(cleanupExpired, 1000);

  // Sync config reactively — getConfig() is called inside the $effect,
  // so Svelte tracks whatever reactive values the caller reads in their closure.
  $effect(() => {
    const config = getConfig();

    // Clear typing users when room/thread changes
    if (
      configRoomId !== null &&
      (configRoomId !== config.roomId || configThreadRootEventId !== config.threadRootEventId)
    ) {
      typingUsers.clear();
    }

    configRoomId = config.roomId;
    configThreadRootEventId = config.threadRootEventId;
    configCurrentUserId = config.currentUserId;
  });

  // Cleanup on destroy
  $effect(() => {
    return () => {
      unsubscribe();
      clearInterval(cleanupInterval);
      typingUsers.clear();
    };
  });

  return {
    /** Reactive list of user IDs currently typing (excludes current user) */
    get userIds(): string[] {
      void state.version;
      if (!configCurrentUserId) return [];
      return Array.from(typingUsers.keys()).filter((id) => id !== configCurrentUserId);
    },

    /** Remove a user from the typing list (e.g. when they post a message) */
    removeTypingUser(userId: string) {
      if (typingUsers.has(userId)) {
        typingUsers.delete(userId);
        state.version++;
      }
    },

    /** Reset send debounce (call after sending a message) */
    resetDebounce() {
      lastSentAt = 0;
    },

    /** Send typing indicator to other users (debounced) */
    async sendTypingIndicator(): Promise<void> {
      if (!configRoomId) return;

      const now = Date.now();
      if (now - lastSentAt < SEND_DEBOUNCE_MS) return;
      lastSentAt = now;

      try {
        await connection().client.mutation(SendTypingIndicatorMutation, {
          input: {
            roomId: configRoomId,
            threadRootEventId: configThreadRootEventId
          }
        });
      } catch (err) {
        console.debug('Failed to send typing indicator:', err);
      }
    }
  };
}

export type TypingIndicator = ReturnType<typeof createTypingIndicator>;
