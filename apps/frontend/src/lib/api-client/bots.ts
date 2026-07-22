import { authHeaders, createChattoClient } from './connect.js';
import { BotService } from '@chatto/api-types/api/v1/bots_connect';
import type { Bot as APIBot } from '@chatto/api-types/api/v1/bots_pb';

export type BotAPIConfig = {
  baseUrl: string;
  bearerToken: string | null;
};

export type BotAccount = {
  id: string;
  login: string;
  displayName: string;
  avatarUrl: string | null;
  ownerId: string;
  description: string;
  createdAt: string | null;
  apiKeyCreatedAt: string | null;
};

export type BotPage = {
  bots: BotAccount[];
  totalCount: number;
  hasMore: boolean;
};

export type CreateBotInput = {
  login: string;
  displayName: string;
  description: string;
};

export type UpdateBotInput = {
  botId: string;
  login?: string;
  displayName?: string;
  description?: string;
};

export function createBotAPI(config: BotAPIConfig) {
  const client = createChattoClient(BotService, config);
  const headers = () => authHeaders(config);

  return {
    async listBots(
      input: { search?: string; limit?: number; offset?: number } = {}
    ): Promise<BotPage> {
      const response = await client.listBots(
        {
          search: input.search ?? '',
          page: { limit: input.limit ?? 20, offset: input.offset ?? 0 }
        },
        { headers: headers() }
      );
      return {
        bots: response.bots.map(botAccount),
        totalCount: Number(response.page?.totalCount ?? response.bots.length),
        hasMore: response.page?.hasMore ?? false
      };
    },

    async createBot(input: CreateBotInput): Promise<BotAccount> {
      const response = await client.createBot(input, { headers: headers() });
      return botAccount(requiredBot(response.bot));
    },

    async updateBot(input: UpdateBotInput): Promise<BotAccount> {
      const response = await client.updateBot(input, { headers: headers() });
      return botAccount(requiredBot(response.bot));
    }
  };
}

export type BotAPI = ReturnType<typeof createBotAPI>;

function requiredBot(bot: APIBot | undefined): APIBot {
  if (bot?.user?.accountProfile.case !== 'bot') {
    throw new Error('bot response did not include a bot account');
  }
  return bot;
}

function botAccount(bot: APIBot): BotAccount {
  const user = bot.user;
  const profile = user?.accountProfile.case === 'bot' ? user.accountProfile.value : undefined;
  if (!user || !profile) throw new Error('bot response did not include a bot account');
  return {
    id: user.id,
    login: user.login,
    displayName: user.displayName,
    avatarUrl: user.avatarUrl || null,
    ownerId: profile.ownerId,
    description: profile.description,
    createdAt: bot.createdAt?.toDate().toISOString() ?? null,
    apiKeyCreatedAt: bot.apiKey?.createdAt?.toDate().toISOString() ?? null
  };
}
