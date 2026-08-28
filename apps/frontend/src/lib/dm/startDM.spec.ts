import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mocks } = vi.hoisted(() => ({
  mocks: {
    getClient: vi.fn(),
    goto: vi.fn(),
    startDM: vi.fn()
  }
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: { getClient: mocks.getClient }
}));

vi.mock('$app/navigation', () => ({ goto: mocks.goto }));

vi.mock('$app/paths', () => ({
  resolve: (path: string, params: Record<string, string>) =>
    path.replace('[serverId]', params.serverId).replace('[roomId]', params.roomId)
}));

vi.mock('$lib/navigation', () => ({ serverIdToSegment: (serverId: string) => serverId }));

import { startDMWith } from './startDM';

describe('startDMWith', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('prepares the destination before navigating to the direct message', async () => {
    mocks.getClient.mockReturnValue({
      getAPI: () => ({ startDM: mocks.startDM })
    });
    mocks.startDM.mockResolvedValue({ id: 'dm-1' });
    const onRoomReady = vi.fn();

    await startDMWith('server-1', 'user-1', { onRoomReady });

    expect(mocks.startDM).toHaveBeenCalledWith(['user-1']);
    expect(onRoomReady).toHaveBeenCalledWith('dm-1');
    expect(mocks.goto).toHaveBeenCalledWith('/chat/server-1/dm-1');
    expect(onRoomReady).toHaveBeenCalledBefore(mocks.goto);
  });

  it('does not navigate when a direct message cannot be started', async () => {
    mocks.getClient.mockReturnValue({
      getAPI: () => ({ startDM: mocks.startDM })
    });
    mocks.startDM.mockResolvedValue(null);
    const onRoomReady = vi.fn();

    await startDMWith('server-1', 'user-1', { onRoomReady });

    expect(onRoomReady).not.toHaveBeenCalled();
    expect(mocks.goto).not.toHaveBeenCalled();
  });
});
