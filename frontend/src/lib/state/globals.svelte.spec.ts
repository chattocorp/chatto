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

  it('remembers desktop toggles for the current app session', () => {
    const sidebar = new SidebarNavState(true);

    sidebar.toggle();
    expect(sidebar.isOpen).toBe(false);

    sidebar.toggle();
    expect(sidebar.isOpen).toBe(true);
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
