type PendingEmailVerification = {
  version: 1;
  serverId: string;
  userId: string;
  email: string;
};

function storageKey(serverId: string): string {
  return `chatto:i:${serverId}:pending-email-verification`;
}

/**
 * Stores a pending email-verification address for this user, server, and tab,
 * then reads it back.
 * A false result means the verification request must not be sent because the
 * follow-up page could not recover its required state.
 */
export function storePendingEmailVerification(
  serverId: string,
  userId: string,
  email: string
): boolean {
  const value: PendingEmailVerification = { version: 1, serverId, userId, email };
  try {
    sessionStorage.setItem(storageKey(serverId), JSON.stringify(value));
    return readPendingEmailVerification(serverId, userId) === email;
  } catch {
    return false;
  }
}

/** Returns the pending address for this server and tab, if it is valid. */
export function readPendingEmailVerification(serverId: string, userId: string): string {
  try {
    const parsed = JSON.parse(
      sessionStorage.getItem(storageKey(serverId)) ?? 'null'
    ) as Partial<PendingEmailVerification> | null;
    if (
      parsed?.version !== 1 ||
      parsed.serverId !== serverId ||
      parsed.userId !== userId ||
      typeof parsed.email !== 'string' ||
      parsed.email === ''
    ) {
      return '';
    }
    return parsed.email;
  } catch {
    return '';
  }
}

/** Clears the pending address when it still matches the expected challenge. */
export function clearPendingEmailVerification(
  serverId: string,
  userId: string,
  expectedEmail?: string
): void {
  try {
    if (expectedEmail && readPendingEmailVerification(serverId, userId) !== expectedEmail) return;
    sessionStorage.removeItem(storageKey(serverId));
  } catch {
    // Storage is best effort while clearing an already unusable challenge.
  }
}
