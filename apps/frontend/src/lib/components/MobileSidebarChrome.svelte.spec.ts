import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import { sidebarNav } from '$lib/state/globals.svelte';
import MobileSidebarChrome from './MobileSidebarChrome.svelte';
import ServerSidebar from './ServerSidebar.svelte';
import { cdp, page } from 'vitest/browser';
import { applyAnimationPreference, watchMotionPreference } from '$lib/dom/motionPreference';
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

function renderDrawer() {
  const view = renderChrome();
  render(ServerSidebar, {
    target: q(view.container, '[data-testid="sidebar-child"]')!,
    props: {
      children: testSnippet('<input aria-label="Navigation filter" value="Keep this filter">'),
      showCurrentUserBar: false
    }
  });
  return {
    ...view,
    panels: [...view.container.querySelectorAll<HTMLElement>('.sidebar-drawer')],
    field: view.container.querySelector('input')!
  };
}

function translation(element: HTMLElement) {
  return new DOMMatrixReadOnly(getComputedStyle(element).transform).m41;
}

describe('MobileSidebarChrome', () => {
  let stopMotionWatch: (() => void) | void;
  beforeEach(async () => {
    await page.viewport(390, 844);
    vi.clearAllMocks();
    document.documentElement.dir = 'ltr';
    resetSidebar();
    applyAnimationPreference('automatic');
    stopMotionWatch = watchMotionPreference(document.documentElement);
  });

  afterEach(async () => {
    stopMotionWatch?.();
    await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: false });
    await cdp().send('Emulation.setEmulatedMedia', { features: [] });
    applyAnimationPreference('automatic');
    document.documentElement.dir = 'ltr';
    await page.viewport(1280, 720);
  });

  it.each(
    [false, true].flatMap((touch) => ['ltr', 'rtl'].map((direction) => [touch, direction] as const))
  )('moves both columns together during dragging with touch=%s in %s', async (touch, direction) => {
    await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
    document.documentElement.dir = direction;
    const { panels, field } = renderDrawer();
    await expect.poll(() => sidebarNav.panelWidth).toBe(390);
    sidebarNav.startDrag();
    sidebarNav.updateDrag(195);
    flushSync();
    const offset = (touch ? 195 : 150) * (direction === 'ltr' ? -1 : 1);
    for (const panel of panels) {
      expect(translation(panel)).toBe(offset);
      expect(getComputedStyle(panel).opacity).toBe(touch ? '1' : '0.5');
      expect(panel.getAnimations()).toHaveLength(0);
    }
    sidebarNav.endDrag(1);
    flushSync();
    await expect.poll(() => translation(panels[1])).toBe(0);
    field.focus();
    expect(document.activeElement).toBe(field);
    sidebarNav.close();
    flushSync();
    expect(panels.every((panel) => panel.inert)).toBe(true);
    field.blur();
    field.focus();
    expect(document.activeElement).not.toBe(field);
    await expect.poll(() => getComputedStyle(panels[1]).visibility).toBe('hidden');
  });

  it.each([false, true])(
    'skips motion and delayed visibility with reduced motion and touch=%s',
    async (touch) => {
      await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
      await cdp().send('Emulation.setEmulatedMedia', {
        features: [{ name: 'prefers-reduced-motion', value: 'reduce' }]
      });
      expect(matchMedia('(prefers-reduced-motion: reduce)').matches).toBe(true);
      await expect.poll(() => document.documentElement.dataset.motion).toBe('reduced');
      const { panels } = renderDrawer();
      for (const open of [true, false, true, false]) {
        if (open) sidebarNav.toggle();
        else sidebarNav.close();
        flushSync();
        for (const panel of panels) {
          expect(getComputedStyle(panel).visibility).toBe(open ? 'visible' : 'hidden');
          expect(translation(panel)).toBe(open ? 0 : touch ? -390 : -300);
          expect(panel.getAnimations()).toHaveLength(0);
        }
      }
    }
  );

  it.each([false, true])(
    'reverses an unfinished close without replacing content with touch=%s',
    async (touch) => {
      await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
      const { panels, field } = renderDrawer();
      sidebarNav.toggle();
      flushSync();
      await expect.poll(() => translation(panels[1])).toBe(0);
      sidebarNav.close();
      flushSync();
      // Seek real CSS transitions rather than relying on wall-clock sleeps.
      for (const panel of panels) {
        const animations = panel.getAnimations();
        expect(animations.length).toBeGreaterThan(0);
        for (const animation of animations) {
          animation.pause();
          animation.currentTime = 80;
        }
      }
      const before = translation(panels[1]);
      expect(before).toBeLessThan(0);
      expect(before).toBeGreaterThan(touch ? -390 : -300);
      sidebarNav.toggle();
      flushSync();
      expect(getComputedStyle(panels[1]).visibility).toBe('visible');
      await expect.poll(() => translation(panels[1])).toBe(0);
      expect(field.isConnected).toBe(true);
      expect(field.value).toBe('Keep this filter');
      field.focus();
      expect(document.activeElement).toBe(field);
    }
  );

  it('preserves an active drag through capability changes and cancels it at the pane breakpoint', async () => {
    const stopTracking = sidebarNav.initViewportTracking();
    const { panels, field } = renderDrawer();
    try {
      await expect.poll(() => sidebarNav.panelWidth).toBe(390);
      sidebarNav.startDrag();
      sidebarNav.updateDrag(195);
      flushSync();
      for (const touch of [true, false, true]) {
        await cdp().send('Emulation.setTouchEmulationEnabled', { enabled: touch });
        expect(sidebarNav.progress).toBe(0.5);
        expect(translation(panels[1])).toBe(touch ? -195 : -150);
        expect(panels[1].getAnimations()).toHaveLength(0);
      }
      await page.viewport(768, 844);
      await expect.poll(() => sidebarNav.isMobile).toBe(false);
      expect(sidebarNav.dragOffset).toBeNull();
      expect(sidebarNav.isOpen).toBe(true);
      expect(getComputedStyle(panels[1]).transform).toBe('none');
      expect(field.value).toBe('Keep this filter');
      await page.viewport(767, 844);
      await expect.poll(() => sidebarNav.isMobile).toBe(true);
      expect(sidebarNav.isOpen).toBe(false);
      expect(panels[1].inert).toBe(true);
    } finally {
      stopTracking();
    }
  });

  it.each(['ltr', 'rtl'])(
    'leaves a 3rem backdrop strip and follows resizing in %s',
    async (direction) => {
      document.documentElement.dir = direction;
      await page.viewport(390, 844);
      const { container } = renderChrome();
      const { container: navigation } = render(ServerSidebar, {
        target: q(container, '[data-testid="sidebar-child"]')!,
        props: { children: testSnippet('<nav>Navigation</nav>'), showCurrentUserBar: false }
      });
      sidebarNav.toggle();
      flushSync();
      const gutter = q(container, '[data-testid="mobile-sidebar-panel"]')!;
      const pane = q(navigation, '[data-testid="server-sidebar"]')!;
      const underlyingAction = document.createElement('button');
      underlyingAction.textContent = 'Underlying action';
      underlyingAction.style.cssText = 'position: fixed; inset: 0';
      const onUnderlyingClick = vi.fn();
      underlyingAction.addEventListener('click', onUnderlyingClick);
      q(container, '[data-testid="sidebar-child"]')!.append(underlyingAction);
      // Disable transitions so assertions measure the settled drawer geometry.
      gutter.style.transition = 'none';
      pane.style.transition = 'none';
      try {
        for (const width of [390, 320, 767]) {
          await page.viewport(width, 844);
          await expect.poll(() => sidebarNav.panelWidth).toBe(width);
          expect(gutter.getBoundingClientRect().width + pane.getBoundingClientRect().width).toBe(
            width - 48
          );
          expect(
            Math.min(gutter.getBoundingClientRect().left, pane.getBoundingClientRect().left)
          ).toBe(direction === 'ltr' ? 0 : 48);
          expect(
            Math.max(gutter.getBoundingClientRect().right, pane.getBoundingClientRect().right)
          ).toBe(direction === 'ltr' ? width - 48 : width);
        }
        // A real pointer click must hit the exposed backdrop, not the client below it.
        const x = direction === 'ltr' ? 743 : 24;
        expect(document.elementFromPoint(x, 200)).toBe(
          q(container, '[data-testid="mobile-sidebar-backdrop"]')
        );
        await page.getByTestId('mobile-sidebar-backdrop').click({ position: { x, y: 100 } });
        expect(sidebarNav.isOpen).toBe(false);
        expect(onUnderlyingClick).not.toHaveBeenCalled();
        flushSync();
        // Mouse drawers fade after a short slide rather than crossing the full window.
        expect(getComputedStyle(pane).opacity).toBe('0');
        expect(getComputedStyle(pane).transform).toBe(
          `matrix(1, 0, 0, 1, ${direction === 'ltr' ? -300 : 300}, 0)`
        );
        await expect.poll(() => getComputedStyle(pane).visibility).toBe('hidden');
      } finally {
        document.documentElement.dir = 'ltr';
        await page.viewport(1280, 720);
      }
    }
  );

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
        target: q(container, '[data-testid="sidebar-child"]')!,
        props: { children: testSnippet('<nav>Navigation</nav>'), showCurrentUserBar: false }
      });
      sidebarNav.toggle();
      flushSync();
      for (const panel of [
        q(container, '[data-testid="mobile-sidebar-panel"]')!,
        q(container, '[data-testid="mobile-sidebar-backdrop"]')!,
        q(navigation, '[data-testid="server-sidebar"]')!
      ]) {
        // Mouse windows retain the compact 44 px header even below md.
        expect(panel.getBoundingClientRect().top).toBe(106);
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

    expect(panel.inert).toBe(true);
    expect(panel.classList.contains('max-md:start-0')).toBe(true);
    expect(getComputedStyle(panel).transform).toBe('matrix(1, 0, 0, 1, -300, 0)');
    expect(backdrop.disabled).toBe(true);
    expect(backdrop.getAttribute('aria-hidden')).toBe('true');
    expect(backdrop.style.opacity).toBe('0');
  });

  it('opens and closes from the backdrop state without unmounting it', async () => {
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

    expect(panel.inert).toBe(false);
    await expect.poll(() => getComputedStyle(panel).transform).toBe('matrix(1, 0, 0, 1, 0, 0)');
    expect(backdrop.disabled).toBe(false);
    expect(backdrop.style.opacity).toBe('1');

    backdrop.click();
    flushSync();

    expect(q(container, '[data-testid="mobile-sidebar-backdrop"]')).toBe(backdrop);
    expect(panel.inert).toBe(true);
    await expect.poll(() => getComputedStyle(panel).visibility).toBe('hidden');
    expect(getComputedStyle(panel).opacity).toBe('0');
    expect(backdrop.disabled).toBe(true);
    expect(backdrop.style.opacity).toBe('0');
  });
});
