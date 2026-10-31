import '../../../app.css';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { createRawSnippet, tick } from 'svelte';
import { render } from 'vitest-browser-svelte';
import { createTestServerScope } from '$lib/test-utils/serverScope.svelte';
import { queryClient } from '$lib/query/queryClient';
import { applyAnimationPreference } from '$lib/dom/motionPreference';
import LinkHover from './LinkHover.svelte';
import LinkTitleTestHarness from './LinkTitleTestHarness.svelte';
import MessageContent from '../MessageContent.svelte';
import {
  removeRegisteredServerQueries,
  refreshRegisteredServerQueries
} from '$lib/query/cacheRegistry';

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
  createTestServerScope({ api: { fetchLinkMetadata: fetchLinkPreview } });
});
afterEach(() => {
  applyAnimationPreference('automatic');
  queryClient.clear();
  vi.restoreAllMocks();
});

function hover(container: HTMLElement) {
  container.querySelector('a')!.parentElement!.dispatchEvent(new MouseEvent('mouseenter'));
}
it('loads a posted URL pill without hover and shares its request with the hover card', async () => {
  let resolve!: (value: unknown) => void;
  fetchLinkPreview.mockReturnValue(
    new Promise((done) => {
      resolve = done;
    })
  );
  const { container } = render(MessageContent, { body: href });
  await vi.waitFor(() => expect(fetchLinkPreview).toHaveBeenCalledExactlyOnceWith(href));
  const pill = container.querySelector<HTMLAnchorElement>('.url-link')!;
  expect(pill.getAttribute('aria-busy')).toBe('true');
  expect(pill.querySelector('.animate-spin')).not.toBeNull();
  expect(container.querySelector('[data-testid="link-hover-card"]')).toBeNull();
  hover(container);
  await vi.waitFor(() =>
    expect(container.querySelector('[data-testid="link-hover-card"]')).not.toBeNull()
  );
  expect(fetchLinkPreview).toHaveBeenCalledTimes(1);
  resolve({ url: href, title: 'Automatically loaded title' });
  await vi.waitFor(() => expect(pill.textContent).toContain('Automatically loaded title'));
  expect(pill.getAttribute('aria-busy')).toBe('false');
  expect(pill.href).toBe(href);
});

it('waits until a posted pill enters the viewport', async () => {
  const target = document.createElement('div');
  target.style.cssText = 'position:fixed;top:200vh';
  document.body.append(target);
  try {
    render(MessageContent, { target, props: { body: href } });
    await vi.waitFor(() => expect(target.querySelector('.url-link')).not.toBeNull());
    // Wait for the initial intersection notification before checking the request.
    await new Promise<void>((resolve) => {
      const observer = new IntersectionObserver(() => {
        observer.disconnect();
        resolve();
      });
      observer.observe(target);
    });
    expect(fetchLinkPreview).not.toHaveBeenCalled();
    target.style.top = '0';
    await vi.waitFor(() => expect(fetchLinkPreview).toHaveBeenCalledExactlyOnceWith(href));
  } finally {
    target.remove();
  }
});

it('keeps authored Markdown labels lazy until interaction', async () => {
  const { container } = render(MessageContent, { body: `[Read the guide](${href})` });
  await vi.waitFor(() => expect(container.querySelector('a')?.textContent).toBe('Read the guide'));
  await new Promise<void>((resolve) => {
    const observer = new IntersectionObserver(() => {
      observer.disconnect();
      resolve();
    });
    observer.observe(container);
  });
  expect(fetchLinkPreview).not.toHaveBeenCalled();
  container.querySelector('a')!.focus();
  await vi.waitFor(() => expect(fetchLinkPreview).toHaveBeenCalledExactlyOnceWith(href));
  expect(container.querySelector('a')?.textContent).toBe('Read the guide');
});

it.each(['success', 'failure'])('shows a pill spinner until metadata fetch %s', async (outcome) => {
  let resolve!: (value: unknown) => void;
  let reject!: (reason: Error) => void;
  fetchLinkPreview.mockReturnValue(
    new Promise((done, fail) => {
      resolve = done;
      reject = fail;
    })
  );
  const { container } = render(LinkTitleTestHarness, { href });
  const trigger = container.querySelector<HTMLAnchorElement>('.url-link')!;
  expect(trigger.getAttribute('aria-busy')).toBe('false');
  hover(container);
  await vi.waitFor(() => expect(fetchLinkPreview).toHaveBeenCalled());
  expect(trigger.getAttribute('aria-busy')).toBe('true');
  expect(trigger.querySelector('.url-link-icon .animate-spin')).not.toBeNull();
  expect(trigger.href).toBe(href);
  if (outcome === 'success') resolve({ url: href, title: 'Loaded title' });
  else reject(new Error('unavailable'));
  await vi.waitFor(() => expect(trigger.getAttribute('aria-busy')).toBe('false'));
  expect(trigger.querySelector('.animate-spin')).toBeNull();
  expect(trigger.querySelector('.url-link-icon')).not.toBeNull();
  expect(trigger.textContent).toContain(outcome === 'success' ? 'Loaded title' : 'example.test');
});

