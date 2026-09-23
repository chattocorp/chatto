import { beforeEach, describe, expect, it } from 'vitest';
import { SidebarNavState } from './globals.svelte';

describe('SidebarNavState', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('defaults the desktop sidebar to open for a fresh session', () => {
    const sidebar = new SidebarNavState();

    expect(sidebar.isOpen).toBe(true);
  });

  it('tracks finger movement across the full viewport and settles at halfway', () => {
    const sidebar = new SidebarNavState();
    sidebar.setMobile(true);
    sidebar.startDrag();
    sidebar.updateDrag(window.innerWidth * 0.4);
    expect(sidebar.progress).toBeCloseTo(0.4);
    sidebar.endDrag(0);
    expect(sidebar.isOpen).toBe(false);

    sidebar.startDrag();
    sidebar.updateDrag(window.innerWidth * 0.6);
    sidebar.endDrag(0);
    expect(sidebar.isOpen).toBe(true);

    sidebar.startDrag();
    sidebar.updateDrag(-window.innerWidth * 0.6);
    expect(sidebar.progress).toBeCloseTo(0.4);
    sidebar.endDrag(0);
    expect(sidebar.isOpen).toBe(false);
  });

  it('remembers desktop toggles for the current app session', () => {
    const sidebar = new SidebarNavState(true);

    sidebar.toggle();
    expect(sidebar.isOpen).toBe(false);

    sidebar.toggle();
    expect(sidebar.isOpen).toBe(true);
  });

  it('uses the measured work plane for swipe distance and resets on unmount', () => {
    const sidebar = new SidebarNavState();
    sidebar.setMobile(true);
    sidebar.setPanelWidth(376);
    sidebar.startDrag();
    sidebar.updateDrag(188);
    expect(sidebar.panelWidth).toBe(376);
    expect(sidebar.progress).toBe(0.5);
    sidebar.endDrag(0);
    expect(sidebar.isOpen).toBe(true);
    sidebar.setPanelWidth(null);
    expect(sidebar.panelWidth).toBe(window.innerWidth);
  });

  it('does not persist mobile overlay open and close changes', () => {
    const sidebar = new SidebarNavState();

    sidebar.setMobile(true);
    expect(sidebar.isOpen).toBe(false);

    sidebar.toggle();
    expect(sidebar.isOpen).toBe(true);

    sidebar.close();
    expect(sidebar.isOpen).toBe(false);

    sidebar.setMobile(false);
    expect(sidebar.isOpen).toBe(true);
  });

  it('restores a closed desktop preference after mobile use', () => {
    const sidebar = new SidebarNavState();

    sidebar.toggle();
    expect(sidebar.isOpen).toBe(false);

    sidebar.setMobile(true);
    sidebar.toggle();
    expect(sidebar.isOpen).toBe(true);

    sidebar.setMobile(false);
    expect(sidebar.isOpen).toBe(false);
  });
});
