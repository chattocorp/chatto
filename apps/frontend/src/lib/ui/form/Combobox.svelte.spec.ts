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
});
