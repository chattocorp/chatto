import '../../../app.css';
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

it('keeps native focus inside its row when nested in a scrolled pane', async () => {
  const outer = document.createElement('div');
  outer.style.cssText = 'position:relative;height:200px;overflow:hidden';
  const pane = document.createElement('div');
  pane.style.cssText = 'height:200px;overflow:auto';
  const spacer = document.createElement('div');
  spacer.style.height = '600px';
  outer.append(pane);
  pane.append(spacer);
  document.body.append(outer);
  const screen = render(Checkbox, { id: 'scrolled-checkbox', label: 'Low-cut filter' });
  const details = document.createElement('details');
  const summary = document.createElement('summary');
  summary.textContent = 'Microphone processing';
  details.append(summary, screen.container);
  pane.append(details);
  details.open = true;
  try {
    pane.scrollTop = pane.scrollHeight;
    const input = screen.container.querySelector('input')!;
    const label = screen.container.querySelector('label')!;
    const row = label.getBoundingClientRect();
    const native = input.getBoundingClientRect();
    // The hidden input must scroll with its visible label, not with the outer frame.
    expect(native.top).toBeGreaterThanOrEqual(row.top);
    expect(native.bottom).toBeLessThanOrEqual(row.bottom);
    await screen.getByText('Low-cut filter').click();
    expect(input.checked).toBe(true);
    expect(outer.scrollTop).toBe(0);
    expect(document.activeElement).toBe(input);
  } finally {
    await screen.unmount();
    outer.remove();
  }
});
