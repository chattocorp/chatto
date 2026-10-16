import { describe, expect, it, vi } from 'vitest';
import { accountManagementHref, type AccountManagementViewer } from './accountManagementHref';

vi.mock('$lib/navigation', () => ({
  serverIdToSegment: (id: string) => (id === 'origin' ? '-' : 'chat.example.org')
}));

const viewer: AccountManagementViewer = {
  accountId: 'viewer',
  permissions: { canAdminViewUsers: false, canManageBots: false, canAdminManageAccounts: false }
};
const bot = { id: 'bot', isBot: true, bot: { ownerUserId: 'owner' } };

describe('account management destinations', () => {
  it.each(['origin', 'remote'])('routes accessible accounts on %s', (serverId) => {
    const segment = serverId === 'origin' ? '-' : 'chat.example.org';
    const admin = {
      ...viewer,
      permissions: { ...viewer.permissions, canAdminViewUsers: true, canManageBots: true }
    };
    expect(accountManagementHref(serverId, { id: 'user' }, admin)).toBe(
      `/chat/${segment}/manage/server/members/user`
    );
    expect(accountManagementHref(serverId, bot, admin)).toBe(
      `/chat/${segment}/manage/server/bots/bot`
    );
  });

  it('does not link accounts without destination access', () => {
    expect(accountManagementHref('origin', { id: 'user' }, viewer)).toBeNull();
    expect(accountManagementHref('origin', bot, viewer)).toBeNull();
    const reader = { ...viewer, permissions: { ...viewer.permissions, canAdminViewUsers: true } };
    expect(accountManagementHref('origin', bot, reader)).toBeNull();
  });

  it('allows bot owners and account managers without bot.manage', () => {
    expect(accountManagementHref('origin', bot, { ...viewer, accountId: 'owner' })).toBe(
      '/chat/-/manage/server/bots/bot'
    );
    const manager = {
      ...viewer,
      permissions: { ...viewer.permissions, canAdminManageAccounts: true }
    };
    expect(accountManagementHref('origin', bot, manager)).toBe('/chat/-/manage/server/bots/bot');
    expect(accountManagementHref('origin', { id: 'user' }, manager)).toBeNull();
  });

  it('does not treat missing ownership as an ownership grant', () => {
    expect(
      accountManagementHref('origin', { id: 'bot', isBot: true }, { ...viewer, accountId: null })
    ).toBeNull();
  });
});
