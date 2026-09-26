import { flushSync } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { testSnippet } from '$lib/test-utils';
import FrameView from './FrameView.svelte';

afterEach(() => {
  document.body.querySelectorAll(':scope > button').forEach((button) => button.remove());
});

function renderView(onclose = vi.fn()) {
  const view = render(FrameView, {
    props: {
      title: 'Server Directory',
      subtitle: 'Discover other Chatto servers',
      onclose,
      children: testSnippet('<input data-testid="frame-view-input" />')
    }
  });
  const root = view.container.querySelector<HTMLElement>('[data-testid="frame-view"]')!;
  return { ...view, root, onclose };
}

describe('FrameView', () => {
  it('shows a pane header and takes focus when it opens', async () => {
    const { root } = renderView();

    expect(root.getAttribute('aria-label')).toBe('Server Directory');
    expect(root.querySelector('h1')?.textContent).toContain('Server Directory');
    expect(document.activeElement).toBe(root);
  });

  it('closes from the close button and from Escape', async () => {
    const { root, onclose, getByRole } = renderView();

    await getByRole('button', { name: 'Close' }).click();
    expect(onclose).toHaveBeenCalledTimes(1);

    root
      .querySelector<HTMLInputElement>('[data-testid="frame-view-input"]')!
      .dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    expect(onclose).toHaveBeenCalledTimes(2);
  });

  it('leaves an Escape that a nested control handled', async () => {
    const { root, onclose } = renderView();
    const input = root.querySelector<HTMLInputElement>('[data-testid="frame-view-input"]')!;
    input.addEventListener('keydown', (event) => event.preventDefault());

    input.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })
    );

    expect(onclose).not.toHaveBeenCalled();
  });

  it('ignores an Escape from outside the view', async () => {
    const { onclose } = renderView();
    const outside = document.createElement('button');
    document.body.append(outside);

    outside.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));

    expect(onclose).not.toHaveBeenCalled();
  });

  it('returns focus to the opener when it closes', async () => {
    const opener = document.createElement('button');
    document.body.append(opener);
    opener.focus();

    const { unmount } = renderView();
    expect(document.activeElement).not.toBe(opener);
    unmount();
    flushSync();

    await vi.waitFor(() => expect(document.activeElement).toBe(opener));
  });
});
