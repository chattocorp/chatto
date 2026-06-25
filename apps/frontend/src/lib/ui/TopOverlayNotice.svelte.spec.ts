import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import TopOverlayNotice from './TopOverlayNotice.svelte';

function buttonWithText(container: Element, text: string): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll('button')).find((candidate) =>
    candidate.textContent?.includes(text)
  );
  if (!button) {
    throw new Error(`Button with text "${text}" not found`);
  }
  return button;
}

describe('TopOverlayNotice', () => {
  it('renders title, message, and action callbacks', async () => {
    const primary = vi.fn();
    const secondary = vi.fn();
    const { container } = render(TopOverlayNotice, {
      props: {
        title: 'Enable push notifications',
        message: 'Get notified when Chatto is closed.',
        primaryAction: {
          label: 'Enable',
          onclick: primary
        },
        secondaryAction: {
          label: 'Not now',
          onclick: secondary
        }
      }
    });

    expect(container.textContent).toContain('Enable push notifications');
    expect(container.textContent).toContain('Get notified when Chatto is closed.');
    expect(container.querySelector('section')).toHaveClass('menu');
    expect(container.querySelector('.menu-section')).not.toBeNull();
    expect(container.firstElementChild?.className).toContain('safe-area-inset-top');
    await expect.element(buttonWithText(container, 'Not now')).toHaveClass('btn-secondary');

    buttonWithText(container, 'Enable').click();
    buttonWithText(container, 'Not now').click();

    expect(primary).toHaveBeenCalledOnce();
    expect(secondary).toHaveBeenCalledOnce();
  });
});
