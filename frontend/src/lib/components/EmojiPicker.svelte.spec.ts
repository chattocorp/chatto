import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync, tick } from 'svelte';
import EmojiPicker from './EmojiPicker.svelte';
import { EMOJI_BY_CATEGORY } from '$lib/emoji';

function renderPicker(props: { onSelect?: (e: string) => void; onClose?: () => void } = {}) {
  return render(EmojiPicker, {
    props: {
      onSelect: props.onSelect ?? (() => {}),
      onClose: props.onClose ?? (() => {})
    }
  });
}

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
