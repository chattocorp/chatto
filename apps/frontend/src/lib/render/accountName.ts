/** Fixed account-type label, identical in every locale. */
export const BOT_ACCOUNT_LABEL = 'BOT';

/** Canonical account metadata needed to label automation in the UI. */
export type AccountNameIdentity = { isBot?: boolean; deleted?: boolean };

/** Missing or deleted identities must not carry a bot label. */
export function isBotAccount(identity?: AccountNameIdentity | null): boolean {
  return identity?.isBot === true && !identity.deleted;
}

/** Format an account reference for plain-text UI; never store this as a name. */
export function formatAccountName(name: string, identity?: AccountNameIdentity | null): string {
  return isBotAccount(identity) ? `${name} (${BOT_ACCOUNT_LABEL})` : name;
}

/** Placeholder for a rich account name in a localized sentence. */
export function accountNameToken(index: number): string {
  return `\uFFF0${index}\uFFF1`;
}
