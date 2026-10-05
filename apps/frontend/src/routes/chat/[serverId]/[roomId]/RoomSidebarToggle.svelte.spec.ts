import '../../../../app.css';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { tick } from 'svelte';
import { loadLocaleMessages } from '$lib/i18n/messages';
import { setReactiveLocale } from '$lib/i18n/state.svelte';
import RoomSidebarToggle from './RoomSidebarToggle.svelte';

describe('RoomSidebarToggle', () => {
  beforeEach(async () => {
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
  });

  it('opens the members panel when it is hidden', async () => {
    const onToggle = vi.fn();
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: null,
        onToggle
      }
    });

    const button = container.querySelector('[aria-label="Members"]') as HTMLButtonElement | null;
    expect(button).toBeTruthy();

    button!.click();
    await tick();

    expect(onToggle).toHaveBeenCalledWith('members');
  });

  it('keeps a stable name and reports the open panel as pressed', async () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: 'members',
        onToggle: vi.fn()
      }
    });

    const button = container.querySelector('[aria-label="Members"]');
    expect(button?.getAttribute('aria-pressed')).toBe('true');
    expect(button?.getAttribute('title')).toBe('Members');
  });

  it.each(['en-GB', 'de-DE'] as const)(
    'shows the labels only when the pane header has room in %s',
    async (locale) => {
      await loadLocaleMessages(locale);
      setReactiveLocale(locale);
      const { container } = render(RoomSidebarToggle, {
        props: { activePanel: null, mode: 'always', onToggle: vi.fn() }
      });
      const header = document.createElement('div');
      header.className = '@container/pane-header';
      container.parentElement!.insertBefore(header, container);
      header.append(container);
      const labels = [...container.querySelectorAll('button > span:last-child')];

      header.style.width = '79.99rem';
      await expect
        .poll(() => labels.every((label) => getComputedStyle(label).display === 'none'))
        .toBe(true);

      header.style.width = '80rem';
      await expect
        .poll(() => labels.every((label) => getComputedStyle(label).display !== 'none'))
        .toBe(true);
    }
  );

  it('switches to the files panel', async () => {
    const onToggle = vi.fn();
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: 'members',
        onToggle
      }
    });

    const button = container.querySelector('[aria-label="Files"]') as HTMLButtonElement | null;
    expect(button).toBeTruthy();

    button!.click();
    await tick();

    expect(onToggle).toHaveBeenCalledWith('files');
  });

  it('puts the outlined pin control first and uses the notification colour for unseen pins', () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: null,
        panels: ['members', 'pins'],
        hasUnseenPins: true,
        onToggle: vi.fn(),
        mode: 'always'
      }
    });

    const buttons = container.querySelectorAll('button');
    expect(buttons[0]?.getAttribute('aria-label')).toContain('Pins');
    expect(buttons[0]?.querySelector('.pane-header-icon-glyph')?.className).toContain(
      'pin-outline'
    );
    expect(buttons[0]?.querySelector('[data-testid="unseen-pin-dot"]')?.classList).toContain(
      'bg-attention'
    );
  });

  it('switches to room-scoped search', async () => {
    const onToggle = vi.fn();
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: 'members',
        onToggle
      }
    });

    const button = container.querySelector(
      '[aria-label="Search in this room"]'
    ) as HTMLButtonElement | null;
    expect(button).toBeTruthy();

    button!.click();
    await tick();

    expect(onToggle).toHaveBeenCalledWith('search');
  });

  it('updates the search label when the active locale changes', async () => {
    await loadLocaleMessages('en-GB');
    setReactiveLocale('en-GB');
    const { container } = render(RoomSidebarToggle, {
      props: { activePanel: null, onToggle: vi.fn() }
    });

    expect(container.querySelector('[aria-label="Search in this room"]')).toBeTruthy();
    await loadLocaleMessages('de-DE');
    setReactiveLocale('de-DE');
    await tick();

    expect(container.querySelector('[aria-label="In diesem Raum suchen"]')).toBeTruthy();
    setReactiveLocale('en-GB');
  });

  it('switches to the call panel', async () => {
    const onToggle = vi.fn();
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: 'members',
        onToggle
      }
    });

    const button = container.querySelector('[aria-label="Call"]') as HTMLButtonElement | null;
    expect(button).toBeTruthy();

    button!.click();
    await tick();

    expect(onToggle).toHaveBeenCalledWith('call');
  });

  it('can render only the files panel', async () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: null,
        panels: ['files'],
        onToggle: vi.fn()
      }
    });

    expect(container.querySelector('[aria-label="Members"]')).toBeFalsy();
    expect(container.querySelector('[aria-label="Files"]')).toBeTruthy();
  });

  it('can render only files and call panels', async () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: null,
        panels: ['files', 'call'],
        onToggle: vi.fn()
      }
    });

    expect(container.querySelector('[aria-label="Members"]')).toBeFalsy();
    expect(container.querySelector('[aria-label="Files"]')).toBeTruthy();
    expect(container.querySelector('[aria-label="Call"]')).toBeTruthy();
  });

  it('uses a background-only pressed state for the active panel', async () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: 'files',
        onToggle: vi.fn()
      }
    });

    const filesButton = container.querySelector('[aria-label="Files"]') as HTMLButtonElement | null;
    const membersButton = container.querySelector(
      '[aria-label="Members"]'
    ) as HTMLButtonElement | null;
    expect(filesButton).toBeTruthy();
    expect(membersButton).toBeTruthy();
    expect(filesButton!.getAttribute('aria-pressed')).toBe('true');
    expect(membersButton!.getAttribute('aria-pressed')).toBe('false');
    expect(filesButton!.classList.contains('pane-header-icon-button-active')).toBe(true);
    expect(membersButton!.classList.contains('pane-header-label-button')).toBe(true);
    expect(membersButton!.classList.contains('pane-header-icon-button-active')).toBe(false);
  });

  it('highlights and pulses the call tab when a call is active in the room', async () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: 'members',
        hasActiveCall: true,
        onToggle: vi.fn()
      }
    });

    const callButton = container.querySelector('[aria-label="Call"]') as HTMLButtonElement | null;

    expect(callButton).toBeTruthy();
    expect(callButton!.classList.contains('text-action')).toBe(true);
    expect(callButton!.querySelector('[data-testid="active-call-pulse-icon"]')).toBeTruthy();
  });

  it('keeps the active call tab highlighted without the pulse twin when selected', async () => {
    const { container } = render(RoomSidebarToggle, {
      props: {
        activePanel: 'call',
        hasActiveCall: true,
        onToggle: vi.fn()
      }
    });

    const callButton = container.querySelector('[aria-label="Call"]') as HTMLButtonElement | null;

    expect(callButton).toBeTruthy();
    expect(callButton!.classList.contains('text-action')).toBe(true);
    expect(callButton!.classList.contains('pane-header-icon-button-active')).toBe(true);
    expect(callButton!.querySelector('[data-testid="active-call-pulse-icon"]')).toBeFalsy();
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
