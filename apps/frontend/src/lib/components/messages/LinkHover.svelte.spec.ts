import '../../../app.css';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { createRawSnippet, tick } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { queryClient } from '$lib/query/queryClient';
import { applyAnimationPreference } from '$lib/dom/motionPreference';
import LinkHover from './LinkHover.svelte';

vi.mock(
  '$lib/state/server/scope.svelte',
  async () => (await import('$lib/test-utils/serverScope.svelte')).serverScopeModule
);
const fetchLinkPreview = vi.fn();
const href = 'https://example.test/guide';
const children = createRawSnippet(() => ({
  render: () => '<a href="https://example.test/guide">example.test/guide</a>'
}));
beforeEach(() => {
  applyAnimationPreference('enabled');
  fetchLinkPreview.mockReset().mockResolvedValue({
    url: href,
    title: 'A useful guide',
    description: 'Details',
    imageUrl: null
  });
  createTestServerScope({ api: { fetchLinkPreview } });
});
afterEach(() => {
  applyAnimationPreference('automatic');
  queryClient.clear();
});

function hover(container: HTMLElement) {
  container.querySelector('a')!.parentElement!.dispatchEvent(new MouseEvent('mouseenter'));
}
it('fetches metadata on hover by default and describes the link', async () => {
  const { container } = render(LinkHover, { href, children });
  expect(fetchLinkPreview).not.toHaveBeenCalled();
  hover(container);
  await vi.waitFor(() =>
    expect(container.querySelector('[data-testid="link-hover-card"]')?.textContent).toContain(href)
  );
  const card = container.querySelector<HTMLElement>('[data-testid="link-hover-card"]')!;
  expect(container.querySelector('a')!.getAttribute('aria-describedby')).toContain(card.id);
  await vi.waitFor(() => expect(container.textContent).toContain('A useful guide'));
  expect(fetchLinkPreview).toHaveBeenCalledExactlyOnceWith(href);
});
it('uses local entity metadata without fetching', async () => {
  const { container } = render(LinkHover, {
    href,
    children,
    local: { name: 'Development', description: 'Build together', server: 'Community' }
  });
  hover(container);
  await vi.waitFor(() => expect(container.textContent).toContain('Build together'));
  expect(fetchLinkPreview).not.toHaveBeenCalled();
});
it('keeps the card in the top layer until its zoom-fade exit completes', async () => {
  const { container } = render(LinkHover, {
    href,
    children,
    local: { name: 'Development' }
  });
  hover(container);
  await vi.waitFor(() => expect(container.querySelector('[popover]')).not.toBeNull());
  const popover = container.querySelector<HTMLElement>('[popover]')!;
  await Promise.all(popover.getAnimations().map((animation) => animation.finished));
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
  await tick();
  expect(popover.matches(':popover-open')).toBe(true);
  // Svelte starts the outro on the next animation frame; tick only flushes DOM work.
  await vi.waitFor(() =>
    expect(
      popover
        .getAnimations()
        .some(
          (animation) => (animation.effect as KeyframeEffect).getKeyframes().at(-1)?.opacity === '0'
        )
    ).toBe(true)
  );
  const exit = popover
    .getAnimations()
    .find(
      (animation) => (animation.effect as KeyframeEffect).getKeyframes().at(-1)?.opacity === '0'
    );
  expect(exit).toBeDefined();
  const frames = (exit!.effect as KeyframeEffect).getKeyframes();
  expect(frames.at(-1)?.opacity).toBe('0');
  expect(frames.at(-1)?.transform).toContain('scale(0.95)');
  await vi.waitFor(() => expect(container.querySelector('[popover]')).toBeNull());
  expect(container.querySelector('a')!.hasAttribute('aria-describedby')).toBe(false);
});
it('cancels brief hovers and pending opens on Escape', async () => {
  // Advance the debounce in the same task as the synthetic pointer events.
  // Wall-clock waits let the browser's real pointer re-enter the new fixture.
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  try {
    const { container } = render(LinkHover, { href, children });
    hover(container);
    vi.advanceTimersByTime(349);
    await tick();
    expect(container.querySelector('[data-testid="link-hover-card"]')).toBeNull();
    container.querySelector('a')!.parentElement!.dispatchEvent(new MouseEvent('mouseleave'));
    vi.advanceTimersByTime(400);
    await tick();
    expect(fetchLinkPreview).not.toHaveBeenCalled();
    expect(container.querySelector('[data-testid="link-hover-card"]')).toBeNull();
    hover(container);
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    vi.advanceTimersByTime(400);
    await tick();
    expect(container.querySelector('[data-testid="link-hover-card"]')).toBeNull();
    // A sustained hover still opens: cancellation must not disable the trigger.
    hover(container);
    vi.advanceTimersByTime(350);
    await tick();
    expect(container.querySelector('[data-testid="link-hover-card"]')).not.toBeNull();
  } finally {
    vi.useRealTimers();
  }
});
it('skips transitions when animations are disabled in the app', async () => {
  applyAnimationPreference('disabled');
  const { container } = render(LinkHover, { href, children, local: { name: 'Development' } });
  hover(container);
  await vi.waitFor(() => expect(container.querySelector('[popover]')).not.toBeNull());
  expect(container.querySelector('[popover]')!.getAnimations()).toHaveLength(0);
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
  await vi.waitFor(() => expect(container.querySelector('[popover]')).toBeNull());
});
it('shares cached previews across cards and keeps a URL fallback on failure', async () => {
  const first = render(LinkHover, { href, children });
  hover(first.container);
  await vi.waitFor(() => expect(first.container.textContent).toContain('A useful guide'));
  const second = render(LinkHover, { href, children });
  hover(second.container);
  await vi.waitFor(() => expect(second.container.textContent).toContain('A useful guide'));
  expect(fetchLinkPreview).toHaveBeenCalledTimes(1);
  fetchLinkPreview.mockRejectedValue(new Error('unavailable'));
  const failed = render(LinkHover, { href: href + '/missing', children });
  hover(failed.container);
  await vi.waitFor(() => expect(fetchLinkPreview).toHaveBeenCalledTimes(2));
  await vi.waitFor(() => expect(failed.container.querySelector('[role="status"]')).toBeNull());
  expect(failed.container.textContent).toContain(href + '/missing');
  expect(failed.container.textContent).toContain('No preview is available');
});
