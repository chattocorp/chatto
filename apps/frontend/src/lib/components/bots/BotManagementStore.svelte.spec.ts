import { describe, expect, it, vi } from 'vitest';
import type { BotAPI, BotAccount } from '$lib/api-client/bots';
import type { UserAPI } from '$lib/api-client/users';
import { BotManagementStore } from './BotManagementStore.svelte';

function bot(id: string, ownerId = 'owner-1'): BotAccount {
  return {
    id,
    login: `${id}_bot`,
    displayName: `Bot ${id}`,
    avatarUrl: null,
    ownerId,
    description: `Description for ${id}`,
    createdAt: '2026-07-22T12:00:00.000Z',
    apiKeyCreatedAt: null
  };
}

describe('BotManagementStore', () => {
  it('loads pages, hydrates owners once, and reconciles creates and updates', async () => {
    const listBots = vi
      .fn()
      .mockResolvedValueOnce({ bots: [bot('one')], totalCount: 2, hasMore: true })
      .mockResolvedValueOnce({ bots: [bot('two')], totalCount: 2, hasMore: false });
    const createBot = vi.fn().mockResolvedValue(bot('three'));
    const updateBot = vi.fn().mockResolvedValue({ ...bot('one'), displayName: 'Updated bot' });
    const batchGetUsers = vi.fn().mockResolvedValue([
      {
        id: 'owner-1',
        login: 'owner',
        displayName: 'Bot Owner',
        deleted: false,
        avatarUrl: null
      }
    ]);
    const botAPI = { listBots, createBot, updateBot } as unknown as BotAPI;
    const userAPI = { batchGetUsers } as unknown as UserAPI;
    const store = new BotManagementStore(() => botAPI, () => userAPI);

    await store.load();
    expect(store.bots.map((item) => item.id)).toEqual(['one']);
    expect(store.owner(store.bots[0])?.displayName).toBe('Bot Owner');

    await store.loadMore();
    expect(store.bots.map((item) => item.id)).toEqual(['one', 'two']);
    expect(batchGetUsers).toHaveBeenCalledTimes(1);

    await store.create({ login: 'three_bot', displayName: 'Three', description: 'Three' });
    expect(store.bots[0].id).toBe('three');
    expect(store.totalCount).toBe(3);

    await store.update({ botId: 'one', displayName: 'Updated bot' });
    expect(store.bots.find((item) => item.id === 'one')?.displayName).toBe('Updated bot');
  });
});
