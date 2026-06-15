import { beforeEach, describe, expect, it } from 'vitest';
import { getMainSidebarOpen, setMainSidebarOpen } from '$lib/storage/mainSidebarOpen';
import { SidebarNavState } from './globals.svelte';

describe('SidebarNavState', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('hydrates the desktop open state from storage', () => {
    setMainSidebarOpen(false);

    const sidebar = new SidebarNavState();

    expect(sidebar.isOpen).toBe(false);
  });

  it('persists desktop toggles', () => {
    const sidebar = new SidebarNavState(true);

    sidebar.toggle();
    expect(sidebar.isOpen).toBe(false);
    expect(getMainSidebarOpen()).toBe(false);

    sidebar.toggle();
    expect(sidebar.isOpen).toBe(true);
    expect(getMainSidebarOpen()).toBe(true);
  });

  it('does not persist mobile overlay open and close changes', () => {
    setMainSidebarOpen(true);
    const sidebar = new SidebarNavState();

    sidebar.setMobile(true);
    expect(sidebar.isOpen).toBe(false);

    sidebar.toggle();
    expect(sidebar.isOpen).toBe(true);
    expect(getMainSidebarOpen()).toBe(true);

    sidebar.close();
    expect(sidebar.isOpen).toBe(false);
    expect(getMainSidebarOpen()).toBe(true);

    sidebar.setMobile(false);
    expect(sidebar.isOpen).toBe(true);
  });

  it('restores a closed desktop preference after mobile use', () => {
    setMainSidebarOpen(false);
    const sidebar = new SidebarNavState();

    sidebar.setMobile(true);
    sidebar.toggle();
    expect(sidebar.isOpen).toBe(true);

    sidebar.setMobile(false);
    expect(sidebar.isOpen).toBe(false);
    expect(getMainSidebarOpen()).toBe(false);
  });
});
