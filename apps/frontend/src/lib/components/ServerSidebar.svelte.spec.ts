import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { q, testSnippet } from '$lib/test-utils';
import { sidebarNav } from '$lib/state/globals.svelte';
import ServerSidebar from './ServerSidebar.svelte';

vi.mock('$app/navigation', () => ({
  goto: vi.fn(),
  pushState: vi.fn()
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'origin'
}));

vi.mock('$lib/state/server/connection.svelte', () => ({
  useConnection: () => () => ({
    connectBaseUrl: 'https://chat.example.test',
    bearerToken: 'token'
  })
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    tryGetStore: () => ({
      currentUser: { user: null },
      voiceCall: null,
      rooms: null
    })
  }
}));

vi.mock('$lib/state/userProfiles.svelte', () => ({
  getLiveAvatarUrl: (_userId: string, fallback: string | null) => fallback,
  getLiveCustomStatus: (_userId: string, fallback: unknown) => fallback,
  getLiveDisplayName: (_userId: string, fallback: string) => fallback
}));

function resetSidebar() {
  sidebarNav.setMobile(false);
  if (!sidebarNav.isOpen) sidebarNav.toggle();
  sidebarNav.setMobile(true);
}

describe('ServerSidebar', () => {
  beforeEach(() => {
    resetSidebar();
  });

  it('uses the shared mobile closed marker for delayed visibility hiding', async () => {
    const { container } = render(ServerSidebar, {
      props: {
        children: testSnippet('<nav>Rooms</nav>')
      }
    });
    await tick();

    const sidebar = q(container, '[data-testid="server-sidebar"]');
    expect(sidebar).not.toBeNull();
    if (!sidebar) return;

    expect(sidebar.classList.contains('sidebar-mobile-closed')).toBe(true);
    expect(sidebar.classList.contains('max-md:invisible')).toBe(false);
  });
});
