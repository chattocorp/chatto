import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import Checkbox from './Checkbox.svelte';

describe('Checkbox', () => {
  it('toggles through the native input and selects the complete row', async () => {
    const { container, getByRole, getByText } = render(Checkbox, {
      props: {
        id: 'timezone-sharing',
        label: 'Show my time zone on my profile',
        description: 'Other members can see your current local time.'
      }
    });

    const label = container.querySelector('label') as HTMLLabelElement;
    const input = getByRole('checkbox', { name: 'Show my time zone on my profile' });

    expect(label.classList.contains('checkbox-option-selected')).toBe(false);
    await getByText('Show my time zone on my profile').click();
    await expect.element(input).toBeChecked();
    expect(label.classList.contains('checkbox-option-selected')).toBe(true);
    expect(input.element().getAttribute('aria-describedby')).toBe('timezone-sharing-description');
  });

  it('keeps the error treatment when a checked option is invalid', () => {
    const { container, getByRole } = render(Checkbox, {
      props: {
        id: 'terms',
        label: 'Accept the terms',
        checked: true,
        error: 'Review the terms before you continue.'
      }
    });

    const label = container.querySelector('label') as HTMLLabelElement;
    const box = container.querySelector('.checkbox-box') as HTMLSpanElement;
    const input = getByRole('checkbox', { name: 'Accept the terms' });

    expect(label.classList.contains('checkbox-option-error')).toBe(true);
    expect(label.classList.contains('checkbox-option-selected')).toBe(false);
    expect(box.classList.contains('checkbox-box-selected')).toBe(true);
    expect(box.classList.contains('checkbox-box-error')).toBe(true);
    expect(input.element().getAttribute('aria-invalid')).toBe('true');
    expect(input.element().getAttribute('aria-describedby')).toBe('terms-error');
  });

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
    expect(label.classList.contains('checkbox-option-selected')).toBe(true);
    expect(input.checked).toBe(true);
    expect(input.disabled).toBe(true);
  });
});
