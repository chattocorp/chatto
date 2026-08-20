import { authHeaders, createChattoClient } from './connect.js';
import { BotService } from '@chatto/api-types/api/v1/bots_connect';
import { type Bot as APIBot } from '@chatto/api-types/api/v1/bots_pb';

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
  ownerUserId: string;
  createdAt: Date | null;
  apiKeyCreatedAt: Date | null;
  apiKeyRotatedAt: Date | null;
};

export function createBotAPI(config: BotAPIConfig) {
  const client = createChattoClient(BotService, config);
  const headers = () => authHeaders(config);
  return {
    async listBots(options: { signal?: AbortSignal } = {}): Promise<Bot[]> {
      const bots: Bot[] = [];
      let offset = 0;
      for (;;) {
        const response = await client.listBots(
          { page: { limit: 100, offset } },
          { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
        );
        bots.push(...response.bots.map(botFromAPI));
        if (!response.page?.hasMore || response.bots.length === 0) return bots;
        offset += response.bots.length;
      }
    },
    async createBot(input: {
      login: string;
      displayName: string;
    }): Promise<{ bot: Bot; apiKey: string }> {
      const response = await client.createBot(input, { headers: headers() });
      return { bot: botFromAPI(requiredBot(response.bot)), apiKey: response.apiKey };
    },
    async updateBot(input: {
      botUserId: string;
      login?: string;
      displayName?: string;
    }): Promise<Bot> {
      const response = await client.updateBot(input, { headers: headers() });
      return botFromAPI(requiredBot(response.bot));
    },
    async deleteBot(botUserId: string): Promise<boolean> {
      return (await client.deleteBot({ botUserId }, { headers: headers() })).deleted;
    },
    async rotateBotAPIKey(botUserId: string): Promise<{ bot: Bot; apiKey: string }> {
      const response = await client.rotateBotApiKey({ botUserId }, { headers: headers() });
      return { bot: botFromAPI(requiredBot(response.bot)), apiKey: response.apiKey };
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
    ownerUserId: bot.ownerUserId,
    createdAt: bot.createdAt?.toDate() ?? null,
    apiKeyCreatedAt: bot.apiKeyCreatedAt?.toDate() ?? null,
    apiKeyRotatedAt: bot.apiKeyRotatedAt?.toDate() ?? null
  };
}
