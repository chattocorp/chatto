import { authHeaders, createChattoClient } from './connect.js';
import { BotService } from '@chatto/api-types/api/v1/bots_connect';
import {
  BotPermissionDecision,
  BotPermissionScopeKind,
  type Bot as APIBot,
  type BotPermissionCell as APIBotPermissionCell,
  type BotPermissionMatrix as APIBotPermissionMatrix,
  type BotPermissionMatrixScope as APIBotPermissionMatrixScope
} from '@chatto/api-types/api/v1/bots_pb';

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

export type BotPermissionDecisionValue = 'ALLOW' | 'DENY' | 'NONE';
export type BotPermissionScope =
  { tier: 'server' } | { tier: 'group'; groupId: string } | { tier: 'room'; roomId: string };
export type BotPermissionMatrixScope = {
  id: string;
  label: string;
  kind: 'SERVER' | 'GROUP' | 'ROOM';
  parentGroupId: string;
};
export type BotPermissionCell = {
  permission: string;
  scopeId: string;
  configured: BotPermissionDecisionValue;
  delegated: BotPermissionDecisionValue;
  ownerGranted: boolean;
  effectiveGranted: boolean;
};
export type BotPermissionMatrix = {
  botUserId: string;
  applicablePermissions: string[];
  scopes: BotPermissionMatrixScope[];
  cells: BotPermissionCell[];
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
    },
    async getPermissionMatrix(
      botUserId: string,
      options: { signal?: AbortSignal } = {}
    ): Promise<BotPermissionMatrix> {
      const response = await client.getBotPermissionMatrix(
        { botUserId },
        { headers: headers(), ...(options.signal ? { signal: options.signal } : {}) }
      );
      return matrixFromAPI(response.matrix);
    },
    async setPermission(input: {
      botUserId: string;
      permission: string;
      scope: BotPermissionScope;
      decision: BotPermissionDecisionValue;
    }): Promise<BotPermissionCell> {
      const response = await client.setBotPermission(
        {
          botUserId: input.botUserId,
          permission: input.permission,
          scope: scopeToAPI(input.scope),
          decision: decisionToAPI(input.decision)
        },
        { headers: headers() }
      );
      return cellFromAPI(response.cell);
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

function matrixFromAPI(matrix: APIBotPermissionMatrix | undefined): BotPermissionMatrix {
  if (!matrix) throw new Error('bot response did not include a permission matrix');
  return {
    botUserId: matrix.botUserId,
    applicablePermissions: [...matrix.applicablePermissions],
    scopes: matrix.scopes.map(scopeFromAPI),
    cells: matrix.cells.map(cellFromAPI)
  };
}

function scopeFromAPI(scope: APIBotPermissionMatrixScope): BotPermissionMatrixScope {
  return {
    id: scope.id,
    label: scope.label,
    kind:
      scope.kind === BotPermissionScopeKind.GROUP
        ? 'GROUP'
        : scope.kind === BotPermissionScopeKind.ROOM
          ? 'ROOM'
          : 'SERVER',
    parentGroupId: scope.parentGroupId
  };
}

function cellFromAPI(cell: APIBotPermissionCell | undefined): BotPermissionCell {
  if (!cell) throw new Error('bot response did not include a permission cell');
  return {
    permission: cell.permission,
    scopeId: cell.scopeId,
    configured: decisionFromAPI(cell.configured),
    delegated: decisionFromAPI(cell.delegated),
    ownerGranted: cell.ownerGranted,
    effectiveGranted: cell.effectiveGranted
  };
}

function decisionFromAPI(decision: BotPermissionDecision): BotPermissionDecisionValue {
  if (decision === BotPermissionDecision.ALLOW) return 'ALLOW';
  if (decision === BotPermissionDecision.DENY) return 'DENY';
  return 'NONE';
}

function decisionToAPI(decision: BotPermissionDecisionValue): BotPermissionDecision {
  if (decision === 'ALLOW') return BotPermissionDecision.ALLOW;
  if (decision === 'DENY') return BotPermissionDecision.DENY;
  return BotPermissionDecision.NONE;
}

function scopeToAPI(scope: BotPermissionScope) {
  if (scope.tier === 'group') return { kind: BotPermissionScopeKind.GROUP, id: scope.groupId };
  if (scope.tier === 'room') return { kind: BotPermissionScopeKind.ROOM, id: scope.roomId };
  return { kind: BotPermissionScopeKind.SERVER, id: '' };
}
