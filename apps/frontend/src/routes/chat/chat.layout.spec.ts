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
import { load as loadManagement } from './[serverId]/manage/+layout';

describe('chat layout catalog loading', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    calls.length = 0;
  });

  it.each([
    ['chat', load, undefined],
    ['management', loadManagement, true]
  ] as const)(
    'loads %s sections after the parent without tracking child routes',
    async (_, loader, management) => {
      const parent = vi.fn(async () => {
        calls.push('parent');
        return {};
      });

      await loader({
        parent,
        route: {
          get id() {
            throw new Error('A catalog layout must not track child route IDs');
          }
        }
      } as never);

      expect(calls).toEqual(['parent', 'catalog']);
      expect(parent).toHaveBeenCalledOnce();
      expect(preloadChatLocaleMessages).toHaveBeenCalledOnce();
      expect(preloadChatLocaleMessages.mock.calls[0]).toEqual(management ? [true] : []);
    }
  );
});