it('reflects metadata loading from the composer without a hover request', async () => {
  const { container, rerender } = render(LinkTitleTestHarness, { href, loading: true });
  const trigger = container.querySelector<HTMLAnchorElement>('.url-link')!;
  expect(trigger.getAttribute('aria-busy')).toBe('true');
  expect(trigger.querySelector('.url-link-icon .animate-spin')).not.toBeNull();
  await rerender({ href, loading: false });
  expect(trigger.getAttribute('aria-busy')).toBe('false');
  expect(trigger.querySelector('.animate-spin')).toBeNull();
  expect(fetchLinkPreview).not.toHaveBeenCalled();
});

/** Dispatch a touch pointer without changing the browser's mouse capability. */
function touchPointer(link: Element, type: string, clientX = 20) {
  link.dispatchEvent(
    new PointerEvent(type, {
      bubbles: true,
      cancelable: true,
      pointerType: 'touch',
      pointerId: 1,
      isPrimary: true,
      clientX,
      clientY: 20
    })
  );
}

it('opens one persistent touch sheet and consumes the native menu and release click', async () => {
  const { container } = render(LinkHover, { href, children });
  const link = container.querySelector('a')!;
  const parentTouch = vi.fn();
  const parentMenu = vi.fn();
  container.addEventListener('touchstart', parentTouch);
  container.addEventListener('contextmenu', parentMenu);
  touchPointer(link, 'pointerdown');
  link.focus();
  link.dispatchEvent(new Event('touchstart', { bubbles: true }));
  await vi.waitFor(() => expect(container.querySelector('dialog[open]')).not.toBeNull());
  // A hybrid device can report mouse movement while the touch menu is open.
  link.dispatchEvent(new PointerEvent('pointerover', { bubbles: true, pointerType: 'mouse' }));
  const nativeMenu = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
  link.dispatchEvent(nativeMenu);
  touchPointer(link, 'pointerup');
  const click = new MouseEvent('click', { bubbles: true, cancelable: true });
  link.dispatchEvent(click);
  await tick();
  expect(nativeMenu.defaultPrevented).toBe(true);
  expect(click.defaultPrevented).toBe(true);
  // The app shell still needs the touch sequence for sidebar swipes.
  expect(parentTouch).toHaveBeenCalledOnce();
  expect(parentMenu).not.toHaveBeenCalled();
  expect(container.querySelectorAll('dialog[open]')).toHaveLength(1);
  // A mouse hover card can remain in the DOM until its exit animation ends.
  await vi.waitFor(() =>
    expect(container.querySelector('[data-testid="link-hover-card"]')).toBeNull()
  );
  await vi.waitFor(() =>
    expect(container.querySelector('dialog')?.textContent).toContain('A useful guide')
  );
  await new Promise((resolve) => setTimeout(resolve, 250));
  expect(container.querySelector('dialog[open]')).not.toBeNull();
  expect(fetchLinkPreview).toHaveBeenCalledExactlyOnceWith(href);
  const copy = [...container.querySelectorAll('dialog button')].find((button) =>
    button.textContent?.includes('Copy link')
  )!;
  const write = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
  (copy as HTMLButtonElement).click();
  await vi.waitFor(() => expect(container.querySelector('dialog')).toBeNull());
  expect(write).toHaveBeenCalledWith(href);
});

it('shows a hover card on the first mouse entry after touch input', async () => {
  const { container } = render(LinkHover, { href, children, local: { name: 'Room' } });
  const link = container.querySelector('a')!;
  touchPointer(link, 'pointerdown');
  touchPointer(link, 'pointerup');
  // Pointer entry precedes mouseenter; pointermove comes later.
  link.dispatchEvent(new PointerEvent('pointerover', { bubbles: true, pointerType: 'mouse' }));
  hover(container);
  await vi.waitFor(() =>
    expect(container.querySelector('[data-testid="link-hover-card"]')).not.toBeNull()
  );
});

