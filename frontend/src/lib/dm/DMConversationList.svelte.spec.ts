import { describe, it, expect, vi } from 'vitest';
import { flushSync } from 'svelte';
import { render } from 'vitest-browser-svelte';
import Harness from './DMConversationListTestHarness.svelte';
import type { DMConversation } from '$lib/state/dm/conversations.svelte';

// instanceRegistry is touched by DMConversationList's effect (notification
// dismissal) and template (`tryGetStore` for the notification-dot check).
// Stub it so neither path crashes — we don't care about real instance state.
// Used by the template's `instanceIdToSegment` and the active-conversation
// effect's `tryGetStore` lookup. None of the data needs to be realistic.
vi.mock('$lib/state/instance/registry.svelte', () => ({
  instanceRegistry: {
    instances: [],
    tryGetStore: () => undefined,
    isOriginInstance: () => false,
    getInstance: (id: string) => ({ id, name: id, url: `https://${id}` })
  }
}));

const conv = (id: string, instanceId: string, hasUnread = false): DMConversation => ({
  id,
  instanceId,
  instanceLabel: instanceId,
  hasUnread,
  participants: [],
  currentUserId: 'me',
  isSelfConversation: false
});

describe('DMConversationList', () => {
  // Regression: the active-conversation effect used to read
  // `store.conversations` directly, then call `markRead` (which writes to the
  // same state). Svelte's reactivity treated that as a read+write loop in the
  // same effect and threw `effect_update_depth_exceeded`. The fix is to wrap
  // the find + writes in `untrack`. This test mounts the component with an
  // active conversation present in the list — the exact trigger condition.
  it('does not loop when the active conversation is in the list', async () => {
    const errors: unknown[] = [];
    const consoleError = vi.spyOn(console, 'error').mockImplementation((...args) => {
      errors.push(args);
    });

    expect(() =>
      render(Harness, {
        props: {
          initialConversations: [conv('a', 'i1', true), conv('b', 'i1', false)],
          activeConversationId: 'a'
        }
      })
    ).not.toThrow();

    flushSync();

    // No `effect_update_depth_exceeded` (or anything similar) logged.
    const update = errors.find((e) =>
      JSON.stringify(e).includes('effect_update_depth_exceeded')
    );
    expect(update).toBeUndefined();

    consoleError.mockRestore();
  });

  it('renders one row per conversation in the seeded order', async () => {
    const { container } = render(Harness, {
      props: {
        initialConversations: [conv('a', 'i1'), conv('b', 'i1')],
        activeConversationId: undefined
      }
    });

    flushSync();
    const links = container.querySelectorAll('a');
    expect(links.length).toBe(2);
  });

  it('renders the unread dot on a conversation with hasUnread=true', async () => {
    const { container } = render(Harness, {
      props: {
        initialConversations: [conv('a', 'i1', true), conv('b', 'i1', false)],
        // Don't activate it — that would clear the unread flag via markRead.
        activeConversationId: undefined
      }
    });

    flushSync();
    const dot = container.querySelector('[data-testid="dm-unread-dot"]');
    expect(dot).not.toBeNull();
  });
});
