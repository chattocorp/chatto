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
  [true, 80, false, 0, false],
  [true, 90, false, 0.5, false],
  [true, 100, false, 1, true],
  [true, 100, true, 0, false],
  [false, 100, false, 0, false]
])(
  'fades the rainbow over the final fifth (%s, %s, %s)',
  async (rainbow, value, disabled, opacity, dancing) => {
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
    const band = screen.container.querySelector<HTMLElement>('[data-rainbow-band]');
    expect(band !== null).toBe(opacity > 0);
    if (band) {
      expect(Number(band.style.opacity)).toBeCloseTo(opacity);
      const spectrum = band.firstElementChild!;
      expect(getComputedStyle(spectrum).animationName).toBe('rainbow-travel');
    }
    expect(screen.container.querySelector('.awesome-text') !== null).toBe(dancing);
  }
);

it('animates the actual painted rainbow element rather than the native input', () => {
  const screen = render(RangeField, {
    id: 'moving-rainbow',
    label: 'Your Voice',
    min: 0,
    max: 100,
    value: 100,
    displayValue: 'AWESOME',
    rainbow: true
  });
  const spectrum = screen.container.querySelector('.rainbow-spectrum')!;
  const animation = spectrum.getAnimations()[0];
  animation.pause();
  animation.currentTime = 0;
  const before = getComputedStyle(spectrum).transform;
  animation.currentTime = 1000;
  expect(getComputedStyle(spectrum).transform).not.toBe(before);
  expect(getComputedStyle(spectrum).backgroundImage).toContain('gradient');
  expect(getComputedStyle(screen.container.querySelector('input')!).animationName).toBe('none');
  const text = screen.container.querySelector('.awesome-text')!;
  expect(text.getAnimations()).toHaveLength(2);
  expect(getComputedStyle(text).color).toBe('rgba(0, 0, 0, 0)');
  expect(getComputedStyle(text).backgroundClip).toBe('text');
});

it('keeps the rainbow and text static when reduced motion is requested', async () => {
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
    const spectrum = screen.container.querySelector('.rainbow-spectrum')!;
    const text = screen.container.querySelector('.awesome-text')!;
    expect(getComputedStyle(spectrum).animationName).toBe('none');
    expect(getComputedStyle(text).animationName).toBe('none');
    expect(getComputedStyle(spectrum).backgroundImage).toContain('gradient');
  } finally {
    await session.send('Emulation.setEmulatedMedia', { features: [] });
  }
});
