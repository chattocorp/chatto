import { render } from 'vitest-browser-svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { q, testSnippet } from '$lib/test-utils';
import RoomGroupSection from './RoomGroupSection.svelte';

const items = [{ id: 'general' }, { id: 'announcements' }];

beforeEach(() => localStorage.clear());

describe('RoomGroupSection', () => {
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
    expect(itemsAttachment).toHaveBeenCalledOnce();
  });
});
