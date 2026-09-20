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
    path.replace('[serverId]', params.serverId).replace('[userId]', params.userId)
}));

vi.mock('$lib/navigation', () => ({ serverIdToSegment: (serverId: string) => serverId }));

import { startDMWith } from './startDM';

describe('startDMWith', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('navigates to the recipient without creating a room first', async () => {
    await startDMWith('server-1', 'user-1');

    expect(mocks.goto).toHaveBeenCalledWith('/chat/server-1/dm/user-1');
    expect(mocks.getClient).not.toHaveBeenCalled();
    expect(mocks.startDM).not.toHaveBeenCalled();
  });
});
