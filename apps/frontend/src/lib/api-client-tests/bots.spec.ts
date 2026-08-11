import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createBotAPI } from '$lib/api-client/bots';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  deleteBot: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return { ...actual, createClient: mocks.createClient };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createBotAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.deleteBot.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({ deleteBot: mocks.deleteBot });
  });

  it('deletes a bot with bearer authentication', async () => {
    mocks.deleteBot.mockResolvedValue({ deleted: true });
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    await expect(api.deleteBot('UBOT')).resolves.toBeUndefined();
    expect(mocks.deleteBot).toHaveBeenCalledWith(
      { botId: 'UBOT' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('rejects an unacknowledged deletion', async () => {
    mocks.deleteBot.mockResolvedValue({ deleted: false });
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.deleteBot('UBOT')).rejects.toThrow('not acknowledged');
  });
});
