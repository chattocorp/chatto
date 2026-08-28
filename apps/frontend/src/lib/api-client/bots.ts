import { authHeaders, createChattoClient } from './connect.js';
import { BotService } from '@chatto/api-types/api/v1/bots_connect';
import { CredentialLastUsedState, type Bot as APIBot } from '@chatto/api-types/api/v1/bots_pb';

export type BotAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
  onAuthenticationRequired?: (serverId: string) => void;
};

export type Bot = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl: string | null;
  bio: string | null;
  timezone: string | null;
  ownerUserId: string;
  createdAt: Date | null;
  apiKeyCreatedAt: Date | null;
  apiKeyRotatedAt: Date | null;
  incomingWebhooks: {
    id: string;
    name: string;
    createdAt: Date | null;
    lastUsedState: 'no_use_recorded' | 'recorded' | 'unavailable';
    lastUsedAt: Date | null;
  }[];
};

export type BotPage = {
  bots: Bot[];
  totalCount: number;
  hasMore: boolean;
};

export function createBotAPI(config: BotAPIConfig) {
  const client = createChattoClient(BotService, config);
  const headers = () => authHeaders(config);
  return {
    async listBots(
      input: { search?: string | null; limit: number; offset: number },
      options: { signal?: AbortSignal } = {}
    ): Promise<BotPage> {
      const response = await client.listBots(
        {
          search: input.search ?? '',
          page: { limit: input.limit, offset: input.offset }
        },
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      return {
        bots: response.bots.map(botFromAPI),
        totalCount: Number(response.page?.totalCount ?? response.bots.length),
        hasMore: response.page?.hasMore ?? false
      };
    },
    async getBot(botUserId: string, options: { signal?: AbortSignal } = {}): Promise<Bot> {
      const response = await client.getBot(
        { botUserId },
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      return botFromAPI(requiredBot(response.bot));
    },
    async createBot(input: {
      login: string;
      displayName: string;
    }): Promise<{ bot: Bot; apiKey: string }> {
      const response = await client.createBot(input, { headers: headers() });
      return { bot: botFromAPI(requiredBot(response.bot)), apiKey: response.apiKey };
    },
    async deleteBot(botUserId: string): Promise<boolean> {
      return (await client.deleteBot({ botUserId }, { headers: headers() })).deleted;
    },
    async rotateBotAPIKey(botUserId: string): Promise<{ bot: Bot; apiKey: string }> {
      const response = await client.rotateBotApiKey({ botUserId }, { headers: headers() });
      return { bot: botFromAPI(requiredBot(response.bot)), apiKey: response.apiKey };
    },
    async createBotIncomingWebhook(
      botUserId: string,
      name: string
    ): Promise<{ bot: Bot; webhookUrl: string }> {
      const response = await client.createBotIncomingWebhook(
        { botUserId, name },
        { headers: headers() }
      );
      return { bot: botFromAPI(requiredBot(response.bot)), webhookUrl: response.webhookUrl };
    },
    async revokeBotIncomingWebhook(botUserId: string, webhookId: string): Promise<Bot> {
      const response = await client.revokeBotIncomingWebhook(
        { botUserId, webhookId },
        { headers: headers() }
      );
      return botFromAPI(requiredBot(response.bot));
    },
    async reassignBotOwner(botUserId: string, ownerUserId: string): Promise<Bot> {
      const response = await client.reassignBotOwner(
        { botUserId, ownerUserId },
        { headers: headers() }
      );
      return botFromAPI(requiredBot(response.bot));
    }
  };
}

export type BotAPI = ReturnType<typeof createBotAPI>;

function requiredBot(bot: APIBot | undefined): APIBot {
  if (!bot?.user) throw new Error('bot response did not include bot metadata');
  return bot;
}

function botFromAPI(bot: APIBot): Bot {
  const user = bot.user;
  if (!user) throw new Error('bot response did not include a user');
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: user.avatarUrl ?? null,
    bio: user.bio ?? null,
    timezone: user.timezone ?? null,
    ownerUserId: bot.ownerUserId,
    createdAt: bot.createdAt?.toDate() ?? null,
    apiKeyCreatedAt: bot.apiKeyCreatedAt?.toDate() ?? null,
    apiKeyRotatedAt: bot.apiKeyRotatedAt?.toDate() ?? null,
    incomingWebhooks: (bot.incomingWebhooks ?? []).map((webhook) => ({
      id: webhook.id,
      name: webhook.name,
      createdAt: webhook.createdAt?.toDate() ?? null,
      lastUsedState:
        webhook.lastUsedState === CredentialLastUsedState.RECORDED
          ? 'recorded'
          : webhook.lastUsedState === CredentialLastUsedState.NO_USE_RECORDED
            ? 'no_use_recorded'
            : 'unavailable',
      lastUsedAt: webhook.lastUsedAt?.toDate() ?? null
    }))
  };
}
