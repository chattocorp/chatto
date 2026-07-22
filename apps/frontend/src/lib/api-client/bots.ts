import { authHeaders, createChattoClient } from './connect.js';
import { BotService } from '@chatto/api-types/api/v1/bots_connect';
import {
  BotPermissionDecision,
  BotPermissionScopeKind,
  type Bot as APIBot,
  type BotPermissionMatrix as APIBotPermissionMatrix
} from '@chatto/api-types/api/v1/bots_pb';

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

export type BotPermissionScope = {
  id: string;
  label: string;
  kind: 'SERVER' | 'GROUP' | 'ROOM';
  parentGroupId: string;
};

export type BotPermissionMatrix = {
  botId: string;
  applicablePermissions: string[];
  scopes: BotPermissionScope[];
  cells: Array<{
    permission: string;
    scopeId: string;
    directDecision: 'ALLOW' | 'DENY' | 'NONE';
    effectiveDecision: 'ALLOW' | 'DENY' | 'NONE';
    ownerAllowed: boolean;
  }>;
};

export function createBotAPI(config: BotAPIConfig) {
  const client = createChattoClient(BotService, config);
  const headers = () => authHeaders(config);

  return {
    async listBots(
      input: {
        search?: string;
        limit?: number;
        offset?: number;
        ownedByCallerOnly?: boolean;
      } = {}
    ): Promise<BotPage> {
      const response = await client.listBots(
        {
          search: input.search ?? '',
          page: { limit: input.limit ?? 20, offset: input.offset ?? 0 },
          ownedByCallerOnly: input.ownedByCallerOnly ?? false
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
    },

    async getPermissionMatrix(botId: string): Promise<BotPermissionMatrix> {
      const response = await client.getBotPermissionMatrix({ botId }, { headers: headers() });
      if (!response.matrix) throw new Error('bot permission response did not include a matrix');
      return botPermissionMatrix(response.matrix);
    },

    async setPermission(input: {
      botId: string;
      permission: string;
      scope: BotPermissionScope;
      decision: 'ALLOW' | 'DENY' | 'NONE';
    }): Promise<void> {
      await client.setBotPermission(
        {
          botId: input.botId,
          permission: input.permission,
          scope: {
            kind: botPermissionScopeKind(input.scope.kind),
            id: input.scope.kind === 'SERVER' ? '' : input.scope.id.replace(/^[^:]+:/, '')
          },
          decision: botPermissionDecision(input.decision)
        },
        { headers: headers() }
      );
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

function botPermissionMatrix(matrix: APIBotPermissionMatrix): BotPermissionMatrix {
  return {
    botId: matrix.botId,
    applicablePermissions: [...matrix.applicablePermissions],
    scopes: matrix.scopes.map((scope) => ({
      id: scope.id,
      label: scope.label,
      kind: botPermissionScopeKindName(scope.kind),
      parentGroupId: scope.parentGroupId
    })),
    cells: matrix.cells.map((cell) => ({
      permission: cell.permission,
      scopeId: cell.scopeId,
      directDecision: botPermissionDecisionName(cell.directDecision),
      effectiveDecision: botPermissionDecisionName(cell.effectiveDecision),
      ownerAllowed: cell.ownerAllowed
    }))
  };
}

function botPermissionDecisionName(decision: BotPermissionDecision): 'ALLOW' | 'DENY' | 'NONE' {
  if (decision === BotPermissionDecision.ALLOW) return 'ALLOW';
  if (decision === BotPermissionDecision.DENY) return 'DENY';
  return 'NONE';
}

function botPermissionDecision(decision: 'ALLOW' | 'DENY' | 'NONE'): BotPermissionDecision {
  if (decision === 'ALLOW') return BotPermissionDecision.ALLOW;
  if (decision === 'DENY') return BotPermissionDecision.DENY;
  return BotPermissionDecision.NONE;
}

function botPermissionScopeKindName(kind: BotPermissionScopeKind): 'SERVER' | 'GROUP' | 'ROOM' {
  if (kind === BotPermissionScopeKind.GROUP) return 'GROUP';
  if (kind === BotPermissionScopeKind.ROOM) return 'ROOM';
  return 'SERVER';
}

function botPermissionScopeKind(kind: 'SERVER' | 'GROUP' | 'ROOM'): BotPermissionScopeKind {
  if (kind === 'GROUP') return BotPermissionScopeKind.GROUP;
  if (kind === 'ROOM') return BotPermissionScopeKind.ROOM;
  return BotPermissionScopeKind.SERVER;
}
