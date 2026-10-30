import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { emojiPreferences } from '$lib/state/emojiPreferences.svelte';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import EmojiAutocomplete from './EmojiAutocomplete.svelte';
import { page } from 'vitest/browser';

beforeEach(() => {
  emojiPreferences.tone = 0;
});
afterEach(() => {
  emojiPreferences.tone = 0;
});

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
  it('accepts a fuzzy shortcode without separators', () => {
    const onSelect = vi.fn();
    const { component } = renderAutocomplete({ query: 'headshakingvertically', onSelect });
    component.handleKeyDown(new KeyboardEvent('keydown', { key: ' ', cancelable: true }));
    expect(onSelect).toHaveBeenCalledWith('🙂‍↕️', 'head_shaking_vertically', ' ');
  });
  it('opens variants for the chosen result and selects a tone with the keyboard', () => {
    const onSelect = vi.fn();
    const { container, component } = renderAutocomplete({ query: 'Thum', onSelect });
    const variants = () =>
      [...container.querySelectorAll('button .text-xl')].map((glyph) => glyph.textContent?.trim());
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'ArrowDown', cancelable: true }));
    flushSync();
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Tab', cancelable: true }));
    flushSync();
    expect(onSelect).not.toHaveBeenCalled();
    expect(variants()).toEqual(['👎', '👎🏻', '👎🏼', '👎🏽', '👎🏾', '👎🏿']);
    for (let i = 0; i < 3; i++) {
      component.handleKeyDown(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
    }
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Enter', cancelable: true }));
    expect(onSelect).toHaveBeenCalledOnce();
    expect(onSelect).toHaveBeenCalledWith('👎🏽', '-1::skin-tone-4', 'Enter');
    expect(emojiPreferences.tone).toBe(4);
  });

  it('does not offer skin tones for smileys', () => {
    const onSelect = vi.fn();
    const { component } = renderAutocomplete({ query: 'smile', onSelect });
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Enter' }));
    expect(onSelect).toHaveBeenCalledExactlyOnceWith('😄', 'smile', 'Enter');
  });
  it('inserts a variant without moving focus or the editor cursor before the callback', async () => {
    const editor = document.createElement('input');
    document.body.append(editor);
    try {
      const onSelect = vi.fn();
      const { container, component } = renderAutocomplete({ query: 'wave', onSelect });
      editor.value = ':wave';
      editor.focus();
      editor.setSelectionRange(5, 5);
      component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Enter' }));
      flushSync();
      await page.elementLocator(container.querySelectorAll('button')[5]).click();
      expect(document.activeElement).toBe(editor);
      expect(editor.selectionStart).toBe(5);
      expect(onSelect).toHaveBeenCalledOnce();
      expect(onSelect).toHaveBeenCalledWith('👋🏿', 'wave::skin-tone-6', 'click');
    } finally {
      editor.remove();
    }
  });

  it('shows a short base label while selecting the preferred tone', () => {
    const onSelect = vi.fn();
    const { container, component } = renderAutocomplete({ query: 'wave', onSelect });
    emojiPreferences.tone = 5;
    flushSync();
    expect(container.textContent).toContain(':wave:');
    expect(container.textContent).not.toContain('skin-tone-');
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Enter', cancelable: true }));
    flushSync();
    expect(container.querySelector('.menu-item-active')?.textContent).toContain('👋🏾');
    expect(onSelect).not.toHaveBeenCalled();
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Tab', cancelable: true }));
    expect(onSelect).toHaveBeenCalledWith('👋🏾', 'wave::skin-tone-5', 'Tab');
  });
  it('Escape returns to the selected emoji without inserting or changing the saved tone', () => {
    const onSelect = vi.fn();
    const onClose = vi.fn();
    const { container, component } = renderAutocomplete({ query: 'Thum', onSelect, onClose });
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Enter' }));
    flushSync();
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Escape' }));
    flushSync();
    expect(container.querySelector('.menu-item-active')?.textContent).toContain(':thumbsdown:');
    expect(onSelect).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(emojiPreferences.tone).toBe(0);
  });
  it('shows familiar names for thumb aliases', () => {
    const { container } = renderAutocomplete({ query: 'Thum' });
    expect(container.textContent).toContain(':thumbsup:');
    expect(container.textContent).toContain(':thumbsdown:');
    expect(container.textContent).not.toContain(':+1:');
  });
  it('returns to search when the draft query changes during tone selection', async () => {
    const onSelect = vi.fn();
    const { container, component, rerender } = renderAutocomplete({ query: 'wave', onSelect });
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Enter' }));
    flushSync();
    await rerender({ query: 'fire', onSelect, onClose: () => {} });
    expect(shortcodes(container)[0]).toBe('fire');
    component.handleKeyDown(new KeyboardEvent('keydown', { key: 'Enter' }));
    expect(onSelect).toHaveBeenCalledWith('🔥', 'fire', 'Enter');
  });
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
