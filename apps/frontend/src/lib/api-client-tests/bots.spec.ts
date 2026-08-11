import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createBotAPI } from '$lib/api-client/bots';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  deleteBot: vi.fn(),
  listApplicationCapabilities: vi.fn(),
  setBotCapabilities: vi.fn()
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
    mocks.listApplicationCapabilities.mockReset();
    mocks.setBotCapabilities.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      deleteBot: mocks.deleteBot,
      listApplicationCapabilities: mocks.listApplicationCapabilities,
      setBotCapabilities: mocks.setBotCapabilities
    });
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

  it('lists and updates server-defined capabilities', async () => {
    const capability = {
      id: 'dm.messages.read',
      displayName: 'Read direct messages',
      description: 'Read explicitly shared direct messages.'
    };
    mocks.listApplicationCapabilities.mockResolvedValue({ capabilities: [capability] });
    mocks.setBotCapabilities.mockResolvedValue({
      bot: {
        user: {
          id: 'UBOT',
          login: 'helper_bot',
          displayName: 'Helper',
          accountProfile: {
            case: 'bot',
            value: { ownerId: 'U1', description: 'Helps', capabilities: [capability] }
          }
        }
      }
    });
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    await expect(api.listApplicationCapabilities()).resolves.toEqual([capability]);
    await expect(api.setCapabilities('UBOT', [capability.id])).resolves.toMatchObject({
      id: 'UBOT',
      capabilities: [capability]
    });
    expect(mocks.setBotCapabilities).toHaveBeenCalledWith(
      { botId: 'UBOT', capabilityIds: [capability.id] },
      { headers: { Authorization: 'Bearer token' } }
    );
  });
});
