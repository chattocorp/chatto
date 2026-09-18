import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MotdContent from './MotdContent.svelte';
import '../../app.css';

describe('MOTD preview', () => {
  it('updates rendered Markdown when the active server or message changes', async () => {
    const rendered = render(MotdContent, { motd: '**First server**', onclick: vi.fn() });
    await expect.element(rendered.getByText('First server', { exact: true })).toBeVisible();
    await rendered.rerender({ motd: '**Second server**' });
    await expect.element(rendered.getByText('Second server', { exact: true })).toBeVisible();
    expect(rendered.container.querySelector('strong')?.textContent).toBe('Second server');
  });

  it('keeps links separate from the modal trigger and preserves the content selector', async () => {
    const onclick = vi.fn();
    const rendered = render(MotdContent, { motd: '[Details](https://example.com)', onclick });
    // Match the padded header around its controls' negative margins.
    rendered.container.style.padding = '16px';
    const link = rendered.getByRole('link', { name: 'Details' });
    await expect.element(link).toBeVisible();
    await expect.element(rendered.getByTestId('motd-content')).toHaveTextContent('Details');
    const anchor = rendered.container.querySelector('a')!;
    expect(anchor.closest('button')).toBeNull();
    // Exercise the real link hit target without contacting an external site.
    anchor.addEventListener('click', (event) => event.preventDefault(), { once: true });
    await link.click();
    expect(onclick).not.toHaveBeenCalled();
    await rendered.getByRole('button', { name: 'Message of the Day' }).click({
      position: { x: 4, y: 4 }
    });
    expect(onclick).toHaveBeenCalledOnce();
  });
});
