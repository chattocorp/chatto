import { describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import SegmentedControl from './SegmentedControl.svelte';
import '../../app.css';

describe('SegmentedControl', () => {
  const options = [
    { value: 'relevance', label: 'Most relevant' },
    { value: 'newest', label: 'Newest' }
  ] as const;

  it('renders one native radio group and reports selection changes', async () => {
    const onchange = vi.fn();
    const { container } = render(SegmentedControl, {
      props: { label: 'Sort messages', options, value: 'relevance', onchange }
    });
    const radios = [...container.querySelectorAll<HTMLInputElement>('input[type="radio"]')];

    expect(container.querySelector('[role="radiogroup"]')?.getAttribute('aria-label')).toBe(
      'Sort messages'
    );
    expect(radios).toHaveLength(2);
    expect(radios[0]?.name).toBe(radios[1]?.name);
    expect(radios[0]?.checked).toBe(true);

    await userEvent.click(container.querySelector('label:last-child')!);

    expect(onchange).toHaveBeenCalledWith('newest');
  });

  it('disables every option with the group', () => {
    const { container } = render(SegmentedControl, {
      props: {
        label: 'Sort messages',
        options,
        value: 'newest',
        onchange: vi.fn(),
        disabled: true
      }
    });

    expect(
      [...container.querySelectorAll<HTMLInputElement>('input[type="radio"]')].every(
        (radio) => radio.disabled
      )
    ).toBe(true);
  });

  it('moves the selected indicator between unequal options', async () => {
    const onchange = vi.fn();
    const rendered = render(SegmentedControl, {
      props: { label: 'Sort messages', options, value: 'relevance', onchange }
    });
    const fieldset = rendered.container.querySelector<HTMLElement>('[role="radiogroup"]')!;

    await vi.waitFor(() => expect(fieldset.dataset.indicatorReady).toBe('true'));
    const initialX = fieldset.style.getPropertyValue('--segmented-indicator-x');
    const initialWidth = fieldset.style.getPropertyValue('--segmented-indicator-width');

    await rendered.rerender({ label: 'Sort messages', options, value: 'newest', onchange });

    await vi.waitFor(() => {
      expect(fieldset.style.getPropertyValue('--segmented-indicator-x')).not.toBe(initialX);
    });
    expect(fieldset.style.getPropertyValue('--segmented-indicator-width')).not.toBe(initialWidth);
  });

  it('centres the option faces and selected indicator vertically in the track', async () => {
    const { container } = render(SegmentedControl, {
      props: { label: 'Sort messages', options, value: 'relevance', onchange: vi.fn() }
    });
    const group = container.querySelector<HTMLElement>('[role="radiogroup"]')!;
    await vi.waitFor(() => expect(group.dataset.indicatorReady).toBe('true'));
    const track = group.getBoundingClientRect();
    const faces = [
      ...group.querySelectorAll('label > span'),
      group.querySelector('.segmented-selection')!
    ];
    for (const face of faces) {
      const rect = face.getBoundingClientRect();
      expect(rect.top - track.top).toBeCloseTo(track.bottom - rect.bottom);
      expect(rect.height).toBeGreaterThan(track.height / 2);
    }
  });

  it('keeps native radio arrow-key navigation', async () => {
    const onchange = vi.fn();
    const { container } = render(SegmentedControl, {
      props: { label: 'Sort messages', options, value: 'relevance', onchange }
    });
    container.querySelector<HTMLInputElement>('input')!.focus();
    await userEvent.keyboard('{ArrowRight}');
    expect(onchange).toHaveBeenCalledWith('newest');
  });
});
