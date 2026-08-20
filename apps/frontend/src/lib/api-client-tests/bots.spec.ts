import { Timestamp } from '@bufbuild/protobuf';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { BotPermissionDecision, BotPermissionScopeKind } from '@chatto/api-types/api/v1/bots_pb';
import { createBotAPI } from '$lib/api-client/bots';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  listBots: vi.fn(),
  createBot: vi.fn(),
  rotateBotApiKey: vi.fn(),
  getBotPermissionMatrix: vi.fn(),
  setBotPermission: vi.fn()
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
      rotateBotApiKey: mocks.rotateBotApiKey,
      getBotPermissionMatrix: mocks.getBotPermissionMatrix,
      setBotPermission: mocks.setBotPermission
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

  it('maps the delegated permission matrix and sends scoped decisions', async () => {
    mocks.getBotPermissionMatrix.mockResolvedValue({
      matrix: {
        botUserId: 'U-bot',
        applicablePermissions: ['message.post'],
        scopes: [
          {
            id: 'R1',
            label: 'General',
            kind: BotPermissionScopeKind.ROOM,
            parentGroupId: ''
          }
        ],
        cells: [
          {
            permission: 'message.post',
            scopeId: 'R1',
            configured: BotPermissionDecision.ALLOW,
            delegated: BotPermissionDecision.ALLOW,
            ownerGranted: false,
            effectiveGranted: false
          }
        ]
      }
    });
    mocks.setBotPermission.mockResolvedValue({
      cell: {
        permission: 'message.post',
        scopeId: 'R1',
        configured: BotPermissionDecision.DENY,
        delegated: BotPermissionDecision.DENY,
        ownerGranted: true,
        effectiveGranted: false
      }
    });
    const api = createBotAPI({ baseUrl: '/api/connect', bearerToken: 'token' });

    const matrix = await api.getPermissionMatrix('U-bot');
    expect(matrix.cells[0]).toEqual({
      permission: 'message.post',
      scopeId: 'R1',
      configured: 'ALLOW',
      delegated: 'ALLOW',
      ownerGranted: false,
      effectiveGranted: false
    });

    await expect(
      api.setPermission({
        botUserId: 'U-bot',
        permission: 'message.post',
        scope: { tier: 'room', roomId: 'R1' },
        decision: 'DENY'
      })
    ).resolves.toMatchObject({ configured: 'DENY', effectiveGranted: false });
    expect(mocks.setBotPermission).toHaveBeenCalledWith(
      {
        botUserId: 'U-bot',
        permission: 'message.post',
        scope: { kind: BotPermissionScopeKind.ROOM, id: 'R1' },
        decision: BotPermissionDecision.DENY
      },
      { headers: { Authorization: 'Bearer token' } }
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
