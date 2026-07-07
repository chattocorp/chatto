import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync, tick } from 'svelte';
import EmojiPicker from '$lib/components/EmojiPicker.svelte';
import { PINNED_REACTIONS } from '$lib/emoji';
import { __resetRecentEmojisForTests, getRecentEmojis } from '$lib/state/recentEmojis.svelte';
import { serverStorageKey } from '$lib/storage/serverStorage';
import MessageHoverBar from './MessageHoverBar.svelte';

const SERVER_ID = 'recent-reactions-server';

const mocks = vi.hoisted(() => ({
  actions: {
    toggleReaction: vi.fn(),
    startEdit: vi.fn(),
    openDeleteConfirmation: vi.fn()
  }
}));

vi.mock('$lib/hooks', () => ({
  useMessageActions: () => mocks.actions
}));

function renderBar() {
  return render(MessageHoverBar, {
    props: {
      serverId: SERVER_ID,
      roomId: 'room-1',
      messageEventId: 'message-event-1',
      eventId: 'event-1',
      messageBody: 'Hello',
      canReact: true
    }
  });
}

function quickReactionLabels(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll<HTMLButtonElement>('[aria-label^="React with "]'))
    .map((button) => button.getAttribute('aria-label')?.replace('React with ', '') ?? '')
    .filter(Boolean);
}

function searchInput(container: HTMLElement): HTMLInputElement {
  const input = container.querySelector<HTMLInputElement>('input[placeholder="Search emojis..."]');
  if (!input) throw new Error('emoji search input not found');
  return input;
}

async function searchEmoji(container: HTMLElement, query: string) {
  const input = searchInput(container);
  input.value = query;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  await tick();
}

beforeEach(() => {
  localStorage.clear();
  __resetRecentEmojisForTests();
  vi.clearAllMocks();
});

describe('MessageHoverBar recent reactions integration', () => {
  it('uses a checkmark emoji selected in the picker as the first non-pinned quick reaction', async () => {
    const bar = renderBar();
    expect(quickReactionLabels(bar.container)).toEqual(['👍', '👋', '🤣', '🙏', '❤️', '😂']);

    const picker = render(EmojiPicker, {
      props: {
        serverId: SERVER_ID,
        onSelect: vi.fn(),
        onClose: vi.fn()
      }
    });
    await searchEmoji(picker.container, 'check');
    (picker.container.querySelector('button[title="white_check_mark"]') as HTMLButtonElement).click();
    flushSync();
    await tick();

    const reactions = quickReactionLabels(bar.container);
    expect(reactions.slice(0, PINNED_REACTIONS.length)).toEqual([...PINNED_REACTIONS]);
    expect(reactions[PINNED_REACTIONS.length]).toBe('✅');
    expect(reactions).toHaveLength(6);
    expect(reactions).not.toContain('😂');
  });

  it('hydrates recent quick reactions from server-scoped localStorage', () => {
    localStorage.setItem(serverStorageKey(SERVER_ID, 'recentEmojis'), JSON.stringify(['🔥']));

    const { container } = renderBar();
    const reactions = quickReactionLabels(container);

    expect(reactions.slice(0, PINNED_REACTIONS.length)).toEqual([...PINNED_REACTIONS]);
    expect(reactions[PINNED_REACTIONS.length]).toBe('🔥');
  });

  it('does not reorder recent reactions when a toolbar quick reaction is clicked', async () => {
    const { container } = renderBar();
    const before = [...getRecentEmojis(SERVER_ID).quickReactions];

    (container.querySelector('[aria-label="React with ❤️"]') as HTMLButtonElement).click();
    await vi.waitFor(() => expect(mocks.actions.toggleReaction).toHaveBeenCalledOnce());

    expect([...getRecentEmojis(SERVER_ID).quickReactions]).toEqual(before);
  });
});
