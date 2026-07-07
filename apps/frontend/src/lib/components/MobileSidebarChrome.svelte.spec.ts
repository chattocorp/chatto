import { flushSync } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import { sidebarNav } from '$lib/state/globals.svelte';
import MobileSidebarChrome from './MobileSidebarChrome.svelte';

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

function pointer(type: string, x: number, y = 120) {
  return new PointerEvent(type, {
    bubbles: true,
    cancelable: true,
    pointerId: 1,
    clientX: x,
    clientY: y
  });
}

describe('MobileSidebarChrome', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetSidebar();
  });

  it('renders the gutter panel and children in the sidebar row', () => {
    const { container } = renderChrome();

    expect(q(container, '[data-testid="mobile-sidebar-panel"]')).not.toBeNull();
    expect(q(container, '[data-testid="sidebar-child"]')).not.toBeNull();
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
    expect(panel.style.transform).toBe('translateX(-324px)');
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
    expect(panel.style.transform).toBe('translateX(0px)');
    expect(backdrop.disabled).toBe(false);
    expect(backdrop.style.opacity).toBe('1');

    backdrop.click();
    flushSync();

    expect(q(container, '[data-testid="mobile-sidebar-backdrop"]')).toBe(backdrop);
    expect(panel.classList.contains('sidebar-mobile-closed')).toBe(true);
    expect(panel.style.transform).toBe('translateX(-324px)');
    expect(backdrop.disabled).toBe(true);
    expect(backdrop.style.opacity).toBe('0');
  });

  it('keeps edge target pointer events from bubbling to window handlers', () => {
    const { container } = renderChrome();
    const onWindowPointerDown = vi.fn();
    window.addEventListener('pointerdown', onWindowPointerDown);

    try {
      const edge = q(container, '[data-testid="mobile-sidebar-edge"]');
      expect(edge).not.toBeNull();
      if (!edge) return;

      edge.dispatchEvent(pointer('pointerdown', 2));

      expect(onWindowPointerDown).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener('pointerdown', onWindowPointerDown);
    }
  });
});
