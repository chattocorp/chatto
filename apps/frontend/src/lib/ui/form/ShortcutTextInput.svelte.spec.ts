import '../../../app.css';
import { expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import ShortcutTextInput from './ShortcutTextInput.svelte';

it('focuses and selects its value when its Command/Control shortcut is pressed', () => {
  const { container } = render(ShortcutTextInput, {
    props: {
      id: 'permission-filter',
      label: 'Filter permissions',
      shortcutKey: '/',
      value: 'room'
    }
  });
  const input = container.querySelector<HTMLInputElement>('#permission-filter')!;
  const shortcut = new KeyboardEvent('keydown', {
    key: '/',
    metaKey: true,
    bubbles: true,
    cancelable: true
  });

  window.dispatchEvent(shortcut);

  expect(input.placeholder).toMatch(/(⌘\/|Ctrl-\/)/);
  expect(shortcut.defaultPrevented).toBe(true);
  expect(document.activeElement).toBe(input);
  expect(input.selectionStart).toBe(0);
  expect(input.selectionEnd).toBe(4);
});

it('leaves disabled inputs alone', () => {
  const { container } = render(ShortcutTextInput, {
    props: {
      id: 'disabled-filter',
      label: 'Filter permissions',
      shortcutKey: '/',
      value: 'room',
      disabled: true
    }
  });
  const input = container.querySelector<HTMLInputElement>('#disabled-filter')!;
  const shortcut = new KeyboardEvent('keydown', {
    key: '/',
    ctrlKey: true,
    bubbles: true,
    cancelable: true
  });

  window.dispatchEvent(shortcut);

  expect(shortcut.defaultPrevented).toBe(false);
  expect(document.activeElement).not.toBe(input);
});
