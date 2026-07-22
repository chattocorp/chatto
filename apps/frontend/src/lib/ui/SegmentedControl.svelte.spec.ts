import { describe, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { render } from 'vitest-browser-svelte';
import SegmentedControl from './SegmentedControl.svelte';

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

    expect(container.querySelector('legend')?.textContent).toBe('Sort messages');
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
});
