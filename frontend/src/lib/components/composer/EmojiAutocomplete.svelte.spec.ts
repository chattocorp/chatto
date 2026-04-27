import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import EmojiAutocomplete from './EmojiAutocomplete.svelte';

function renderAutocomplete(props: {
  query: string;
  onSelect?: (emoji: string, name: string) => void;
  onClose?: () => void;
}) {
  return render(EmojiAutocomplete, {
    props: {
      query: props.query,
      onSelect: props.onSelect ?? (() => {}),
      onClose: props.onClose ?? (() => {})
    }
  });
}

function shortcodes(container: HTMLElement): string[] {
  // The snippet renders ":name:" in a text-sm span
  return Array.from(container.querySelectorAll('span'))
    .map((s) => s.textContent ?? '')
    .filter((t) => /^:[^:]+:$/.test(t.trim()))
    .map((t) => t.trim().slice(1, -1));
}

describe('EmojiAutocomplete', () => {
  describe('search', () => {
    it('renders nothing when no emoji matches', () => {
      const { container } = renderAutocomplete({ query: 'zzzznothing' });
      expect(container.querySelector('button')).toBeNull();
    });

    it('renders matching emojis as buttons', () => {
      const { container } = renderAutocomplete({ query: 'heart' });
      const codes = shortcodes(container);
      expect(codes.length).toBeGreaterThan(0);
      expect(codes).toContain('heart');
    });

    it('limits results to the top 10', () => {
      // 'a' is a very common substring → many matches
      const { container } = renderAutocomplete({ query: 'a' });
      const codes = shortcodes(container);
      expect(codes.length).toBeLessThanOrEqual(10);
    });

    it('ranks an exact-name match first', () => {
      const { container } = renderAutocomplete({ query: 'fire' });
      const codes = shortcodes(container);
      expect(codes[0]).toBe('fire');
    });
  });

  describe('keyboard forwarding', () => {
    it('Enter selects the highlighted item and forwards (emoji, name) to onSelect', () => {
      const onSelect = vi.fn();
      const { component } = renderAutocomplete({ query: 'fire', onSelect });
      component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Enter', cancelable: true }));
      // First arg is the emoji char, second is the shortcode name
      expect(onSelect).toHaveBeenCalledOnce();
      const [emoji, name] = onSelect.mock.calls[0];
      expect(typeof emoji).toBe('string');
      expect(name).toBe('fire');
    });

    it('Tab also selects (configured as a select key)', () => {
      const onSelect = vi.fn();
      const { component } = renderAutocomplete({ query: 'fire', onSelect });
      component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Tab', cancelable: true }));
      expect(onSelect).toHaveBeenCalledOnce();
      expect(onSelect.mock.calls[0][1]).toBe('fire');
    });

    it('Escape calls onClose', () => {
      const onClose = vi.fn();
      const { component } = renderAutocomplete({ query: 'fire', onClose });
      component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
      expect(onClose).toHaveBeenCalledOnce();
    });

    it('returns false when there are no items to navigate', () => {
      const { component } = renderAutocomplete({ query: 'zzzznothing' });
      const ev = new KeyboardEvent('keydown', { key: 'ArrowDown', cancelable: true });
      expect(component.handleKeyDown(ev)).toBe(false);
    });

    it('ArrowDown moves the highlight to the next item', () => {
      const { container, component } = renderAutocomplete({ query: 'sm' });
      const before = container.querySelector('.menu-item-active');
      component.handleKeyDown(new KeyboardEvent('keydown', { key: 'ArrowDown', cancelable: true }));
      flushSync();
      const after = container.querySelector('.menu-item-active');
      expect(after).not.toBeNull();
      expect(after).not.toBe(before);
    });
  });

  describe('selection via click', () => {
    it('clicking a result fires onSelect with that emoji + name', () => {
      const onSelect = vi.fn();
      const { container } = renderAutocomplete({ query: 'fire', onSelect });
      const buttons = container.querySelectorAll('button');
      (buttons[0] as HTMLButtonElement).click();
      expect(onSelect).toHaveBeenCalledOnce();
      const [, name] = onSelect.mock.calls[0];
      expect(name).toBe('fire');
    });
  });
});
