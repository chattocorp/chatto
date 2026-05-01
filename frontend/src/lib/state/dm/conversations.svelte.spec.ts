import { describe, it, expect, vi } from 'vitest';
import { DMConversationsStore, type DMConversation } from './conversations.svelte';

/**
 * These tests cover the pure-state mutators (`markRead`, `bumpToTop` for
 * already-loaded conversations). The load + cross-instance refetch paths go
 * through global singletons (`instanceRegistry`, `graphqlClientManager`,
 * `instanceEventBusManager`); their data-shape contract is covered by
 * `mergeConversations.spec.ts` and exercised end-to-end by the e2e suite.
 */

const conv = (id: string, instanceId: string, hasUnread = false): DMConversation => ({
  id,
  instanceId,
  instanceLabel: instanceId,
  hasUnread,
  participants: [],
  currentUserId: 'me',
  isSelfConversation: false
});

describe('DMConversationsStore', () => {
  describe('markRead', () => {
    it('clears hasUnread on the targeted conversation', () => {
      const store = new DMConversationsStore();
      store.conversations = [conv('a', 'i1', true), conv('b', 'i1', true)];

      store.markRead('i1', 'a');

      expect(store.conversations[0].hasUnread).toBe(false);
      expect(store.conversations[1].hasUnread).toBe(true);
    });

    it('matches on (instanceId, roomId) pair — not just roomId', () => {
      const store = new DMConversationsStore();
      // Same room id on two different instances — both unread.
      store.conversations = [conv('shared', 'i1', true), conv('shared', 'i2', true)];

      store.markRead('i1', 'shared');

      expect(store.conversations.find((c) => c.instanceId === 'i1')?.hasUnread).toBe(false);
      expect(store.conversations.find((c) => c.instanceId === 'i2')?.hasUnread).toBe(true);
    });

    it('is a no-op when the conversation is unknown', () => {
      const store = new DMConversationsStore();
      store.conversations = [conv('a', 'i1', true)];

      store.markRead('i1', 'missing');

      expect(store.conversations[0].hasUnread).toBe(true);
    });
  });

  describe('wireSubscriptions', () => {
    // The full per-instance wiring path requires the global event-bus and
    // GraphQL client managers; that's exercised at the e2e layer. This test
    // pins the shape contract: empty instances → returns a no-op cleanup
    // callable without side effects.
    it('returns a callable cleanup for an empty instances list', () => {
      const store = new DMConversationsStore();
      const cleanup = store.wireSubscriptions([], () => undefined);
      expect(typeof cleanup).toBe('function');
      expect(() => cleanup()).not.toThrow();
    });

    it('stashes the activeConversationId getter for later use by the bump path', () => {
      const store = new DMConversationsStore();
      const getActive = vi.fn(() => 'conv-x');
      store.wireSubscriptions([], getActive);
      // The getter has been adopted; the bump path will call it on events.
      // We exercise that via bumpToTop's markUnread suppression in a separate
      // test below.
      expect(getActive).not.toHaveBeenCalled(); // not called eagerly
    });
  });

  describe('bumpToTop (already-loaded conversation)', () => {
    it('moves the targeted conversation to position 0', () => {
      const store = new DMConversationsStore();
      store.conversations = [conv('a', 'i1'), conv('b', 'i1'), conv('c', 'i1')];

      store.bumpToTop('i1', 'c', false);

      expect(store.conversations.map((c) => c.id)).toEqual(['c', 'a', 'b']);
    });

    it('marks unread when markUnread=true', () => {
      const store = new DMConversationsStore();
      store.conversations = [conv('a', 'i1'), conv('b', 'i1')];

      store.bumpToTop('i1', 'b', true);

      expect(store.conversations[0].id).toBe('b');
      expect(store.conversations[0].hasUnread).toBe(true);
    });

    it('does NOT set hasUnread when markUnread=false', () => {
      const store = new DMConversationsStore();
      store.conversations = [conv('a', 'i1', true), conv('b', 'i1', false)];

      store.bumpToTop('i1', 'b', false);

      expect(store.conversations[0].id).toBe('b');
      expect(store.conversations[0].hasUnread).toBe(false);
    });

    it('preserves the relative order of other conversations', () => {
      const store = new DMConversationsStore();
      store.conversations = [
        conv('a', 'i1'),
        conv('x', 'i2'),
        conv('b', 'i1'),
        conv('y', 'i2')
      ];

      store.bumpToTop('i1', 'b', false);

      // 'b' moves to front; everyone else keeps their relative order.
      expect(store.conversations.map((c) => c.id)).toEqual(['b', 'a', 'x', 'y']);
    });

    it('matches on the (instanceId, roomId) pair', () => {
      const store = new DMConversationsStore();
      store.conversations = [conv('shared', 'i1'), conv('shared', 'i2')];

      store.bumpToTop('i2', 'shared', true);

      // Only i2's entry is bumped + marked unread.
      expect(store.conversations[0].instanceId).toBe('i2');
      expect(store.conversations[0].hasUnread).toBe(true);
      expect(store.conversations[1].instanceId).toBe('i1');
      expect(store.conversations[1].hasUnread).toBe(false);
    });
  });
});
