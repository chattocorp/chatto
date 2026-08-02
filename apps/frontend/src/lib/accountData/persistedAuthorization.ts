import type { AuthlingClientConfiguration } from '$lib/clientConfig';
import type { AccountDataAuthorization } from './authorization';

const AUTHORIZATION_KEY = 'chatto:account-data:authorization';
const STORE_KEY = 'chatto:account-data:tinybase';

/** Persist a short-lived Authling grant across tabs and browser restarts. */
export function savePersistedAuthorization(authorization: AccountDataAuthorization): void {
  localStorage.setItem(AUTHORIZATION_KEY, JSON.stringify(authorization));
}

/** Load a valid grant only when it still belongs to the configured Authling client. */
export function loadPersistedAuthorization(
  expected: AuthlingClientConfiguration,
  now = Date.now()
): AccountDataAuthorization | null {
  const raw = localStorage.getItem(AUTHORIZATION_KEY);
  if (!raw) return null;
  try {
    const authorization = JSON.parse(raw) as AccountDataAuthorization;
    if (
      typeof authorization.accessToken !== 'string' ||
      typeof authorization.expiresAt !== 'number' ||
      authorization.expiresAt <= now + 5000 ||
      authorization.issuer !== expected.issuer ||
      authorization.clientId !== expected.clientId ||
      typeof authorization.accountId !== 'string' ||
      typeof authorization.providerLabel !== 'string'
    ) {
      clearPersistedAuthorization();
      return null;
    }
    return authorization;
  } catch {
    clearPersistedAuthorization();
    return null;
  }
}

export function clearPersistedAuthorization(): void {
  localStorage.removeItem(AUTHORIZATION_KEY);
}

/** Clear this browser's Authling grant and synchronized cache without creating deletion stamps. */
export function clearPersistedAccountDataSession(): void {
  clearPersistedAuthorization();
  localStorage.removeItem(STORE_KEY);
}
