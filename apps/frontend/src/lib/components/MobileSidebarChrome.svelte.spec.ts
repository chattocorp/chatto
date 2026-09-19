import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import { sidebarNav } from '$lib/state/globals.svelte';
import MobileSidebarChrome from './MobileSidebarChrome.svelte';
import ServerSidebar from './ServerSidebar.svelte';
import { page } from 'vitest/browser';
import '../../app.css';

function resetSidebar() {
  sidebarNav.setMobile(false);
  if (!sidebarNav.isOpen) sidebarNav.toggle();
  sidebarNav.setMobile(true);
}

function renderChrome() {
  return render(MobileSidebarChrome, {
    props: {
      children: testSnippet('<main data-testid="sidebar-child"></main>')
    }
  });
}

describe('MobileSidebarChrome', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    document.documentElement.dir = 'ltr';
    resetSidebar();
  });

  it.each(['ltr', 'rtl'])('fills the mobile viewport and follows resizing in %s', async (direction) => {
    document.documentElement.dir = direction;
    await page.viewport(390, 844);
    const { container } = renderChrome();
    const { container: navigation } = render(ServerSidebar, {
      props: { children: testSnippet('<nav>Navigation</nav>'), showCurrentUserBar: false }
    });
    sidebarNav.toggle();
    flushSync();
    const gutter = q(container, '[data-testid="mobile-sidebar-panel"]')!;
    const pane = q(navigation, '[data-testid="server-sidebar"]')!;
    // Disable transitions so assertions measure the settled drawer geometry.
    gutter.style.transition = 'none';
    pane.style.transition = 'none';
    try {
      for (const width of [390, 320, 767]) {
        await page.viewport(width, 844);
        await expect.poll(() => sidebarNav.panelWidth).toBe(width);
        expect(gutter.getBoundingClientRect().width + pane.getBoundingClientRect().width).toBe(width);
        expect(Math.min(gutter.getBoundingClientRect().left, pane.getBoundingClientRect().left)).toBe(0);
        expect(Math.max(gutter.getBoundingClientRect().right, pane.getBoundingClientRect().right)).toBe(width);
      }
      sidebarNav.close();
      flushSync();
      const rect = pane.getBoundingClientRect();
      expect(direction === 'ltr' ? rect.right <= 0 : rect.left >= 767).toBe(true);
    } finally {
      document.documentElement.dir = 'ltr';
      await page.viewport(1280, 720);
    }
  });

  it('renders the gutter panel and children in the sidebar row', () => {
    const { container } = renderChrome();

    expect(q(container, '[data-testid="mobile-sidebar-panel"]')).not.toBeNull();
    expect(q(container, '[data-testid="sidebar-child"]')).not.toBeNull();
    expect(q(container, '[data-testid="mobile-sidebar-edge"]')).toBeNull();
  });

  it('keeps both columns and the backdrop inside the device safe areas', async () => {
    await page.viewport(390, 844);
    document.documentElement.style.setProperty('--mobile-sidebar-safe-top', '62px');
    document.documentElement.style.setProperty('--mobile-sidebar-safe-bottom', '34px');
    try {
      const { container } = renderChrome();
      const { container: navigation } = render(ServerSidebar, {
        props: { children: testSnippet('<nav>Navigation</nav>'), showCurrentUserBar: false }
      });
      sidebarNav.toggle();
      flushSync();
      for (const panel of [
        q(container, '[data-testid="mobile-sidebar-panel"]')!,
        q(container, '[data-testid="mobile-sidebar-backdrop"]')!,
        q(navigation, '[data-testid="server-sidebar"]')!
      ]) {
        expect(panel.getBoundingClientRect().top).toBe(118);
        expect(panel.getBoundingClientRect().bottom).toBe(810);
      }
    } finally {
      document.documentElement.style.removeProperty('--mobile-sidebar-safe-top');
      document.documentElement.style.removeProperty('--mobile-sidebar-safe-bottom');
      await page.viewport(1280, 720);
    }
  });

  it('marks mobile sidebar chrome as closed when the sidebar is closed', () => {
    const { container } = renderChrome();

    const panel = q(container, '[data-testid="mobile-sidebar-panel"]');
    const backdrop = q(
      container,
      '[data-testid="mobile-sidebar-backdrop"]'
    ) as HTMLButtonElement | null;
    expect(panel).not.toBeNull();
    expect(backdrop).not.toBeNull();
    if (!panel || !backdrop) return;

    expect(panel.classList.contains('sidebar-mobile-closed')).toBe(true);
    expect(panel.classList.contains('max-md:start-0')).toBe(true);
    expect(panel.style.transform).toBe(`translateX(calc(-${window.innerWidth}px * var(--inline-direction)))`);
    expect(backdrop.disabled).toBe(true);
    expect(backdrop.getAttribute('aria-hidden')).toBe('true');
    expect(backdrop.style.opacity).toBe('0');
  });

  it('opens and closes from the backdrop state without unmounting it', () => {
    const { container } = renderChrome();

    sidebarNav.toggle();
    flushSync();

    const panel = q(container, '[data-testid="mobile-sidebar-panel"]');
    const backdrop = q(
      container,
      '[data-testid="mobile-sidebar-backdrop"]'
    ) as HTMLButtonElement | null;
    expect(panel).not.toBeNull();
    expect(backdrop).not.toBeNull();
    if (!panel || !backdrop) return;

    expect(panel.classList.contains('sidebar-mobile-closed')).toBe(false);
    expect(panel.style.transform).toBe('translateX(calc(0px * var(--inline-direction)))');
    expect(backdrop.disabled).toBe(false);
    expect(backdrop.style.opacity).toBe('1');

    backdrop.click();
    flushSync();

    expect(q(container, '[data-testid="mobile-sidebar-backdrop"]')).toBe(backdrop);
    expect(panel.classList.contains('sidebar-mobile-closed')).toBe(true);
    expect(panel.style.transform).toBe(`translateX(calc(-${window.innerWidth}px * var(--inline-direction)))`);
    expect(backdrop.disabled).toBe(true);
    expect(backdrop.style.opacity).toBe('0');
  });
});
