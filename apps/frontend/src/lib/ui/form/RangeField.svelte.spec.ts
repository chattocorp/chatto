import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import RangeField from './RangeField.svelte';

describe('RangeField', () => {
  it('associates its label and forwards input changes', () => {
    const oninput = vi.fn();
    const { container } = render(RangeField, {
      props: {
        id: 'volume',
        label: 'Notification volume',
        min: 0,
        max: 100,
        value: 40,
        displayValue: '40%',
        oninput
      }
    });

    const field = container.querySelector('input') as HTMLInputElement;
    const label = container.querySelector('label') as HTMLLabelElement;

    expect(label.htmlFor).toBe('volume');
    expect(field.value).toBe('40');

    field.value = '65';
    field.dispatchEvent(new Event('input', { bubbles: true }));
    expect(oninput).toHaveBeenCalledOnce();
  });
});
