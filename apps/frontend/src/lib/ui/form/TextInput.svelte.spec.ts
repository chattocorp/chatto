import '../../../app.css';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import TextInput from './TextInput.svelte';

describe('TextInput', () => {
  it('mentions the length limit only near it', async () => {
    const rendered = render(TextInput, {
      id: 'motd',
      label: 'MOTD',
      description: 'Shown in the chat header.',
      value: 'a'.repeat(89),
      maxBytes: 100
    });
    expect(rendered.container.textContent).toContain('Shown in the chat header.');
    expect(rendered.container.textContent).not.toContain('maximum length');

    await rendered.rerender({ value: 'a'.repeat(90) });
    expect(rendered.container.textContent).toContain(
      'Shown in the chat header. Almost at the maximum length.'
    );

    await rendered.rerender({ value: 'a'.repeat(100) });
    expect(rendered.container.textContent).toContain('Maximum length reached.');
  });

  it('communicates and enforces a UTF-8 byte limit', () => {
    const { container } = render(TextInput, {
      id: 'motd',
      label: 'Message of the Day',
      value: '💬',
      maxBytes: 5
    });
    const input = container.querySelector('input')!;
    expect(container.textContent).not.toContain('maximum length');

    const blocked = new InputEvent('beforeinput', {
      bubbles: true,
      cancelable: true,
      data: 'ab',
      inputType: 'insertText'
    });
    input.setSelectionRange(input.value.length, input.value.length);
    input.dispatchEvent(blocked);
    expect(blocked.defaultPrevented).toBe(true);

    input.value = '💬a';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    expect(input.value).toBe('💬a');

    input.value = '💬ab';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    expect(input.value).toBe('💬a');
  });

  it('renders a labelled date and time field with a form name', async () => {
    const screen = render(TextInput, {
      props: {
        id: 'expires-at',
        label: 'Clear status at',
        type: 'datetime-local',
        name: 'expires-at',
        value: '2026-10-01T17:30'
      }
    });
    const field = screen.getByLabelText('Clear status at');

    await expect.element(field).toHaveAttribute('type', 'datetime-local');
    await expect.element(field).toHaveAttribute('name', 'expires-at');
    await expect.element(field).toHaveValue('2026-10-01T17:30');
  });
});
