import '../../app.css';
import { cdp, page, userEvent } from 'vitest/browser';
import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';
import { flushSync, tick } from 'svelte';
import { EMOJI_BY_CATEGORY } from '$lib/emoji';
import { __resetRecentEmojisForTests } from '$lib/state/recentEmojis.svelte';
import { render } from 'vitest-browser-svelte';
import EmojiPicker from './EmojiPicker.svelte';
import { emojiPreferences, EmojiPreferences } from '$lib/state/emojiPreferences.svelte';

const TEST_SERVER_ID = 'test-server';

function renderPicker(
  props: { onSelect?: (e: string) => void; onClose?: () => void; serverId?: string } = {}
) {
  return render(EmojiPicker, {
    props: {
      serverId: props.serverId ?? TEST_SERVER_ID,
      onSelect: props.onSelect ?? (() => {}),
      onClose: props.onClose ?? (() => {})
    }
  });
}

beforeEach(() => {
  emojiPreferences.tone = 0;
  localStorage.clear();
  __resetRecentEmojisForTests();
});

function searchInput(container: HTMLElement): HTMLInputElement {
  const input = container.querySelector('input[type="text"]');
  if (!input) throw new Error('search input not found');
  return input as HTMLInputElement;
}

async function type(input: HTMLInputElement, value: string) {
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  // searchResults is a $derived, so values propagate within this microtask
  await tick();
}

