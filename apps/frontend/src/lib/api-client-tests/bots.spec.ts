import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createBotAPI } from '$lib/api-client/bots';
import { CredentialLastUsedState } from '@chatto/api-types/api/v1/bots_pb';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listBots: vi.fn(),
  getBot: vi.fn(),
  createBot: vi.fn(),
  createBotApiKey: vi.fn(),
  revokeBotApiKey: vi.fn(),
  createBotIncomingWebhook: vi.fn(),
  revokeBotIncomingWebhook: vi.fn(),
  reassignBotOwner: vi.fn()
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
      createBotApiKey: mocks.createBotApiKey,
      revokeBotApiKey: mocks.revokeBotApiKey,
      createBotIncomingWebhook: mocks.createBotIncomingWebhook,
      revokeBotIncomingWebhook: mocks.revokeBotIncomingWebhook,
      reassignBotOwner: mocks.reassignBotOwner
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
            avatarUrl: '',
            bio: 'Build helper',
            timezone: 'Europe/Berlin'
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
          bio: 'Build helper',
          timezone: 'Europe/Berlin',
          ownerUserId: 'U-owner',
          createdAt,
          apiKeyCreatedAt: createdAt,
          apiKeys: [],
          incomingWebhooks: []
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

  it('creates and revokes named API keys with safe usage metadata', async () => {
    const createdAt = new Date('2026-08-30T10:00:00Z');
    const apiBot = {
      user: { id: 'one', login: 'one_bot', displayName: 'One' },
      ownerUserId: 'U-owner',
      apiKeys: [
        {
          id: 'K-one',
          name: 'Production',
          createdAt: Timestamp.fromDate(createdAt),
          lastUsedState: CredentialLastUsedState.NO_USE_RECORDED
        }
      ]
    };
    mocks.createBotApiKey.mockResolvedValue({ bot: apiBot, apiKey: 'show-once-secret' });
    mocks.revokeBotApiKey.mockResolvedValue({ bot: { ...apiBot, apiKeys: [] } });
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    await expect(api.createBotAPIKey('one', 'Production')).resolves.toMatchObject({
      bot: {
        apiKeys: [
          {
            id: 'K-one',
            name: 'Production',
            createdAt,
            lastUsedState: 'no_use_recorded',
            lastUsedAt: null
          }
        ]
      },
      apiKey: 'show-once-secret'
    });
    expect(mocks.createBotApiKey).toHaveBeenCalledWith(
      { botUserId: 'one', name: 'Production' },
      { headers: { Authorization: 'Bearer token' } }
    );
    await expect(api.revokeBotAPIKey('one', 'K-one')).resolves.toMatchObject({ apiKeys: [] });
    expect(mocks.revokeBotApiKey).toHaveBeenCalledWith(
      { botUserId: 'one', keyId: 'K-one' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });

  it('manages named incoming webhooks and maps safe usage metadata', async () => {
    const createdAt = new Date('2026-08-27T10:00:00Z');
    const apiBot = {
      user: { id: 'one', login: 'one_bot', displayName: 'One' },
      ownerUserId: 'U-owner',
      incomingWebhooks: [
        {
          id: 'W-one',
          name: 'Production',
          createdAt: Timestamp.fromDate(createdAt),
          lastUsedState: CredentialLastUsedState.NO_USE_RECORDED
        }
      ]
    };
    mocks.createBotIncomingWebhook.mockResolvedValue({
      bot: apiBot,
      webhookUrl: 'https://chat.example/webhooks/incoming/secret'
    });
    mocks.revokeBotIncomingWebhook.mockResolvedValue({
      bot: { ...apiBot, incomingWebhooks: [] }
    });
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    await expect(api.createBotIncomingWebhook('one', 'Production')).resolves.toMatchObject({
      bot: {
        id: 'one',
        incomingWebhooks: [
          {
            id: 'W-one',
            name: 'Production',
            createdAt,
            lastUsedState: 'no_use_recorded',
            lastUsedAt: null
          }
        ]
      },
      webhookUrl: 'https://chat.example/webhooks/incoming/secret'
    });
    expect(mocks.createBotIncomingWebhook).toHaveBeenCalledWith(
      { botUserId: 'one', name: 'Production' },
      { headers: { Authorization: 'Bearer token' } }
    );
    await expect(api.revokeBotIncomingWebhook('one', 'W-one')).resolves.toMatchObject({
      id: 'one',
      incomingWebhooks: []
    });
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

  it('treats unknown credential last-use states as unavailable', async () => {
    mocks.getBot.mockResolvedValue({
      bot: {
        user: { id: 'one', login: 'one_bot', displayName: 'One' },
        ownerUserId: 'U-owner',
        incomingWebhooks: [
          {
            id: 'W-one',
            name: 'Future state',
            lastUsedState: 99 as CredentialLastUsedState
          }
        ]
      }
    });
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: null });

    await expect(api.getBot('one')).resolves.toMatchObject({
      incomingWebhooks: [{ id: 'W-one', lastUsedState: 'unavailable', lastUsedAt: null }]
    });
  });

  it('reassigns a bot owner and returns the updated bot', async () => {
    mocks.reassignBotOwner.mockResolvedValue({
      bot: {
        user: { id: 'one', login: 'one_bot', displayName: 'One' },
        ownerUserId: 'U-new-owner'
      }
    });
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    await expect(api.reassignBotOwner('one', 'U-new-owner')).resolves.toMatchObject({
      id: 'one',
      ownerUserId: 'U-new-owner'
    });
    expect(mocks.reassignBotOwner).toHaveBeenCalledWith(
      { botUserId: 'one', ownerUserId: 'U-new-owner' },
      { headers: { Authorization: 'Bearer token' } }
    );
  });
});
