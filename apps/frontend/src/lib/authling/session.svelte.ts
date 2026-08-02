import type { AuthlingClientConfiguration } from '$lib/clientConfig';
import type { AccountDataAuthorization } from '$lib/accountData/authorization';
import {
  clearPersistedAuthorization,
  loadPersistedAuthorization,
  savePersistedAuthorization
} from '$lib/accountData/persistedAuthorization';

export type AuthlingSessionStatus = 'disconnected' | 'connecting' | 'connected' | 'error';

/** Owns this frontend's persisted Authling grant and visible session state. */
export class AuthlingSession {
  status = $state<AuthlingSessionStatus>('disconnected');
  providerLabel = $state<string | null>(null);
  accountId = $state<string | null>(null);
  error = $state<string | null>(null);

  restore(configuration: AuthlingClientConfiguration): AccountDataAuthorization | null {
    const authorization = loadPersistedAuthorization(configuration);
    if (!authorization) return null;
    this.providerLabel = authorization.providerLabel;
    this.accountId = authorization.accountId;
    return authorization;
  }

  beginConnecting(): void {
    this.status = 'connecting';
    this.error = null;
  }

  establish(authorization: AccountDataAuthorization): void {
    savePersistedAuthorization(authorization);
    this.providerLabel = authorization.providerLabel;
    this.accountId = authorization.accountId;
    this.status = 'connected';
    this.error = null;
  }

  transportDisconnected(): void {
    this.status = 'disconnected';
  }

  fail(error: unknown): void {
    this.clearGrant();
    this.status = 'error';
    this.error = error instanceof Error ? error.message : 'Account-data synchronization failed.';
  }

  clearGrant(): void {
    clearPersistedAuthorization();
    this.providerLabel = null;
    this.accountId = null;
  }

  reset(): void {
    this.clearGrant();
    this.status = 'disconnected';
    this.error = null;
  }
}
