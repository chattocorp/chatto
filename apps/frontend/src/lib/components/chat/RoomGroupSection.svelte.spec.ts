import { prefersReducedMotion } from 'svelte/motion';
import { render } from 'vitest-browser-svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { q, testSnippet } from '$lib/test-utils';
import RoomGroupSection from './RoomGroupSection.svelte';

const items = [{ id: 'general' }, { id: 'announcements' }];

beforeEach(() => localStorage.clear());
afterEach(() => vi.restoreAllMocks());

describe('RoomGroupSection', () => {
  it('preserves the drag highlight when an empty group receives an item', async () => {
    const { container, rerender } = render(RoomGroupSection, {
      label: 'Drop target',
      items: [],
      item: testSnippet('<span>Room</span>'),
      persistKey: 'test:room-group-section:drag-highlight',
      itemsAttachment: (node: HTMLDivElement) => {
        node.classList.add('sidebar-drop-target-active');
      }
    });
    const dropzone = q(container, '[data-testid="room-group-items-dropzone"]')!;
    await expect.element(dropzone).toHaveClass('sidebar-drop-target-active');
    await rerender({ items: [{ id: 'dragged-room' }] });
    await expect.element(dropzone).not.toHaveClass('min-h-8');
    await expect.element(dropzone).toHaveClass('sidebar-drop-target-active');
    await rerender({ items: [] });
    await expect.element(dropzone).toHaveClass('min-h-8');
    await expect.element(dropzone).toHaveClass('sidebar-drop-target-active');
  });

  it('persists its collapsed state and keeps highlighted entries visible', async () => {
    const persistKey = 'test:room-group-section:collapse';
    const { container } = render(RoomGroupSection, {
      props: {
        label: 'Community',
        items,
        item: testSnippet('<span data-testid="room-group-entry">Room</span>'),
        persistKey,
        testid: 'room-group-heading',
        keepVisibleWhenCollapsed: (entry) => entry.id === 'announcements'
      }
    });

    const toggle = q(container, '[data-testid="room-group-heading"]');
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(container.querySelectorAll('[data-testid="room-group-entry"]')).toHaveLength(2);

    toggle?.click();

    await expect.element(toggle).toHaveAttribute('aria-expanded', 'false');
    await expect
      .poll(() => container.querySelectorAll('[data-testid="room-group-entry"]').length)
      .toBe(1);
    expect(localStorage.getItem(persistKey)).toBe('1');
  });

  it('keeps free-form content mounted and inert while collapsed', async () => {
    const { container } = render(RoomGroupSection, {
      props: {
        label: 'Bio',
        items: [],
        content: testSnippet('<p data-testid="bio-content">Development assistant</p>'),
        persistKey: 'test:room-group-section:content',
        testid: 'bio-toggle'
      }
    });
    const content = q(container, '[data-testid="bio-content"]');
    const toggle = q(container, '[data-testid="bio-toggle"]');
    toggle?.click();
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(q(container, '[data-testid="bio-content"]')).toBe(content);
    expect(content?.closest('[inert]')).not.toBeNull();
    toggle?.click();
    await expect.element(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(q(container, '[data-testid="bio-content"]')).toBe(content);
    expect(content?.closest('[inert]')).toBeNull();
  });

  it('draws a full-width divider when it follows another room group', () => {
    const { container } = render(RoomGroupSection, {
      props: {
        label: 'Community',
        items,
        item: testSnippet('<span>Room</span>'),
        persistKey: 'test:room-group-section:divider',
        separated: true
      }
    });

    const section = q(container, '[data-testid="room-group-section"]');
    expect(section?.classList).toContain('border-t');
    expect(section?.classList).toContain('border-border');
  });

  it('mirrors its collapsed inline-end disclosure icon in RTL', () => {
    const { container } = render(RoomGroupSection, {
      props: {
        label: 'Community',
        items,
        item: testSnippet('<span>Room</span>'),
        persistKey: 'test:room-group-section:rtl',
        defaultCollapsed: true
      }
    });

    const icon = q(container, '.iconify');
    expect(icon?.classList).toContain('icon-[uil--angle-right-b]');
    expect(icon?.classList).toContain('rtl:-scale-x-100');
    expect(icon?.classList).not.toContain('rotate-90');
  });

  it('renders an attached drop target for an expanded empty group', async () => {
    const itemsAttachment = vi.fn();
    const { container } = render(RoomGroupSection, {
      props: {
        label: 'Empty',
        items: [],
        item: testSnippet('<span>Room</span>'),
        persistKey: 'test:room-group-section:empty-drop-target',
        itemsAttachment
      }
    });

    const dropzone = q(container, '[data-testid="room-group-items-dropzone"]');
    await expect.element(dropzone).toBeInTheDocument();
    expect(dropzone?.classList).toContain('min-h-8');
    expect(dropzone?.classList).toContain('sidebar-drop-target');
    expect(itemsAttachment).toHaveBeenCalledOnce();
  });
});

// Observe actual browser animation keyframes, not only the final collapsed DOM.
describe('section motion', () => {
  it.each([false, true])('slides the whole collection with a drop target: %s', async (attached) => {
    const animate = vi.spyOn(Element.prototype, 'animate');
    const { container } = render(RoomGroupSection, {
      label: 'Animated',
      items,
      item: testSnippet('<div style="height: 40px">Room</div>'),
      persistKey: `test:room-group-motion:${attached}`,
      itemsAttachment: attached ? () => {} : undefined
    });
    const toggle = q(container, 'button')!;
    // Ignore initial component intros. This check targets a user disclosure action.
    await Promise.all(
      container.getAnimations({ subtree: true }).map((animation) => animation.finished)
    );
    animate.mockClear();
    toggle.click();
    await expect
      .poll(() =>
        animate.mock.calls.some(
          ([frames]) => Array.isArray(frames) && frames.some((frame) => 'height' in frame)
        )
      )
      .toBe(true);
    await expect.poll(() => container.textContent?.includes('Room')).toBe(false);
    animate.mockClear();
    toggle.click();
    await expect
      .poll(() =>
        animate.mock.calls.some(
          ([frames]) => Array.isArray(frames) && frames.some((frame) => 'height' in frame)
        )
      )
      .toBe(true);
    animate.mockRestore();
  });
});

it('slides hidden draggable rows while a highlighted row stays visible', async () => {
  const animate = vi.spyOn(Element.prototype, 'animate');
  const { container } = render(RoomGroupSection, {
    label: 'Highlighted',
    items,
    item: testSnippet('<div style="height: 40px" data-room>Room</div>'),
    persistKey: 'test:room-group-motion:highlighted',
    itemsAttachment: () => {},
    keepVisibleWhenCollapsed: (entry) => entry.id === 'general'
  });
  await Promise.all(
    container.getAnimations({ subtree: true }).map((animation) => animation.finished)
  );
  animate.mockClear();
  q(container, 'button')!.click();
  await expect
    .poll(() =>
      animate.mock.calls.some(
        ([frames]) => Array.isArray(frames) && frames.some((frame) => 'height' in frame)
      )
    )
    .toBe(true);
  await expect.poll(() => container.querySelectorAll('[data-room]').length).toBe(1);
});

it('slides a footer-only section and can reverse an unfinished collapse', async () => {
  const { container } = render(RoomGroupSection, {
    label: 'Footer',
    items: [],
    item: testSnippet(''),
    footer: testSnippet('<div style="height: 120px" data-footer>More rooms</div>'),
    persistKey: 'test:room-group-motion:footer'
  });
  await Promise.all(
    container.getAnimations({ subtree: true }).map((animation) => animation.finished)
  );
  const toggle = q(container, 'button')!;
  toggle.click();
  await expect
    .poll(() =>
      container
        .getAnimations({ subtree: true })
        .some((animation) =>
          (animation.effect as KeyframeEffect).getKeyframes().some((frame) => 'height' in frame)
        )
    )
    .toBe(true);
  toggle.click();
  await expect.element(toggle).toHaveAttribute('aria-expanded', 'true');
  await expect.element(q(container, '[data-footer]')).toBeVisible();
  await Promise.all(
    container
      .getAnimations({ subtree: true })
      .map((animation) => animation.finished.catch(() => {}))
  );
  await expect.element(q(container, '[data-footer]')).toBeVisible();
});

it('skips spatial animations when reduced motion is requested', async () => {
  vi.spyOn(prefersReducedMotion, 'current', 'get').mockReturnValue(true);
  const { container } = render(RoomGroupSection, {
    label: 'Reduced',
    items,
    item: testSnippet('<div style="height: 40px" data-room>Room</div>'),
    persistKey: 'test:room-group-motion:reduced'
  });
  q(container, 'button')!.click();
  await expect.poll(() => container.querySelectorAll('[data-room]').length).toBe(0);
  expect(
    container
      .getAnimations({ subtree: true })
      .filter(
        (animation) =>
          Number(animation.effect?.getTiming().duration) > 0 &&
          (animation.effect as KeyframeEffect).getKeyframes().some((frame) => 'height' in frame)
      )
  ).toHaveLength(0);
});