it('suppresses release navigation after holding longer than the click fallback timer', async () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  try {
    const { container } = render(LinkHover, { href, children, local: { name: 'Room' } });
    const link = container.querySelector('a')!;
    touchPointer(link, 'pointerdown');
    vi.advanceTimersByTime(2000);
    await tick();
    touchPointer(link, 'pointerup');
    const release = new MouseEvent('click', { bubbles: true, cancelable: true });
    link.dispatchEvent(release);
    expect(release.defaultPrevented).toBe(true);
    expect(container.querySelector('dialog[open]')).not.toBeNull();
    const nextClick = new MouseEvent('click', { bubbles: true, cancelable: true });
    const deliberateClick = vi.fn((event: MouseEvent) => {
      expect(event.defaultPrevented).toBe(false);
      // The fixture is a native anchor. Keep the test iframe on its origin.
      event.preventDefault();
    });
    link.addEventListener('click', deliberateClick, { once: true });
    link.dispatchEvent(nextClick);
    expect(deliberateClick).toHaveBeenCalledOnce();
  } finally {
    vi.useRealTimers();
  }
});

it('discards a release click retargeted into the sheet, then permits a new deliberate press', async () => {
  const open = vi.spyOn(window, 'open').mockReturnValue(null);
  const { container } = render(LinkHover, { href, children, local: { name: 'Room' } });
  touchPointer(container.querySelector('a')!, 'pointerdown');
  const action = await vi.waitFor(() => {
    const button = [...container.querySelectorAll<HTMLButtonElement>('dialog button')].find(
      (button) => button.textContent?.trim() === 'Open link'
    );
    expect(button).toBeDefined();
    return button!;
  });
  action.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, detail: 1 }));
  expect(open).not.toHaveBeenCalled();
  expect(container.querySelector('dialog[open]')).not.toBeNull();
  touchPointer(action, 'pointerdown');
  touchPointer(action, 'pointerup');
  action.click();
  expect(open).toHaveBeenCalledExactlyOnceWith(href, '_blank', 'noopener,noreferrer');
});

it.each(['tap', 'scroll', 'cancel'])('does not preview during a touch %s', async (gesture) => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  try {
    const { container } = render(LinkHover, { href, children });
    const link = container.querySelector('a')!;
    touchPointer(link, 'pointerdown');
    if (gesture === 'scroll') touchPointer(link, 'pointermove', 50);
    else touchPointer(link, gesture === 'tap' ? 'pointerup' : 'pointercancel');
    // Compatibility mouse/focus events must not open a hover card on touch.
    hover(container);
    link.focus();
    vi.advanceTimersByTime(600);
    await tick();
    expect(container.querySelector('dialog')).toBeNull();
    expect(container.querySelector('[data-testid="link-hover-card"]')).toBeNull();
    expect(fetchLinkPreview).not.toHaveBeenCalled();
  } finally {
    vi.useRealTimers();
  }
});

it.each([false, true])('uses Shift=%s to bypass the message menu', (shiftKey) => {
  const { container } = render(LinkHover, { href, children });
  const parentMenu = vi.fn();
  container.addEventListener('contextmenu', parentMenu);
  const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, shiftKey });
  container.querySelector('a')!.dispatchEvent(event);
  expect(event.defaultPrevented).toBe(false);
  expect(parentMenu).toHaveBeenCalledTimes(shiftKey ? 0 : 1);
});
it('reuses a fetched title without replacing the focused anchor or its destination', async () => {
  const { container } = render(LinkTitleTestHarness, { href });
  const trigger = container.querySelector<HTMLAnchorElement>('.url-link')!;
  expect(trigger.textContent).toContain('example.test');
  trigger.focus();
  await vi.waitFor(() => expect(trigger.textContent).toContain('A useful guide'));
  expect(document.activeElement).toBe(trigger);
  expect(trigger.href).toBe(href);
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
  await vi.waitFor(() =>
    expect(container.querySelector('[data-testid="link-hover-card"]')).toBeNull()
  );
  expect(trigger.textContent).toContain('A useful guide');
  expect(fetchLinkPreview).toHaveBeenCalledTimes(1);
});

it('uses existing metadata immediately and truncates long titles without fetching', async () => {
  const title = 'A very long page title '.repeat(30);
  const { container } = render(LinkTitleTestHarness, {
    href,
    preview: {
      url: href,
      title,
      description: null,
      imageUrl: null,
      siteName: null,
      embedType: null,
      embedId: null
    }
  });
  const trigger = container.querySelector<HTMLAnchorElement>('.url-link')!;
  expect(trigger.textContent).toContain(title.trim());
  container.style.width = '320px';
  expect(trigger.getBoundingClientRect().width).toBeLessThanOrEqual(320);
  expect(getComputedStyle(trigger.querySelector('bdi')!).textOverflow).toBe('ellipsis');
  hover(container);
  await vi.waitFor(() =>
    expect(container.querySelector('[data-testid="link-hover-card"]')).not.toBeNull()
  );
  expect(fetchLinkPreview).not.toHaveBeenCalled();
});

