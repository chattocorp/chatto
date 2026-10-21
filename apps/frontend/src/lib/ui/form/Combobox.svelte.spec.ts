import '../../../app.css';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { flushSync } from 'svelte';
import Combobox from './Combobox.svelte';

const items = [
  { value: 'login', label: 'LoginSucceededEvent' },
  { value: 'join', label: 'UserJoinedRoomEvent' }
];

function input(container: Element): HTMLInputElement {
  return container.querySelector('input') as HTMLInputElement;
}

describe('Combobox', () => {
  it('keeps freeform text as the value and can clear it', async () => {
    const ontextchange = vi.fn();
    const { container } = render(Combobox<(typeof items)[number]>, {
      props: {
        id: 'event-type',
        label: 'Event type',
        items,
        getValue: (item) => item.value,
        getLabel: (item) => item.label,
        ontextchange
      }
    });

    const field = input(container);
    field.value = 'system:bootstrap';
    field.dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();

    expect(ontextchange).toHaveBeenCalledWith('system:bootstrap');
    expect(field.value).toBe('system:bootstrap');

    const clear = container.querySelector('button[aria-label="Clear"]') as HTMLButtonElement;
    clear.click();
    flushSync();

    expect(field.value).toBe('');
    expect(ontextchange).toHaveBeenLastCalledWith('');
  });

  it('selects an option with the keyboard', async () => {
    const onselect = vi.fn();
    const { container } = render(Combobox<(typeof items)[number]>, {
      props: {
        id: 'event-type',
        label: 'Event type',
        items,
        getValue: (item) => item.value,
        getLabel: (item) => item.label,
        onselect
      }
    });

    const field = input(container);
    field.dispatchEvent(new FocusEvent('focus', { bubbles: true }));
    field.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    field.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
    flushSync();

    expect(onselect).toHaveBeenCalledWith(items[1]);
    expect(field.value).toBe('UserJoinedRoomEvent');
  });

  it('selects multiple values without duplicates and removes pills with the keyboard', async () => {
    const onselect = vi.fn();
    const { container, getByRole } = render(Combobox<(typeof items)[number]>, {
      props: {
        id: 'multiple-events',
        label: 'Events',
        items,
        multiple: true,
        getValue: (item) => item.value,
        getLabel: (item) => item.label,
        onselect
      }
    });
    const field = input(container);
    field.focus();
    await getByRole('option', { name: items[0].label }).click();
    expect(field.value).toBe('');
    await expect.element(getByRole('button', { name: `Remove ${items[0].label}` })).toBeVisible();
    field.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }));
    flushSync();
    await expect.element(getByRole('option', { name: items[0].label })).not.toBeInTheDocument();
    const enter = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true });
    field.dispatchEvent(enter);
    flushSync();
    expect(enter.defaultPrevented).toBe(true);
    expect(onselect).toHaveBeenCalledTimes(2);
    expect(field.value).toBe('');
    expect(container.querySelectorAll('[data-testid="combobox-selection"]')).toHaveLength(2);
    field.value = 'query';
    field.dispatchEvent(new Event('input', { bubbles: true }));
    field.dispatchEvent(new KeyboardEvent('keydown', { key: 'Backspace', bubbles: true }));
    flushSync();
    expect(container.querySelectorAll('[data-testid="combobox-selection"]')).toHaveLength(2);
    field.value = '';
    field.dispatchEvent(new Event('input', { bubbles: true }));
    field.dispatchEvent(new KeyboardEvent('keydown', { key: 'Backspace', bubbles: true }));
    flushSync();
    expect(container.querySelectorAll('[data-testid="combobox-selection"]')).toHaveLength(1);
    await getByRole('button', { name: `Remove ${items[0].label}` }).click();
    expect(document.activeElement).toBe(field);
    expect(container.querySelectorAll('[data-testid="combobox-selection"]')).toHaveLength(0);
  });

  it('wraps and truncates long selected labels inside a narrow input', async () => {
    const choices = [
      { value: 'long', label: 'A very long member name that must not widen the dialog' },
      ...items
    ];
    const { container } = render(Combobox<(typeof items)[number]>, {
      props: {
        id: 'narrow-multiple',
        label: 'Members',
        items: choices,
        multiple: true,
        selectedItems: choices,
        getValue: (item) => item.value,
        getLabel: (item) => item.label
      }
    });
    container.style.width = '240px';
    const frame = input(container).parentElement!;
    await vi.waitFor(() => expect(frame.scrollWidth).toBeLessThanOrEqual(frame.clientWidth));
    expect(container.querySelectorAll('[data-testid="combobox-selection"]')).toHaveLength(3);
  });

  it('scrolls a long list inside the section so the menu frame stays whole', async () => {
    const many = Array.from({ length: 30 }, (_, i) => ({ value: `v${i}`, label: `Option ${i}` }));
    const { container } = render(Combobox<(typeof many)[number]>, {
      props: {
        id: 'many',
        label: 'Many',
        items: many,
        getValue: (item) => item.value,
        getLabel: (item) => item.label
      }
    });

    input(container).dispatchEvent(new FocusEvent('focus', { bubbles: true }));
    flushSync();

    const frame = container.querySelector('#many-listbox') as HTMLElement;
    const section = frame.querySelector('.menu-section') as HTMLElement;
    await vi.waitFor(() => expect(section.scrollHeight).toBeGreaterThan(section.clientHeight));
    expect(frame.scrollHeight).toBe(frame.clientHeight);
  });
});
