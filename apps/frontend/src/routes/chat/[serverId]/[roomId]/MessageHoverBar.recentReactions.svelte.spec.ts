import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync, tick } from 'svelte';
import EmojiPicker from '$lib/components/EmojiPicker.svelte';
import { PINNED_REACTIONS } from '$lib/emoji';
import { __resetRecentEmojisForTests, getRecentEmojis } from '$lib/state/recentEmojis.svelte';
import { serverStorageKey } from '@chatto/client/storage/serverStorage';
import MessageHoverBar from './MessageHoverBar.svelte';
import MessageActionMenuTestHarness from './MessageActionMenuTestHarness.svelte';
import MessageEventTestHarness from './MessageEventTestHarness.svelte';
import { MessageActionOverlayState } from './messageActionOverlayState.svelte';
import { buildMessageActionModel } from './messageActionModel';
import { messageEvent } from './messageEventFixture';

const SERVER_ID = 'recent-reactions-server';

const mocks = vi.hoisted(() => ({
  actions: {
    toggleReaction: vi.fn(),
    addReaction: vi.fn(),
    removeReaction: vi.fn(),
    startEdit: vi.fn(),
    openDeleteConfirmation: vi.fn(),
    copyMessageText: vi.fn(),
    copyMessageLink: vi.fn()
  }
}));

vi.mock('$lib/hooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/hooks')>();
  return { ...actual, useMessageActions: () => mocks.actions };
});

function buildAction() {
  return buildMessageActionModel({
    actions: mocks.actions,
    params: {
      serverId: SERVER_ID,
      roomId: 'room-1',
      messageEventId: 'message-event-1',
      eventId: 'event-1',
      messageBody: 'Hello'
    },
    reactions: [],
    canReact: true,
    canEdit: false,
    canDelete: false,
    replyInRoomLabel: 'Reply',
    replyThreadLabel: 'Reply in thread'
  });
}

function renderBar() {
  return render(MessageHoverBar, { props: { action: buildAction() } });
}

function quickReactionLabels(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('[aria-label^="React with "]'))
    .map((button) => button.getAttribute('aria-label')?.replace('React with ', '') ?? '')
    .filter(Boolean);
}

async function selectEmoji(container: HTMLElement, query: string, title: string) {
  const input = container.querySelector<HTMLInputElement>('input[placeholder="Search emojis..."]');
  if (!input) throw new Error('emoji search input not found');
  input.value = query;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  await tick();
  const button = container.querySelector<HTMLButtonElement>(`button[title="${title}"]`);
  if (!button) throw new Error('emoji search result not found');
  button.click();
  flushSync();
  await tick();
}

/** Renders the timeline's emoji picker for a message whose row is not mounted. */
function renderReactionPicker() {
  const event = messageEvent();
  const overlays = new MessageActionOverlayState();
  overlays.open(event.id, { kind: 'emoji', position: { x: 0, y: 0 }, presentation: 'auto' });
  const rendered = render(MessageEventTestHarness, {
    props: { event, serverId: SERVER_ID, showMessage: false, actionOverlays: overlays }
  });
  return { ...rendered, overlays };
}

beforeEach(() => {
  localStorage.clear();
  __resetRecentEmojisForTests();
  vi.clearAllMocks();
});

describe('Recent quick reactions integration', () => {
  it('updates all mounted quick-reaction surfaces through the real reaction picker handler', async () => {
    const bar = renderBar();
    const menu = render(MessageActionMenuTestHarness, {
      props: { action: buildAction(), onClose: vi.fn() }
    });
    const sheet = render(MessageActionMenuTestHarness, {
      props: { action: buildAction(), presentation: 'sheet', onClose: vi.fn() }
    });
    const surfaces = [bar, menu, sheet];
    for (const surface of surfaces) {
      expect(quickReactionLabels(surface.container)).toEqual([...PINNED_REACTIONS]);
    }

    const picker = renderReactionPicker();
    await vi.waitFor(() => expect(picker.container.querySelector('input')).not.toBeNull());
    await selectEmoji(picker.container, 'check', 'white_check_mark');
    expect(picker.overlays.current).toBeNull();
    expect(mocks.actions.toggleReaction).toHaveBeenCalledWith(expect.anything(), '✅', false);
    for (const surface of surfaces) {
      expect(quickReactionLabels(surface.container)).toEqual([...PINNED_REACTIONS, '✅']);
    }
    expect(getRecentEmojis(SERVER_ID).recent).toEqual(['✅']);
  });

  it('continues updating after the component that first created the store is destroyed', async () => {
    // First access must occur in a mounted component, not in the test body.
    const first = renderBar();
    expect(quickReactionLabels(first.container)).toEqual([...PINNED_REACTIONS]);
    const store = getRecentEmojis(SERVER_ID);
    store.recordReaction('🔥');
    flushSync();
    expect(quickReactionLabels(first.container)).toEqual([...PINNED_REACTIONS, '🔥']);
    await first.unmount();

    const second = renderBar();
    store.recordReaction('🚀');
    flushSync();
    expect(quickReactionLabels(second.container)).toEqual([...PINNED_REACTIONS, '🚀', '🔥']);
    store.recordReaction('✅');
    flushSync();
    expect(quickReactionLabels(second.container)).toEqual([...PINNED_REACTIONS, '✅', '🚀']);
  });

  it('keeps profile-status picker choices in general history only', async () => {
    getRecentEmojis(SERVER_ID).recordReaction('🔥');
    const bar = renderBar();
    const picker = render(EmojiPicker, {
      props: { serverId: SERVER_ID, onSelect: vi.fn(), onClose: vi.fn() }
    });
    await selectEmoji(picker.container, 'rocket', 'rocket');
    expect(getRecentEmojis(SERVER_ID).recent).toEqual(['🚀']);
    expect(quickReactionLabels(bar.container)).toEqual([...PINNED_REACTIONS, '🔥']);
  });

  it('records the choice before awaiting the reaction request', async () => {
    let completeRequest!: () => void;
    mocks.actions.toggleReaction.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          completeRequest = resolve;
        })
    );
    const picker = renderReactionPicker();
    await vi.waitFor(() => expect(picker.container.querySelector('input')).not.toBeNull());
    await selectEmoji(picker.container, 'fire', 'fire');
    expect(getRecentEmojis(SERVER_ID).quickReactions).toEqual([...PINNED_REACTIONS, '🔥']);
    expect(picker.overlays.current).toBeNull();
    completeRequest();
  });

  it('hydrates recent quick reactions from the new server-scoped storage key', () => {
    localStorage.setItem(
      serverStorageKey(SERVER_ID, 'recentReactions'),
      JSON.stringify(['🔥', '🚀'])
    );
    expect(quickReactionLabels(renderBar().container)).toEqual([...PINNED_REACTIONS, '🔥', '🚀']);
  });

  it.each(['toolbar', 'menu', 'sheet'] as const)(
    'does not reorder history when a %s quick reaction is clicked',
    async (surface) => {
      const store = getRecentEmojis(SERVER_ID);
      store.recordReaction('🔥');
      store.recordReaction('🚀');
      const rendered =
        surface === 'toolbar'
          ? renderBar()
          : render(MessageActionMenuTestHarness, {
              props: { action: buildAction(), presentation: surface, onClose: vi.fn() }
            });
      const before = [...store.quickReactions];
      rendered.container.querySelector<HTMLButtonElement>('[aria-label="React with 🔥"]')!.click();
      await vi.waitFor(() => expect(mocks.actions.toggleReaction).toHaveBeenCalledOnce());
      expect(store.quickReactions).toEqual(before);
    }
  );
});
