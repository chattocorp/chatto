import { Timestamp } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createBotAPI } from '../bots.js';
import { BotService } from '@chatto/api-types/api/v1/bots_connect';
import { CredentialLastUsedState } from '@chatto/api-types/api/v1/bots_pb';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

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

  it('manages outbound webhooks', async () => {
    mocks.listBotWebhookFailures.mockReturnValue({ failures: [], nextCursor: '' });
    mocks.listBotOutboundWebhooks.mockReturnValue({ webhooks: [{ id: 'OW1', name: 'Hook' }] });
    mocks.createBotOutboundWebhook.mockReturnValue({ webhook: { id: 'OW1' } });
    mocks.updateBotOutboundWebhook.mockReturnValue({ webhook: { id: 'OW1' } });
    mocks.revokeBotOutboundWebhook.mockReturnValue({});
    const api = botAPI();

    await api.listWebhookFailures('one', 'OW1');
    expect(receivedRequest(mocks.listBotWebhookFailures)).toMatchObject({
      botUserId: 'one',
      webhookId: 'OW1',
      cursor: '',
      pageSize: 20
    });
    await expect(api.listOutboundWebhooks('one')).resolves.toMatchObject([{ id: 'OW1' }]);
    await api.createOutboundWebhook({
      botUserId: 'one',
      name: 'Hook',
      url: 'https://hooks.example.test',
      authorization: 'Bearer x',
      enabled: true
    });
    expect(receivedRequest(mocks.createBotOutboundWebhook)).toMatchObject({
      name: 'Hook',
      url: 'https://hooks.example.test'
    });
    await api.updateOutboundWebhook('one', 'OW1', { enabled: false });
    expect(receivedRequest(mocks.updateBotOutboundWebhook)).toMatchObject({
      botUserId: 'one',
      webhookId: 'OW1',
      enabled: false,
      updateMask: { paths: ['enabled'] }
    });
    await api.revokeOutboundWebhook('one', 'OW1');
    expect(receivedRequest(mocks.revokeBotOutboundWebhook)).toMatchObject({
      botUserId: 'one',
      webhookId: 'OW1'
    });
  });

  it('creates and deletes a bot', async () => {
    mocks.createBot.mockReturnValue({
      bot: { user: { id: 'one', login: 'one_bot', displayName: 'One' }, ownerUserId: 'U1' },
      apiKey: 'secret'
    });
    mocks.deleteBot.mockReturnValue({ deleted: true });
    const api = botAPI();

    await expect(api.createBot({ login: 'one_bot', displayName: 'One' })).resolves.toMatchObject({
      bot: { id: 'one', avatarUrl: null, createdAt: null, apiKeys: [] },
      apiKey: 'secret'
    });
    await expect(api.deleteBot('one')).resolves.toBe(true);
  });

  it('rejects a bot answer without bot metadata', async () => {
    mocks.getBot.mockReturnValue({ bot: {} });
    await expect(botAPI().getBot('one')).rejects.toThrow('did not include bot metadata');
  });
});
