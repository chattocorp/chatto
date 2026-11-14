import { serverSessionQueryRoot, type QueryKeyScope } from './keys';

function settingsRoot(scope: QueryKeyScope) {
  return [...serverSessionQueryRoot(scope), 'settings'] as const;
}

function accountSettingsRoot(scope: QueryKeyScope, userId: string) {
  return [...settingsRoot(scope), 'account', userId] as const;
}

export const settingsQueryKeys = {
  root: settingsRoot,
  notificationPoliciesRoot(scope: QueryKeyScope) {
    return [...settingsRoot(scope), 'notification-policies'] as const;
  },
  notificationPolicies(scope: QueryKeyScope, scopes: readonly string[]) {
    return [...settingsQueryKeys.notificationPoliciesRoot(scope), scopes] as const;
  },
  externalIdentities(scope: QueryKeyScope, userId: string) {
    return [...accountSettingsRoot(scope, userId), 'external-identities'] as const;
  },
  verifiedEmails(scope: QueryKeyScope, userId: string) {
    return [...accountSettingsRoot(scope, userId), 'verified-emails'] as const;
  },
  botsRoot(scope: QueryKeyScope) {
    return [...settingsRoot(scope), 'bots'] as const;
  },
  bots(scope: QueryKeyScope, search: string) {
    return [...settingsQueryKeys.botsRoot(scope), 'list', search] as const;
  },
  bot(scope: QueryKeyScope, botUserId: string) {
    return [...settingsQueryKeys.botsRoot(scope), 'detail', botUserId] as const;
  }
};
