import '../../app.css';
import { afterEach, describe, expect, it } from 'vitest';
import { userEvent } from 'vitest/browser';

// Hover colours only apply where the primary input can hover. Headless
// browsers in CI may report no hover support, so the hover tests skip there.
const supportsHover = matchMedia('(hover: hover)').matches;

function mount(html: string) {
  const host = document.createElement('div');
  host.innerHTML = html;
  document.body.append(host);
  return host;
}

function dangerColour(host: HTMLElement) {
  return getComputedStyle(host.querySelector('[data-testid="danger-probe"]')!).color;
}

afterEach(() => {
  document.body.replaceChildren();
});

describe.each(['icon-action', 'btn-secondary attachment-action-button'])(
  '%s emphasis',
  (classes) => {
    it('uses the full-intensity foreground on keyboard focus', async () => {
      const host = mount(`
      <span class="text-text-top" data-testid="foreground-probe">probe</span>
      <button type="button" class="${classes}">Action</button>
    `);
      const button = host.querySelector('button')!;
      const foreground = getComputedStyle(
        host.querySelector('[data-testid="foreground-probe"]')!
      ).color;
      await userEvent.tab();
      expect(document.activeElement).toBe(button);
      await expect.poll(() => getComputedStyle(button).color).toBe(foreground);
    });

    it.skipIf(!supportsHover)(
      'brightens on hover but leaves disabled icons unchanged',
      async () => {
        const host = mount(`
      <span class="text-text-top" data-testid="foreground-probe">probe</span>
      <button type="button" class="${classes}">Action</button>
      <button type="button" class="${classes}" disabled>Disabled</button>
    `);
        const [button, disabled] = host.querySelectorAll('button');
        const foreground = getComputedStyle(
          host.querySelector('[data-testid="foreground-probe"]')!
        ).color;
        const disabledColour = getComputedStyle(disabled).color;
        await userEvent.hover(button);
        await expect.poll(() => getComputedStyle(button).color).toBe(foreground);
        await userEvent.hover(disabled, { force: true });
        expect(getComputedStyle(disabled).color).toBe(disabledColour);
      }
    );
  }
);

describe('icon-action-danger', () => {
  it('shows the danger colour on keyboard focus', async () => {
    const host = mount(`
      <span class="text-danger" data-testid="danger-probe">probe</span>
      <button type="button" class="icon-action icon-action-danger" aria-label="Delete">x</button>
      <button type="button" class="field-action icon-action-danger" aria-label="Clear">x</button>
    `);
    const danger = dangerColour(host);

    for (const button of host.querySelectorAll<HTMLButtonElement>('button')) {
      await userEvent.tab();
      expect(document.activeElement).toBe(button);
      expect(getComputedStyle(button).color).toBe(danger);
    }
  });

  it.skipIf(!supportsHover)(
    'shows the danger colour on hover over the shared icon-action hover colour',
    async () => {
      const host = mount(`
        <span class="text-danger" data-testid="danger-probe">probe</span>
        <button type="button" class="icon-action icon-action-danger" aria-label="Delete">x</button>
        <button type="button" class="field-action icon-action-danger" aria-label="Clear">x</button>
      `);
      const danger = dangerColour(host);

      for (const button of host.querySelectorAll<HTMLButtonElement>('button')) {
        await userEvent.hover(button);
        expect(getComputedStyle(button).color).toBe(danger);
      }
    }
  );

  it.skipIf(!supportsHover)(
    'keeps disabled icon actions out of the danger hover colour',
    async () => {
      const host = mount(`
        <span class="text-danger" data-testid="danger-probe">probe</span>
        <button type="button" class="icon-action icon-action-danger" aria-label="Delete" disabled>x</button>
      `);
      const danger = dangerColour(host);
      const button = host.querySelector('button')!;

      await userEvent.hover(button, { force: true });

      expect(getComputedStyle(button).color).not.toBe(danger);
    }
  );
});