describe('EmojiPicker', () => {
  it('fits all six touch targets inside a narrow sheet', async () => {
    await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: true });
    const { container } = renderPicker();
    container.style.width = '286px';
    (container.querySelector('button[title="wave"]') as HTMLButtonElement).click();
    flushSync();
    const picker = container.firstElementChild as HTMLElement;
    const bounds = picker.getBoundingClientRect();
    for (const button of container.querySelectorAll('[data-emoji-tone]')) {
      const rect = button.getBoundingClientRect();
      expect(rect.width).toBeGreaterThanOrEqual(44);
      expect(rect.left).toBeGreaterThanOrEqual(bounds.left);
      expect(rect.right).toBeLessThanOrEqual(bounds.right);
    }
    expect(picker.scrollWidth).toBeLessThanOrEqual(picker.clientWidth);
  });
  it('offers the selected emoji variants, saves the tone, and keeps exact recents', async () => {
    const onSelect = vi.fn();
    localStorage.setItem(`chatto:i:${TEST_SERVER_ID}:recentEmojis`, JSON.stringify(['👋🏻']));
    const { container } = renderPicker({ onSelect });
    expect(container.querySelector('[data-emoji-tone]')).toBeNull();
    const recent = container.querySelector('.grid button') as HTMLButtonElement;
    expect(recent.textContent?.trim()).toBe('👋🏻');
    recent.click();
    expect(onSelect).toHaveBeenLastCalledWith('👋🏻');
    onSelect.mockClear();
    await type(searchInput(container), 'wave');
    (container.querySelector('button[title="wave"]') as HTMLButtonElement).click();
    flushSync();
    expect(onSelect).not.toHaveBeenCalled();
    expect(
      [...container.querySelectorAll('[data-emoji-tone] .text-xl')].map((el) => el.textContent)
    ).toEqual(['👋', '👋🏻', '👋🏼', '👋🏽', '👋🏾', '👋🏿']);
    (container.querySelector('[data-emoji-tone="4"]') as HTMLButtonElement).click();
    expect(onSelect).toHaveBeenCalledExactlyOnceWith('👋🏽');
    expect(new EmojiPreferences().tone).toBe(4);
  });
  it.each(['Tab', 'Enter'])(
    'preselects the saved tone and supports arrows and %s confirmation',
    async (key) => {
      emojiPreferences.tone = 4;
      const onSelect = vi.fn();
      const { container } = renderPicker({ onSelect });
      await type(searchInput(container), 'thumb');
      (container.querySelector('button[title="+1::skin-tone-4"]') as HTMLButtonElement).click();
      flushSync();
      expect(document.activeElement).toBe(container.querySelector('[data-emoji-tone="4"]'));
      await userEvent.keyboard('{ArrowDown}');
      expect(document.activeElement).toBe(container.querySelector('[data-emoji-tone="5"]'));
      await userEvent.keyboard(`{${key}}`);
      expect(onSelect).toHaveBeenCalledExactlyOnceWith('👍🏾');
    }
  );
  it.each(['Escape', 'Back'])(
    '%s restores the previous search and source button without saving a tone',
    async (method) => {
      const onClose = vi.fn();
      const onSelect = vi.fn();
      const { container } = renderPicker({ onSelect, onClose });
      await type(searchInput(container), 'thumb');
      const source = container.querySelector('button[title="-1"]') as HTMLButtonElement;
      source.click();
      flushSync();
      await userEvent.keyboard('{ArrowDown}');
      if (method === 'Escape') await userEvent.keyboard('{Escape}');
      else await page.getByRole('button', { name: 'Back', exact: true }).click();
      expect(searchInput(container).value).toBe('thumb');
      expect(document.activeElement).toBe(source);
      expect(container.querySelector('[data-emoji-tone]')).toBeNull();
      expect(onClose).not.toHaveBeenCalled();
      expect(onSelect).not.toHaveBeenCalled();
      expect(emojiPreferences.tone).toBe(0);
    }
  );
  describe('default state (no search)', () => {
    it('renders the search input', () => {
      const { container } = renderPicker();
      expect(searchInput(container)).toBeTruthy();
    });

    it('renders all category sections', () => {
      const { container } = renderPicker();
      const headings = Array.from(container.querySelectorAll('div'))
        .map((d) => d.textContent?.trim())
        .filter(Boolean);
      for (const cat of EMOJI_BY_CATEGORY) {
        expect(headings).toContain(cat.name);
      }
    });

    it('does not show "No emojis found" when not searching', () => {
      const { container } = renderPicker();
      expect(container.textContent).not.toContain('No emojis found');
    });
  });

  describe('search', () => {
    it('shows search results matching the query', async () => {
      const { container } = renderPicker();
      await type(searchInput(container), 'smile');

      // Hides categories while searching
      const headings = Array.from(container.querySelectorAll('div'))
        .map((d) => d.textContent?.trim())
        .filter(Boolean);
      for (const cat of EMOJI_BY_CATEGORY) {
        expect(headings).not.toContain(cat.name);
      }
      // Shows at least one emoji button (the result grid)
      const resultButtons = container.querySelectorAll('button');
      expect(resultButtons.length).toBeGreaterThan(0);
    });

    it('shows "No emojis found" for queries with no matches', async () => {
      const { container } = renderPicker();
      await type(searchInput(container), 'zzzzzznotanemoji');
      expect(container.textContent).toContain('No emojis found');
    });

    it('treats whitespace-only queries as empty', async () => {
      const { container } = renderPicker();
      await type(searchInput(container), '   ');
      expect(container.textContent).not.toContain('No emojis found');
      // Categories should still be visible
      const headings = Array.from(container.querySelectorAll('div'))
        .map((d) => d.textContent?.trim())
        .filter(Boolean);
      expect(headings).toContain(EMOJI_BY_CATEGORY[0].name);
    });
  });

  describe('selection', () => {
    it('clicking a category emoji calls onSelect with that emoji', () => {
      const onSelect = vi.fn();
      const { container } = renderPicker({ onSelect });
      const firstButton = container.querySelector('button') as HTMLButtonElement;
      const emojiText = firstButton.textContent?.trim() ?? '';
      firstButton.click();
      expect(onSelect).toHaveBeenCalledWith(emojiText);
    });

    it('clicking a search result calls onSelect with that emoji', async () => {
      const onSelect = vi.fn();
      const { container } = renderPicker({ onSelect });
      await type(searchInput(container), 'smile');
      const firstResult = container.querySelector('.grid button') as HTMLButtonElement;
      const emojiText = firstResult.textContent?.trim() ?? '';
      firstResult.click();
      expect(onSelect).toHaveBeenCalledWith(emojiText);
    });
  });

  describe('Recently Used section', () => {
    it('is not rendered when there are no recents', () => {
      const { container } = renderPicker();
      expect(container.textContent).not.toContain('Recently Used');
    });

    it('appears after selecting an emoji and shows the selection first', async () => {
      const { container } = renderPicker();
      const firstButton = container.querySelector('button') as HTMLButtonElement;
      const emojiText = firstButton.textContent?.trim() ?? '';
      firstButton.click();
      flushSync();
      await tick();
      expect(container.textContent).toContain('Recently Used');
      // The first button in the recent grid should be the just-selected emoji
      const recentGrid = container.querySelector('.grid') as HTMLElement;
      expect(recentGrid.querySelector('button')?.textContent?.trim()).toBe(emojiText);
    });

    it('hydrates from localStorage on mount', () => {
      localStorage.setItem(`chatto:i:${TEST_SERVER_ID}:recentEmojis`, JSON.stringify(['🚀', '🔥']));
      const { container } = renderPicker();
      expect(container.textContent).toContain('Recently Used');
      const firstGrid = container.querySelector('.grid') as HTMLElement;
      const recentButtons = Array.from(firstGrid.querySelectorAll('button'));
      expect(recentButtons[0]?.textContent?.trim()).toBe('🚀');
      expect(recentButtons[1]?.textContent?.trim()).toBe('🔥');
    });

    it('is scoped per server', () => {
      localStorage.setItem(`chatto:i:server-a:recentEmojis`, JSON.stringify(['🚀']));
      const { container } = renderPicker({ serverId: 'server-b' });
      expect(container.textContent).not.toContain('Recently Used');
    });
  });

  describe('Escape semantics', () => {
    it('Escape with a non-empty query clears the query (does not close)', async () => {
      const onClose = vi.fn();
      const { container } = renderPicker({ onClose });
      const input = searchInput(container);
      await type(input, 'smile');
      expect(input.value).toBe('smile');

      const wrapper = input.closest('div[onkeydown], .flex') as HTMLElement;
      // The handler is on the outer flex container; bubble from the input.
      input.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
      );
      flushSync();
      await tick();

      expect(onClose).not.toHaveBeenCalled();
      expect(input.value).toBe('');
      void wrapper; // silence unused
    });

    it('Escape with empty query calls onClose', async () => {
      const onClose = vi.fn();
      const { container } = renderPicker({ onClose });
      const input = searchInput(container);
      input.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
      );
      expect(onClose).toHaveBeenCalledOnce();
    });
  });
});

afterEach(async () => {
  emojiPreferences.tone = 0;
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
  await page.viewport(1280, 720);
});

it.each([false, true])('sizes emoji targets by touch capability, with touch=%s', async (touch) => {
  await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
  const { container } = render(EmojiPicker, {
    serverId: 'input-presentation-test',
    onSelect: () => {},
    onClose: () => {}
  });
  const picker = container.firstElementChild as HTMLElement;
  for (const width of [320, 390, 1024]) {
    await page.viewport(width, 800);
    const button = container.querySelector('button')!;
    expect(button.getBoundingClientRect().height).toBeGreaterThanOrEqual(touch ? 44 : 32);
    expect(button.getBoundingClientRect().width).toBeGreaterThanOrEqual(touch ? 44 : 30);
    expect(picker.scrollWidth).toBeLessThanOrEqual(picker.clientWidth);
    expect(picker.getBoundingClientRect().width).toBeLessThanOrEqual(width);
  }
});
