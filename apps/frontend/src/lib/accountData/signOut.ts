/** Disconnect account-data sync and clear this frontend's local Authling state. */
export async function signOutAccountData(): Promise<void> {
  const { accountDataSync } = await import('./sync.svelte');
  await accountDataSync.signOut();
}
