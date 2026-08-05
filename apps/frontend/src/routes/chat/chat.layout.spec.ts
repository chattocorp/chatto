import { describe, expect, it, vi } from 'vitest';

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
  it('loads the complete locale after the root layout has selected it', async () => {
    calls.length = 0;
    const parent = vi.fn(async () => {
      calls.push('parent');
      return {};
    });

    await load({ parent } as never);

    expect(calls).toEqual(['parent', 'catalog']);
    expect(parent).toHaveBeenCalledOnce();
    expect(preloadChatLocaleMessages).toHaveBeenCalledOnce();
  });
});
