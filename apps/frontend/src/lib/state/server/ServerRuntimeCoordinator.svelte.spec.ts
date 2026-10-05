import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';

const mocks = vi.hoisted(() => ({
  activeServerId: 'remote',
  routeId: '/chat/[serverId]',
  start: vi.fn(),
  setActiveServer: vi.fn(),
  stop: vi.fn()
}));

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => mocks.activeServerId
}));

vi.mock('$app/state', () => ({
  page: {
    get route() {
      return { id: mocks.routeId };
    }
  }
}));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  client: { start: mocks.start, stop: mocks.stop, setActiveServer: mocks.setActiveServer }
}));

import ServerRuntimeCoordinator from './ServerRuntimeCoordinator.svelte';

describe('ServerRuntimeCoordinator', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.routeId = '/chat/[serverId]';
  });

  it('keeps the URL-active chat server live and stops the runtime on unmount', () => {
    const { unmount } = render(ServerRuntimeCoordinator);
    expect(mocks.start).toHaveBeenCalledOnce();
    expect(mocks.setActiveServer).toHaveBeenCalledWith('remote');
    unmount();
    expect(mocks.stop).toHaveBeenCalledOnce();
  });

  it('keeps no server live outside chat routes', () => {
    mocks.routeId = '/login';
    render(ServerRuntimeCoordinator);
    expect(mocks.setActiveServer).toHaveBeenCalledWith(null);
  });
});
