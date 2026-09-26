import '../../../app.css';
import { describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
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

  it('fills its container and extends the pointer target beyond the track', () => {
    const { container } = render(RangeField, {
      props: {
        id: 'ui-contrast',
        label: 'Contrast',
        min: 20,
        max: 40,
        value: 30,
        displayValue: '30'
      }
    });

    const input = container.querySelector('input') as HTMLInputElement;
    const field = input.closest('label') as HTMLLabelElement;
    const track = container.querySelector('.range-track') as HTMLElement;
    expect(track.style.getPropertyValue('--range-progress')).toBe('0.5');
    expect(input.getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
    expect(field.getBoundingClientRect().width).toBeCloseTo(
      field.parentElement!.getBoundingClientRect().width
    );
  });

  it('draws the grip where the native thumb receives the pointer', () => {
    const { container } = render(RangeField, {
      props: {
        id: 'ui-depth',
        label: 'Depth',
        min: 0,
        max: 100,
        step: 10,
        value: 30,
        displayValue: '30%',
        ticks: [0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100]
      }
    });

    const input = container.querySelector('input') as HTMLInputElement;
    const grip = container.querySelector('.range-grip') as HTMLElement;
    const inputBox = input.getBoundingClientRect();
    const gripBox = grip.getBoundingClientRect();
    // The native thumb centre travels between half a grip width from each end.
    const thumbCentre = inputBox.left + gripBox.width / 2 + (inputBox.width - gripBox.width) * 0.3;
    expect(gripBox.left + gripBox.width / 2).toBeCloseTo(thumbCentre, 0);
    expect(container.querySelectorAll('.range-tick')).toHaveLength(11);
    expect(container.querySelectorAll('.range-tick-filled')).toHaveLength(4);
  });

  it('highlights the grip while the pointer is over the track', async () => {
    const { container } = render(RangeField, {
      props: { id: 'hover', label: 'Volume', min: 0, max: 100, value: 40, displayValue: '40%' }
    });
    const track = container.querySelector('.range-track') as HTMLElement;
    const grip = container.querySelector('.range-grip') as HTMLElement;
    const halo = () => getComputedStyle(track).getPropertyValue('--range-active').trim();

    expect(halo()).toBe('');
    await userEvent.hover(track);
    expect(halo()).toBe('1');
    const shadow = getComputedStyle(grip).boxShadow;
    expect(shadow).toMatch(/0px 0px 0px 3px/);
    // The halo must not replace the bevel's inset lighting.
    expect(shadow).toMatch(/inset/);
    await userEvent.unhover(track);
    expect(halo()).toBe('');
  });
});
