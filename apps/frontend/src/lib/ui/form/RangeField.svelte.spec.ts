import '../../../app.css';
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
        ariaValueText: '40 percent, moderate volume',
        describedBy: 'volume-help',
        oninput
      }
    });

    const field = container.querySelector('input') as HTMLInputElement;
    const label = container.querySelector('label') as HTMLLabelElement;

    expect(label.htmlFor).toBe('volume');
    expect(field.value).toBe('40');
    expect(field.getAttribute('aria-valuetext')).toBe('40 percent, moderate volume');
    expect(field.getAttribute('aria-describedby')).toBe('volume-help');

    field.value = '65';
    field.dispatchEvent(new Event('input', { bubbles: true }));
    expect(oninput).toHaveBeenCalledOnce();
  });

  it('fills its container and enlarges the prominent pointer target', () => {
    const { container } = render(RangeField, {
      props: {
        id: 'ui-contrast',
        label: 'Contrast',
        min: 20,
        max: 40,
        value: 30,
        displayValue: '30',
        prominent: true
      }
    });

    const input = container.querySelector('input') as HTMLInputElement;
    const field = input.closest('label') as HTMLLabelElement;
    expect(input.style.getPropertyValue('--range-progress')).toBe('50%');
    expect(input.getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
    expect(field.getBoundingClientRect().width).toBeCloseTo(
      field.parentElement!.getBoundingClientRect().width
    );
  });
});
