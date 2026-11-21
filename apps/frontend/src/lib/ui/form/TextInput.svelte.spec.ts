import '../../../app.css';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import TextInput from './TextInput.svelte';

describe('TextInput', () => {
  it('keeps the part of an edit that fits a UTF-8 byte limit', () => {
    const { container } = render(TextInput, {
      id: 'motd',
      label: 'Message of the Day',
      value: '💬',
      maxBytes: 6
    });
    const input = container.querySelector('input')!;

    input.value = '💬abc';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    expect(input.value).toBe('💬ab');

    input.value = '💬abc';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    expect(input.value).toBe('💬ab');
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
