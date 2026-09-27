import '../../app.css';
import { afterEach, describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';

function mount(html: string) {
  const host = document.createElement('div');
  host.innerHTML = html;
  document.body.append(host);
  return host;
}

afterEach(() => {
  document.body.replaceChildren();
});

describe('icon-action-danger', () => {
  it('shows the danger colour on hover over the shared icon-action hover colour', async () => {
    const host = mount(`
      <span class="text-danger" data-testid="danger-probe">probe</span>
      <button type="button" class="icon-action icon-action-danger" aria-label="Delete">x</button>
      <button type="button" class="field-action icon-action-danger" aria-label="Clear">x</button>
    `);
    const danger = getComputedStyle(host.querySelector('[data-testid="danger-probe"]')!).color;

    for (const button of host.querySelectorAll<HTMLButtonElement>('button')) {
      await userEvent.hover(button);
      expect(getComputedStyle(button).color).toBe(danger);
    }
  });

  it('keeps disabled icon actions out of the danger hover colour', async () => {
    const host = mount(`
      <span class="text-danger" data-testid="danger-probe">probe</span>
      <button type="button" class="icon-action icon-action-danger" aria-label="Delete" disabled>x</button>
    `);
    const danger = getComputedStyle(host.querySelector('[data-testid="danger-probe"]')!).color;
    const button = host.querySelector('button')!;

    await userEvent.hover(button, { force: true });

    expect(getComputedStyle(button).color).not.toBe(danger);
  });
});
