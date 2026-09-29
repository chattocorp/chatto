import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createBotAPI } from '$lib/api-client/bots';
import { BotService } from '@chatto/api-types/api/v1/bots_connect';
import { CredentialLastUsedState } from '@chatto/api-types/api/v1/bots_pb';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';

const mocks = mockService(BotService);

/** A bot API whose requests reach the mocked handlers through the real client. */
function botAPI() {
  return createBotAPI(fakeServer((router) => router.service(BotService, mocks)));
}

describe('createBotAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('lists bots and maps key metadata without exposing a verifier', async () => {
    const createdAt = new Date('2026-08-20T10:00:00Z');
    mocks.listBots.mockReturnValue({
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
          apiKeyCreatedAt: Timestamp.fromDate(createdAt),
          lastLoginChange: Timestamp.fromDate(createdAt)
        }
      ]
    });
    const api = botAPI();

    await expect(api.listBots({ search: 'helper', limit: 20, offset: 40 })).resolves.toEqual({
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
          lastLoginChange: createdAt,
          apiKeys: [],
          incomingWebhooks: []
        }
      ],
      totalCount: 1,
      hasMore: false
    });
    expect(receivedRequest(mocks.listBots)).toMatchObject({
      search: 'helper',
      page: { limit: 20, offset: 40 }
    });
  });

  it('passes cancellation to the server', async () => {
    mocks.listBots.mockReturnValue({ bots: [] });

    // Without the signal, the call would succeed.
    await expect(
      botAPI().listBots({ limit: 1, offset: 0 }, { signal: AbortSignal.abort() })
    ).rejects.toMatchObject({ code: Code.Canceled });
  });

  it('surfaces server errors with their Connect code', async () => {
    mocks.getBot.mockImplementation(() => {
      throw new ConnectError('bot not found', Code.NotFound);
    });

    await expect(botAPI().getBot('missing')).rejects.toMatchObject({ code: Code.NotFound });
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
    mocks.createBotApiKey.mockReturnValue({ bot: apiBot, apiKey: 'show-once-secret' });
    mocks.revokeBotApiKey.mockReturnValue({ bot: { ...apiBot, apiKeys: [] } });
    const api = botAPI();

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
    expect(receivedRequest(mocks.createBotApiKey)).toMatchObject({
      botUserId: 'one',
      name: 'Production'
    });
    await expect(api.revokeBotAPIKey('one', 'K-one')).resolves.toMatchObject({ apiKeys: [] });
    expect(receivedRequest(mocks.revokeBotApiKey)).toMatchObject({
      botUserId: 'one',
      keyId: 'K-one'
    });
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
    mocks.createBotIncomingWebhook.mockReturnValue({
      bot: apiBot,
      webhookUrl: 'https://chat.example/webhooks/incoming/secret'
    });
    mocks.revokeBotIncomingWebhook.mockReturnValue({
      bot: { ...apiBot, incomingWebhooks: [] }
    });
    const api = botAPI();

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
    expect(receivedRequest(mocks.createBotIncomingWebhook)).toMatchObject({
      botUserId: 'one',
      name: 'Production'
    });
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
    mocks.listBots.mockReturnValue({
      bots: [bot('one')],
      page: { totalCount: 2n, hasMore: true }
    });
    const api = botAPI();

    await expect(api.listBots({ limit: 1, offset: 0 })).resolves.toMatchObject({
      bots: [{ id: 'one' }],
      totalCount: 2,
      hasMore: true
    });
    expect(mocks.listBots).toHaveBeenCalledOnce();
    expect(receivedRequest(mocks.listBots)).toMatchObject({
      search: '',
      page: { limit: 1, offset: 0 }
    });
  });

  it('gets one bot by stable user ID', async () => {
    mocks.getBot.mockReturnValue({
      bot: {
        user: { id: 'one', login: 'one_bot', displayName: 'One' },
        ownerUserId: 'U-owner'
      }
    });
    const api = botAPI();

    await expect(api.getBot('one')).resolves.toMatchObject({ id: 'one' });
    expect(receivedRequest(mocks.getBot)).toMatchObject({ botUserId: 'one' });
  });

  it('treats unknown credential last-use states as unavailable', async () => {
    mocks.getBot.mockReturnValue({
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
    const api = botAPI();

    await expect(api.getBot('one')).resolves.toMatchObject({
      incomingWebhooks: [{ id: 'W-one', lastUsedState: 'unavailable', lastUsedAt: null }]
    });
  });

  it('reassigns a bot owner and returns the updated bot', async () => {
    mocks.reassignBotOwner.mockReturnValue({
      bot: {
        user: { id: 'one', login: 'one_bot', displayName: 'One' },
        ownerUserId: 'U-new-owner'
      }
    });
    const api = botAPI();

    await expect(api.reassignBotOwner('one', 'U-new-owner')).resolves.toMatchObject({
      id: 'one',
      ownerUserId: 'U-new-owner'
    });
    expect(receivedRequest(mocks.reassignBotOwner)).toMatchObject({
      botUserId: 'one',
      ownerUserId: 'U-new-owner'
    });
  });
});
