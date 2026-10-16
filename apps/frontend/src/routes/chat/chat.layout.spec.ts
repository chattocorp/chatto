import { beforeEach, describe, expect, it, vi } from 'vitest';

const { calls, preloadChatLocaleMessages } = vi.hoisted(() => ({
  calls: [] as string[],
  preloadChatLocaleMessages: vi.fn(async () => {
    calls.push('catalog');
  })
}));

vi.mock('$lib/i18n/messages', () => ({
  preloadChatLocaleMessages
}));

import { load } from './+layout';

describe('chat layout catalog loading', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    calls.length = 0;
  });

  it.each([
    ['/chat/[serverId]/[roomId]', false],
    ['/chat/[serverId]/manage/server/general', true],
    ['/chat/[serverId]/manage/rooms/[roomId]', true],
    ['/chat/[serverId]/settings/account', false]
  ])('loads the required sections after parent locale selection: %s', async (id, management) => {
    calls.length = 0;
    const parent = vi.fn(async () => {
      calls.push('parent');
      return {};
    });

    await load({ parent, route: { id } } as never);

    expect(calls).toEqual(['parent', 'catalog']);
    expect(parent).toHaveBeenCalledOnce();
    expect(preloadChatLocaleMessages).toHaveBeenCalledOnce();
    expect(preloadChatLocaleMessages).toHaveBeenCalledWith(management);
  });
});
