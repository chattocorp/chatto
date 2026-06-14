import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import RoomSidebarToggle from './RoomSidebarToggle.svelte';

describe('RoomSidebarToggle', () => {
  it('opens the members panel when it is hidden', async () => {
    const onToggle = vi.fn();
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: null,
        onToggle
      }
    });

    const button = container.querySelector(
      '[aria-label="Show members"]'
    ) as HTMLButtonElement | null;
    expect(button).toBeTruthy();

    button!.click();
    await tick();

    expect(onToggle).toHaveBeenCalledWith('members');
  });

  it('uses the hide label while the members panel is active', async () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: 'members',
        onToggle: vi.fn()
      }
    });

    expect(container.querySelector('[aria-label="Hide members"]')).toBeTruthy();
  });

  it('switches to the files panel', async () => {
    const onToggle = vi.fn();
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: 'members',
        onToggle
      }
    });

    const button = container.querySelector('[aria-label="Show files"]') as HTMLButtonElement | null;
    expect(button).toBeTruthy();

    button!.click();
    await tick();

    expect(onToggle).toHaveBeenCalledWith('files');
  });

  it('uses a background-only pressed state for the active panel', async () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: 'files',
        onToggle: vi.fn()
      }
    });

    const filesButton = container.querySelector(
      '[aria-label="Hide files"]'
    ) as HTMLButtonElement | null;
    const membersButton = container.querySelector(
      '[aria-label="Show members"]'
    ) as HTMLButtonElement | null;
    expect(filesButton).toBeTruthy();
    expect(membersButton).toBeTruthy();
    expect(filesButton!.getAttribute('aria-pressed')).toBe('true');
    expect(membersButton!.getAttribute('aria-pressed')).toBe('false');
    expect(filesButton!.classList.contains('pane-header-icon-button-active')).toBe(true);
    expect(membersButton!.classList.contains('pane-header-icon-button')).toBe(true);
    expect(membersButton!.classList.contains('pane-header-icon-button-active')).toBe(false);
  });

  it('renders desktop-only by default', async () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: null,
        onToggle: vi.fn()
      }
    });

    const group = container.querySelector('[data-testid="room-sidebar-toggle"]');
    expect(group).toBeTruthy();
    expect(group!.classList.contains('hidden')).toBe(true);
    expect(group!.classList.contains('lg:inline-flex')).toBe(true);
  });

  it('can render as a mobile-only toggle group', async () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: null,
        onToggle: vi.fn(),
        mode: 'mobile'
      }
    });

    const group = container.querySelector('[data-testid="room-sidebar-toggle"]');
    expect(group).toBeTruthy();
    expect(group!.classList.contains('inline-flex')).toBe(true);
    expect(group!.classList.contains('lg:hidden')).toBe(true);
    expect(group!.classList.contains('hidden')).toBe(false);
  });
});
