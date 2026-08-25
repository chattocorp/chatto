import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import {
  notificationPolicyScopeKey,
  type NotificationAPI,
  type NotificationPolicyField,
  type NotificationPolicyPatch,
  type NotificationPolicyScope,
  type ScopedNotificationPolicy
} from '$lib/api-client/notifications';

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

/**
 * Server-scoped lifecycle owner for the notification policy matrix.
 * Policies remain authoritative server responses; cells only enter pending
 * state while a sparse update is in flight.
 */
export class NotificationPolicyMatrixState {
  readonly #api: NotificationAPI;
  #loadGeneration = 0;
  #refreshGeneration = 0;
  #resetGeneration = 0;
  #scopeSignature = '';
  #loadedScopes: NotificationPolicyScope[] = [];
  readonly #pendingTokens = new SvelteMap<string, symbol>();
  policies = $state.raw<Record<string, ScopedNotificationPolicy>>({});
  readonly pendingKeys = new SvelteSet<string>();
  loading = $state(false);
  error = $state<string | null>(null);
  errorKind = $state<'load' | 'save' | null>(null);

  constructor(api: NotificationAPI) {
    this.#api = api;
  }

  policy(scope: NotificationPolicyScope): ScopedNotificationPolicy | undefined {
    return this.policies[notificationPolicyScopeKey(scope)];
  }

  cellKey(scope: NotificationPolicyScope, field: NotificationPolicyField): string {
    return `${notificationPolicyScopeKey(scope)}::${field}`;
  }

  isPending(scope: NotificationPolicyScope, field: NotificationPolicyField): boolean {
    return this.pendingKeys.has(this.cellKey(scope, field));
  }

  async load(scopes: NotificationPolicyScope[]): Promise<void> {
    const signature = scopes.map(notificationPolicyScopeKey).join('|');
    const generation = ++this.#loadGeneration;
    this.#refreshGeneration++;
    this.#scopeSignature = signature;
    this.#loadedScopes = [...scopes];
    this.loading = true;
    this.error = null;
    this.errorKind = null;
    try {
      const rows = await this.#api.batchGetNotificationPolicies(scopes);
      if (generation !== this.#loadGeneration || signature !== this.#scopeSignature) return;
      this.policies = Object.fromEntries(
        rows.map((policy) => [notificationPolicyScopeKey(policy.scope), policy])
      );
    } catch (error) {
      if (generation !== this.#loadGeneration || signature !== this.#scopeSignature) return;
      this.policies = {};
      this.error = errorMessage(error);
      this.errorKind = 'load';
    } finally {
      if (generation === this.#loadGeneration && signature === this.#scopeSignature) {
        this.loading = false;
      }
    }
  }

  async update(
    scope: NotificationPolicyScope,
    field: NotificationPolicyField,
    value: NotificationPolicyPatch[NotificationPolicyField]
  ): Promise<void> {
    await this.#updatePatch(scope, { [field]: value });
  }

  async #updatePatch(
    scope: NotificationPolicyScope,
    patch: NotificationPolicyPatch
  ): Promise<void> {
    const fields = Object.keys(patch) as NotificationPolicyField[];
    const keys = fields.map((field) => this.cellKey(scope, field));
    if (keys.length === 0 || keys.some((key) => this.pendingKeys.has(key))) return;
    const pendingToken = Symbol(notificationPolicyScopeKey(scope));
    const resetGeneration = this.#resetGeneration;
    for (const key of keys) {
      this.#pendingTokens.set(key, pendingToken);
      this.pendingKeys.add(key);
    }
    this.error = null;
    this.errorKind = null;
    try {
      const policy = await this.#api.updateScopedNotificationPolicy(scope, patch);
      if (resetGeneration !== this.#resetGeneration) return;
      this.policies = {
        ...this.policies,
        [notificationPolicyScopeKey(policy.scope)]: policy
      };
      await this.#refreshLoadedPolicies();
    } catch (error) {
      if (resetGeneration !== this.#resetGeneration) return;
      this.error = errorMessage(error);
      this.errorKind = 'save';
    } finally {
      for (const key of keys) {
        if (this.#pendingTokens.get(key) === pendingToken) {
          this.#pendingTokens.delete(key);
          this.pendingKeys.delete(key);
        }
      }
    }
  }

  reset(): void {
    this.#resetGeneration++;
    this.#loadGeneration++;
    this.#refreshGeneration++;
    this.#scopeSignature = '';
    this.#loadedScopes = [];
    this.#pendingTokens.clear();
    this.pendingKeys.clear();
    this.policies = {};
    this.loading = false;
    this.error = null;
    this.errorKind = null;
  }

  /** Refresh inherited effective values after an ancestor override changes. */
  async #refreshLoadedPolicies(): Promise<void> {
    if (this.#loadedScopes.length === 0) return;
    const scopes = [...this.#loadedScopes];
    const signature = scopes.map(notificationPolicyScopeKey).join('|');
    const loadGeneration = this.#loadGeneration;
    const refreshGeneration = ++this.#refreshGeneration;
    try {
      const rows = await this.#api.batchGetNotificationPolicies(scopes);
      if (
        loadGeneration !== this.#loadGeneration ||
        refreshGeneration !== this.#refreshGeneration ||
        signature !== this.#scopeSignature
      ) {
        return;
      }
      this.policies = Object.fromEntries(
        rows.map((policy) => [notificationPolicyScopeKey(policy.scope), policy])
      );
    } catch (error) {
      if (
        loadGeneration !== this.#loadGeneration ||
        refreshGeneration !== this.#refreshGeneration ||
        signature !== this.#scopeSignature
      ) {
        return;
      }
      this.error = errorMessage(error);
      this.errorKind = 'load';
    }
  }
}
