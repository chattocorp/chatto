/** Account management destinations and their existing page access requirements. */
import { resolve } from '$app/paths';
import type { ResolvedPathname } from '$app/types';
import { serverIdToSegment } from '$lib/navigation';

/** Account identity and public bot ownership used to select an accessible page. */
export type ManagedAccountIdentity = {
  id: string;
  isBot?: boolean;
  bot?: { ownerUserId: string };
};

/** Current viewer and server capabilities required by the destination pages. */
export type AccountManagementViewer = {
  accountId: string | null;
  permissions: {
    canAdminViewUsers: boolean;
    canManageBots: boolean;
    canAdminManageAccounts: boolean;
  };
};

/** Return the account's management URL, or null when the viewer cannot open it. */
export function accountManagementHref(
  serverId: string,
  account: ManagedAccountIdentity,
  viewer: AccountManagementViewer
): ResolvedPathname | null {
  if (account.isBot) {
    if (
      !viewer.permissions.canManageBots &&
      !viewer.permissions.canAdminManageAccounts &&
      !(viewer.accountId && account.bot?.ownerUserId === viewer.accountId)
    )
      return null;
    return resolve('/chat/[serverId]/manage/server/bots/[botId]', {
      serverId: serverIdToSegment(serverId),
      botId: account.id
    });
  }
  if (!viewer.permissions.canAdminViewUsers) return null;
  return resolve('/chat/[serverId]/manage/server/members/[userId]', {
    serverId: serverIdToSegment(serverId),
    userId: account.id
  });
}
