import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { createRawSnippet, flushSync } from 'svelte';
import AutocompletePopup from './AutocompletePopup.svelte';

function press(component: { handleKeyDown: (e: KeyboardEvent) => boolean }, k: string) {
  const ev = new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true });
  const handled = component.handleKeyDown(ev);
  flushSync();
  return { ev, handled };
}

type Item = { id: string; label: string };

function items(...labels: string[]): Item[] {
  return labels.map((label, i) => ({ id: `id-${i}-${label}`, label }));
}

function renderPopup(props: {
  items: Item[];
  selectKeys?: string[];
  onSelect?: (item: Item, key: string) => void;
  onClose?: () => void;
}) {
  const item = createRawSnippet<[{ item: Item; selected: boolean }]>((entry) => ({
    render: () => `<span class="entry">${entry().item.label}</span>`
  }));
  return render(AutocompletePopup<Item>, {
    props: {
      items: props.items,
      getKey: (i) => i.id,
      selectKeys: props.selectKeys,
      onSelect: props.onSelect ?? (() => {}),
      onClose: props.onClose ?? (() => {}),
      testid: 'popup',
      item
    }
  });
}

function activeLabel(container: HTMLElement): string | null {
  const active = container.querySelector('.menu-item-active .entry');
  return active?.textContent ?? null;
}

describe('AutocompletePopup', () => {
  describe('rendering', () => {
    it('renders nothing when items list is empty', () => {
      const { container } = renderPopup({ items: [] });
      expect(container.querySelector('[data-testid="popup"]')).toBeNull();
    });

    it('renders one button per item', () => {
      const { container } = renderPopup({ items: items('alice', 'bob', 'carol') });
      const buttons = container.querySelectorAll('button');
      expect(buttons.length).toBe(3);
    });

    it('marks the first item active by default', () => {
      const { container } = renderPopup({ items: items('alice', 'bob') });
      expect(activeLabel(container)).toBe('alice');
    });
  });

  describe('keyboard navigation', () => {
    it('ArrowDown advances to the next item and consumes the event', () => {
      const { container, component } = renderPopup({ items: items('a', 'b', 'c') });
      const { ev, handled } = press(component, 'ArrowDown');
      expect(handled).toBe(true);
      expect(ev.defaultPrevented).toBe(true);
      expect(activeLabel(container)).toBe('b');
    });

    it('ArrowDown wraps from the last item to the first', () => {
      const { container, component } = renderPopup({ items: items('a', 'b') });
      press(component, 'ArrowDown'); // -> b
      press(component, 'ArrowDown'); // wrap -> a
      expect(activeLabel(container)).toBe('a');
    });

    it('ArrowUp moves to the previous item', () => {
      const { container, component } = renderPopup({ items: items('a', 'b', 'c') });
      press(component, 'ArrowDown'); // -> b
      press(component, 'ArrowUp'); // -> a
      expect(activeLabel(container)).toBe('a');
    });

    it('ArrowUp wraps from the first item to the last', () => {
      const { container, component } = renderPopup({ items: items('a', 'b', 'c') });
      press(component, 'ArrowUp');
      expect(activeLabel(container)).toBe('c');
    });

    it('Escape calls onClose and consumes the event', () => {
      const onClose = vi.fn();
      const { component } = renderPopup({ items: items('a'), onClose });
      const { ev, handled } = press(component, 'Escape');
      expect(handled).toBe(true);
      expect(ev.defaultPrevented).toBe(true);
      expect(onClose).toHaveBeenCalledOnce();
    });

    it('returns false for unhandled keys without preventing default', () => {
      const { component } = renderPopup({ items: items('a') });
      const { ev, handled } = press(component, 'a');
      expect(handled).toBe(false);
      expect(ev.defaultPrevented).toBe(false);
    });

    it('returns false on any key when items list is empty', () => {
      const onClose = vi.fn();
      const { component } = renderPopup({ items: [], onClose });
      expect(press(component, 'ArrowDown').handled).toBe(false);
      expect(press(component, 'Escape').handled).toBe(false);
      expect(onClose).not.toHaveBeenCalled();
    });
  });

  describe('selection', () => {
    it('Enter selects the current item by default', () => {
      const onSelect = vi.fn();
      const { component } = renderPopup({ items: items('a', 'b'), onSelect });
      press(component, 'ArrowDown'); // -> b
      const { ev } = press(component, 'Enter');
      expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ label: 'b' }), 'Enter');
      expect(ev.defaultPrevented).toBe(true);
    });

    it('Tab selects by default and forwards the key used', () => {
      const onSelect = vi.fn();
      const { component } = renderPopup({ items: items('a'), onSelect });
      press(component, 'Tab');
      expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ label: 'a' }), 'Tab');
    });

    it('respects custom selectKeys: Tab is ignored when only Enter is configured', () => {
      const onSelect = vi.fn();
      const { component } = renderPopup({
        items: items('a'),
        selectKeys: ['Enter'],
        onSelect
      });
      const { ev, handled } = press(component, 'Tab');
      expect(handled).toBe(false);
      expect(ev.defaultPrevented).toBe(false);
      expect(onSelect).not.toHaveBeenCalled();
    });

    it('clicking an item fires onSelect with the literal "click" key', async () => {
      const onSelect = vi.fn();
      const { container } = renderPopup({ items: items('a', 'b'), onSelect });
      const buttons = container.querySelectorAll('button');
      (buttons[1] as HTMLButtonElement).click();
      expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ label: 'b' }), 'click');
    });
  });

  describe('scroll into view', () => {
    // The popup's max-h-80 overflow lives in app.css (not loaded in component
    // tests), so assert the behaviour directly: the selected item's element
    // gets scrollIntoView called on it whenever the selection moves.
    it('scrolls the newly selected item into view on keyboard navigation', () => {
      const spy = vi.spyOn(HTMLElement.prototype, 'scrollIntoView');
      try {
        const { container, component } = renderPopup({ items: items('a', 'b', 'c') });
        spy.mockClear(); // ignore the mount-time call on the initial selection

        press(component, 'ArrowDown'); // -> b

        const active = container.querySelector<HTMLElement>('.menu-item-active');
        expect(activeLabel(container)).toBe('b');
        expect(spy).toHaveBeenCalledTimes(1);
        expect(spy.mock.instances[0]).toBe(active);
        expect(spy).toHaveBeenCalledWith({ block: 'nearest' });
      } finally {
        spy.mockRestore();
      }
    });
  });

  describe('items reactivity', () => {
    it('resets the active index to 0 when items change', async () => {
      const { container, rerender, component } = renderPopup({ items: items('a', 'b', 'c') });
      press(component, 'ArrowDown');
      press(component, 'ArrowDown');
      expect(activeLabel(container)).toBe('c');

      const item = createRawSnippet<[{ item: Item; selected: boolean }]>((entry) => ({
        render: () => `<span class="entry">${entry().item.label}</span>`
      }));
      await rerender({
        items: items('x', 'y'),
        getKey: (i: Item) => i.id,
        onSelect: () => {},
        onClose: () => {},
        testid: 'popup',
        item
      });
      flushSync();
      expect(activeLabel(container)).toBe('x');
    });
  });
});
