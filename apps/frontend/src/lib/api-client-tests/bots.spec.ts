import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createBotAPI } from '$lib/api-client/bots';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listBots: vi.fn(),
  createBot: vi.fn(),
  rotateBotApiKey: vi.fn()
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
    for (const mock of Object.values(mocks)) mock.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      listBots: mocks.listBots,
      createBot: mocks.createBot,
      rotateBotApiKey: mocks.rotateBotApiKey
    });
  });

  it('lists bots and maps key metadata without exposing a verifier', async () => {
    const createdAt = new Date('2026-08-20T10:00:00Z');
    mocks.listBots.mockResolvedValue({
      bots: [
        {
          user: {
            id: 'U-bot',
            login: 'helper_bot',
            displayName: 'Helper',
            avatarUrl: ''
          },
          ownerUserId: 'U-owner',
          createdAt: Timestamp.fromDate(createdAt),
          apiKeyCreatedAt: Timestamp.fromDate(createdAt)
        }
      ]
    });
    const signal = new AbortController().signal;
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    await expect(api.listBots({ signal })).resolves.toEqual([
      {
        id: 'U-bot',
        login: 'helper_bot',
        displayName: 'Helper',
        avatarUrl: '',
        ownerUserId: 'U-owner',
        createdAt,
        apiKeyCreatedAt: createdAt,
        apiKeyRotatedAt: null
      }
    ]);
    expect(mocks.listBots).toHaveBeenCalledWith(
      { page: { limit: 100, offset: 0 } },
      { headers: { Authorization: 'Bearer token' }, signal }
    );
  });

  it('loads every bot page automatically', async () => {
    const bot = (id: string) => ({
      user: { id, login: `${id}_bot`, displayName: id },
      ownerUserId: 'U-owner'
    });
    mocks.listBots
      .mockResolvedValueOnce({ bots: [bot('one')], page: { hasMore: true } })
      .mockResolvedValueOnce({ bots: [bot('two')], page: { hasMore: false } });
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.listBots()).resolves.toMatchObject([{ id: 'one' }, { id: 'two' }]);
    expect(mocks.listBots).toHaveBeenNthCalledWith(
      2,
      { page: { limit: 100, offset: 1 } },
      { headers: undefined }
    );
  });
});
