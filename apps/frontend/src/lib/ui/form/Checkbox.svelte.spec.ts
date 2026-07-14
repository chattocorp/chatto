import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import Checkbox from './Checkbox.svelte';

describe('Checkbox', () => {
  it('keeps a saving option checked and non-interactive', () => {
    const { container } = render(Checkbox, {
      props: {
        id: 'moderator',
        label: 'Community moderator',
        checked: true,
        loading: true
      }
    });

    const label = container.querySelector('label') as HTMLLabelElement;
    const input = container.querySelector('input') as HTMLInputElement;

    expect(label.getAttribute('aria-busy')).toBe('true');
    expect(input.checked).toBe(true);
    expect(input.disabled).toBe(true);
  });
});
