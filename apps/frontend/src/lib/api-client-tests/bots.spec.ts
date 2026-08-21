import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createBotAPI } from '$lib/api-client/bots';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listBots: vi.fn(),
  getBot: vi.fn(),
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
      getBot: mocks.getBot,
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

    await expect(
      api.listBots({ search: 'helper', limit: 20, offset: 40 }, { signal })
    ).resolves.toEqual({
      bots: [
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
      ],
      totalCount: 1,
      hasMore: false
    });
    expect(mocks.listBots).toHaveBeenCalledWith(
      { search: 'helper', page: { limit: 20, offset: 40 } },
      { headers: { Authorization: 'Bearer token' }, signal }
    );
  });

  it('returns pagination metadata without eagerly loading later pages', async () => {
    const bot = (id: string) => ({
      user: { id, login: `${id}_bot`, displayName: id },
      ownerUserId: 'U-owner'
    });
    mocks.listBots.mockResolvedValue({
      bots: [bot('one')],
      page: { totalCount: 2, hasMore: true }
    });
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.listBots({ limit: 1, offset: 0 })).resolves.toMatchObject({
      bots: [{ id: 'one' }],
      totalCount: 2,
      hasMore: true
    });
    expect(mocks.listBots).toHaveBeenCalledOnce();
    expect(mocks.listBots).toHaveBeenCalledWith(
      { search: '', page: { limit: 1, offset: 0 } },
      { headers: undefined }
    );
  });

  it('gets one bot by stable user ID', async () => {
    mocks.getBot.mockResolvedValue({
      bot: {
        user: { id: 'one', login: 'one_bot', displayName: 'One' },
        ownerUserId: 'U-owner'
      }
    });
    const signal = new AbortController().signal;
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.getBot('one', { signal })).resolves.toMatchObject({ id: 'one' });
    expect(mocks.getBot).toHaveBeenCalledWith({ botUserId: 'one' }, { headers: undefined, signal });
  });
});