it.each([removeRegisteredServerQueries, refreshRegisteredServerQueries])(
  'reloads a visible pill without hover after %s',
  async (reset) => {
    const scope = createTestServerScope({ api: { fetchLinkMetadata: fetchLinkPreview } });
    const { container } = render(LinkTitleTestHarness, { href, loadWhenVisible: true });
    const trigger = container.querySelector('.url-link')!;
    await vi.waitFor(() => expect(trigger.textContent).toContain('A useful guide'));
    fetchLinkPreview.mockResolvedValue({ url: href, title: 'Reauthorized title' });
    await reset(scope.serverId);
    await vi.waitFor(() => expect(trigger.textContent).toContain('Reauthorized title'));
    expect(fetchLinkPreview).toHaveBeenCalledTimes(2);
    expect(trigger.textContent).not.toContain('A useful guide');
    fetchLinkPreview.mockResolvedValue({ url: href, title: 'Another permission change' });
    await reset(scope.serverId);
    await vi.waitFor(() => expect(trigger.textContent).toContain('Another permission change'));
    expect(fetchLinkPreview).toHaveBeenCalledTimes(3);
  }
);

it('does not reveal a title protected by a social content warning', async () => {
  fetchLinkPreview.mockResolvedValue({
    url: href,
    title: 'Concealed title',
    socialPost: { contentWarning: 'Sensitive content' }
  });
  const { container } = render(LinkTitleTestHarness, { href });
  hover(container);
  await vi.waitFor(() => expect(fetchLinkPreview).toHaveBeenCalled());
  await tick();
  expect(container.querySelector('.url-link')?.textContent).toContain('example.test');
  expect(container.textContent).not.toContain('Concealed title');
});

it('ignores a pending response after the server session is cleared', async () => {
  const scope = createTestServerScope({ api: { fetchLinkMetadata: fetchLinkPreview } });
  let resolve!: (value: unknown) => void;
  fetchLinkPreview.mockReturnValueOnce(
    new Promise((done) => {
      resolve = done;
    })
  );
  const { container } = render(LinkTitleTestHarness, { href, loadWhenVisible: true });
  await vi.waitFor(() => expect(fetchLinkPreview).toHaveBeenCalled());
  fetchLinkPreview.mockResolvedValue({ url: href, title: 'Fresh authorized title' });
  removeRegisteredServerQueries(scope.serverId);
  resolve({ url: href, title: 'Old session title' });
  await tick();
  await vi.waitFor(() =>
    expect(container.querySelector('.url-link')?.textContent).toContain('Fresh authorized title')
  );
  expect(container.textContent).not.toContain('Old session title');
});
it.each([
  ['click', {}],
  ['click', { ctrlKey: true }],
  ['auxclick', { button: 1 }]
] as const)('leaves the whole card navigation native with %s %j', async (type, options) => {
  const open = vi.spyOn(window, 'open').mockReturnValue(null);
  const { container } = render(LinkHover, { href, children });
  container.classList.add('prose');
  hover(container);
  const card = await vi.waitFor(() => {
    const link = container.querySelector<HTMLAnchorElement>('[data-testid="link-hover-card"]');
    expect(link?.textContent).toContain('A useful guide');
    return link!;
  });
  expect(card.tagName).toBe('A');
  expect(card.href).toBe(href);
  expect(card.rel).toBe('noopener noreferrer');
  expect(card.dataset.contentHref).toBe(href);
  expect(getComputedStyle(card).textDecorationLine).toBe('none');
  let prevented: boolean | undefined;
  document.addEventListener(
    type,
    (event) => {
      prevented = event.defaultPrevented;
      event.preventDefault();
    },
    { once: true }
  );
  card
    .querySelector('.font-semibold')!
    .dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true, ...options }));
  expect(prevented).toBe(false);
  expect(open).not.toHaveBeenCalled();
  await vi.waitFor(() =>
    expect(container.querySelector('[data-testid="link-hover-card"]')).toBeNull()
  );
});

it('keeps keyboard focus in the card and returns it to the trigger on Escape', async () => {
  const { container } = render(LinkHover, { href, children, local: { name: 'Development' } });
  const trigger = container.querySelector('a')!;
  trigger.focus();
  const card = await vi.waitFor(() => {
    const link = container.querySelector<HTMLAnchorElement>('[data-testid="link-hover-card"]');
    expect(link).not.toBeNull();
    return link!;
  });
  card.focus();
  container.querySelector('[popover]')!.dispatchEvent(new MouseEvent('mouseleave'));
  await new Promise((resolve) => setTimeout(resolve, 250));
  expect(document.activeElement).toBe(card);
  expect(card.isConnected).toBe(true);
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
  await vi.waitFor(() => expect(card.isConnected).toBe(false));
  expect(document.activeElement).toBe(trigger);
  await new Promise((resolve) => setTimeout(resolve, 400));
  expect(container.querySelector('[data-testid="link-hover-card"]')).toBeNull();
});
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
