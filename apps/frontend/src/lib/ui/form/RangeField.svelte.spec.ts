import '../../../app.css';
import { cdp } from 'vitest/browser';
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

it.each([
  [true, 100, false, true],
  [true, 99.9, false, false],
  [true, 100, true, false],
  [false, 100, false, false]
])(
  'limits the rainbow to an enabled opt-in maximum (%s, %s, %s)',
  async (rainbow, value, disabled, active) => {
    const screen = render(RangeField, {
      id: 'rainbow',
      label: 'Your Voice',
      min: 0,
      max: 100,
      value,
      displayValue: String(value),
      rainbow,
      disabled
    });
    const input = screen.container.querySelector('input')!;
    expect(input.classList.contains('range-rainbow')).toBe(active);
    expect(getComputedStyle(input).animationName).toBe(active ? 'range-rainbow-flow' : 'none');
  }
);

it('keeps the rainbow static when reduced motion is requested', async () => {
  const session = cdp();
  await session.send('Emulation.setEmulatedMedia', {
    features: [{ name: 'prefers-reduced-motion', value: 'reduce' }]
  });
  try {
    const screen = render(RangeField, {
      id: 'quiet-rainbow',
      label: 'Your Voice',
      min: 0,
      max: 100,
      value: 100,
      displayValue: 'AWESOME',
      rainbow: true
    });
    const input = screen.container.querySelector('input')!;
    expect(input.classList.contains('range-rainbow')).toBe(true);
    expect(getComputedStyle(input).animationName).toBe('none');
    expect(getComputedStyle(input).getPropertyValue('--range-spectrum')).toContain(
      'linear-gradient'
    );
  } finally {
    await session.send('Emulation.setEmulatedMedia', { features: [] });
  }
});
