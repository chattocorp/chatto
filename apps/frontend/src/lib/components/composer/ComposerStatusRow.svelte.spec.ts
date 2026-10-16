import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import ComposerStatusRowMock from './ComposerStatusRowMock.svelte';
import '../../../app.css';

describe('Composer status badges', () => {
  it('keeps the empty typing live region available before activity starts', () => {
    const view = render(ComposerStatusRowMock, { typing: false });
    const region = view.container.querySelector('[role="status"]')!;
    expect(getComputedStyle(region).display).not.toBe('none');
    expect(region.textContent?.trim()).toBe('');
  });

  it('keeps slow mode visible beside long typing labels in both directions', async () => {
    const view = render(ComposerStatusRowMock, { remaining: 23 });
    for (const direction of ['ltr', 'rtl'] as const) {
      await view.rerender({ direction });
      const host = view.container
        .querySelector('[data-testid="status-host"]')!
        .getBoundingClientRect();
      const typing = view.container
        .querySelector('[data-testid="typing-indicator"]')!
        .getBoundingClientRect();
      const slow = view.container
        .querySelector('[data-testid="slow-mode-status"]')!
        .getBoundingClientRect();
      expect(Math.min(typing.left, slow.left)).toBeGreaterThanOrEqual(host.left);
      expect(Math.max(typing.right, slow.right)).toBeLessThanOrEqual(host.right);
      expect(direction === 'ltr' ? typing.right <= slow.left : slow.right <= typing.left).toBe(
        true
      );
      expect(slow.top).toBeLessThan(host.top);
      expect(slow.bottom).toBeGreaterThan(host.top);
    }
  });

  it('explains the cooldown on click and disappears when it expires', async () => {
    const view = render(ComposerStatusRowMock, { remaining: 23 });
    const badge = page.getByTestId('slow-mode-status');
    await expect.element(badge).toHaveTextContent('0:23');
    await page.getByRole('button', { name: 'Slow Mode: send again in 0:23.' }).click();
    await expect
      .element(page.getByRole('tooltip'))
      .toHaveTextContent('Slow Mode: send again in 0:23.');
    await view.rerender({ remaining: 0 });
    await expect.element(badge).not.toBeInTheDocument();
    await expect.element(page.getByRole('tooltip')).not.toBeInTheDocument();
    await expect.element(page.getByTestId('typing-indicator')).toBeVisible();
  });

  it('hides the badge for exempt users and when slow mode is disabled', async () => {
    const view = render(ComposerStatusRowMock, { remaining: 23, bypassed: true, typing: false });
    await expect.element(page.getByTestId('slow-mode-status')).not.toBeInTheDocument();
    await view.rerender({ seconds: 0, bypassed: false });
    await expect.element(page.getByTestId('slow-mode-status')).not.toBeInTheDocument();
    await expect.element(page.getByTestId('typing-indicator')).not.toBeInTheDocument();
  });
});
