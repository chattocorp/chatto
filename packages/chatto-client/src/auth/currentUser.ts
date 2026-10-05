import type { ConnectAPIConfig } from '../api/connect.js';
import { batch, signal } from '../reactivity/index.js';
import { getCurrentUserViaConnect, type CurrentUser } from '../api/viewer.js';
import { isAuthenticationRequiredError } from './errors.js';
import { getOriginViewer } from './originViewer.js';
import { isExplicitSignOutRedirectInProgress } from './signOut.js';

export type { CurrentUser };

/**
 * Per-server current-user state. One instance per registered server,
 * owned by `ServerStateStore`. Consumers read the active server's
 * instance via `registry.getStore(serverId).currentUser`, the
 * same way they reach every other per-server store.
 *
 * Authentication-required failures are reported to the owning registry/store.
 * The caller decides whether to prompt for reauthentication, revoke a session,
 * or clear local state.
 */
export class CurrentUserState {
  readonly #userSignal = signal<CurrentUser | undefined>(undefined);
  get user(): CurrentUser | undefined {
    return this.#userSignal.get();
  }
  set user(value: CurrentUser | undefined) {
    this.#userSignal.set(value);
  }
  readonly #loadingSignal = signal(true);
  get loading() {
    return this.#loadingSignal.get();
  }
  set loading(value) {
    this.#loadingSignal.set(value);
  }
  /**
   * The error of the latest viewer read that failed for a reason other than
   * authentication, such as an unreachable server. A new read clears it.
   */
  readonly #loadErrorSignal = signal<unknown>(null);
  get loadError(): unknown {
    return this.#loadErrorSignal.get();
  }
  /** Identity confirmed by the latest successful viewer request. */
  readonly #verifiedUserIdSignal = signal<string | null>(null);
  get verifiedUserId(): string | null {
    return this.#verifiedUserIdSignal.get();
  }
  set verifiedUserId(value: string | null) {
    this.#verifiedUserIdSignal.set(value);
  }
  #cookieAuth: boolean;
  #apiConfig?: ConnectAPIConfig;
  #loadCurrentUser: (config: ConnectAPIConfig) => Promise<CurrentUser>;
  #onAuthenticationRequired?: () => void;
  #loadPromise: Promise<void> | null = null;
  #generation = 0;
  #onLoaded?: (user: CurrentUser) => void;

  constructor(
    cookieAuth: boolean = false,
    apiConfig?: ConnectAPIConfig,
    loadCurrentUser = cookieAuth ? getOriginViewer : getCurrentUserViaConnect,
    onAuthenticationRequired?: () => void,
    onLoaded?: (user: CurrentUser) => void
  ) {
    this.#cookieAuth = cookieAuth;
    this.#apiConfig = apiConfig;
    this.#loadCurrentUser = loadCurrentUser;
    this.#onAuthenticationRequired = onAuthenticationRequired;
    this.#onLoaded = onLoaded;
  }

  /** Load the viewer once, sharing an in-flight request between route and store owners. */
  load(): Promise<void> {
    if (this.#loadPromise) return this.#loadPromise;

    batch(() => {
      this.loading = true;
      this.#loadErrorSignal.set(null);
    });
    const promise = this.#loadViewer(this.#generation).finally(() => {
      if (this.#loadPromise === promise) this.#loadPromise = null;
    });
    this.#loadPromise = promise;
    return promise;
  }

  /** Apply live account data through the registry's identity boundary. False if this owner was replaced. */
  apply(user: CurrentUser): boolean {
    if (this.#onLoaded) this.#onLoaded(user);
    else this.accept(user);
    return this.user?.id === user.id && this.verifiedUserId === user.id;
  }

  /** Publish complete account data after the registry has checked the account boundary. */
  accept(user: CurrentUser): void {
    this.#generation++;
    batch(() => {
      this.user = user;
      this.verifiedUserId = user.id;
      this.#loadErrorSignal.set(null);
      this.loading = false;
    });
  }

  /**
   * Apply a local change to the account data, but only while `userId` is still
   * the loaded account. A response for an earlier account then cannot change the
   * current one. `change` returns the fields to replace, or null to keep the data.
   * Returns whether the data changed.
   */
  update(
    userId: string | null,
    change: (user: CurrentUser) => Partial<CurrentUser> | null
  ): boolean {
    const user = this.user;
    if (!userId || user?.id !== userId) return false;
    const fields = change(user);
    if (!fields) return false;
    this.user = { ...user, ...fields };
    return true;
  }

  /** Clear account data and fence requests from a retired session or store. */
  reset(): void {
    batch(() => {
      this.invalidateVerification();
      this.user = undefined;
    });
  }

  /** Retain display data after auth loss, but reject requests from the previous session. */
  invalidateVerification(): void {
    this.#generation++;
    this.#loadPromise = null;
    batch(() => {
      this.verifiedUserId = null;
      this.loading = false;
    });
  }

  async #loadViewer(generation: number): Promise<void> {
    const isCurrent = () =>
      generation === this.#generation &&
      !(this.#cookieAuth && isExplicitSignOutRedirectInProgress());
    try {
      if (!this.#apiConfig) {
        throw new Error('current user Connect API config is not configured');
      }
      const user = await this.#loadCurrentUser(this.#apiConfig);
      if (!isCurrent()) return;
      this.apply(user);
    } catch (err) {
      if (!isCurrent()) return;
      if (isAuthenticationRequiredError(err)) {
        // Observers see the ended session together with the finished read.
        batch(() => {
          this.invalidateVerification();
          // The bearer interceptor already tried a refresh. If that grant was
          // rejected, the registry marked reauthentication required. A 401
          // after a successful refresh is not proof that the session was revoked.
          if (this.#cookieAuth || !this.#apiConfig?.renewBearerToken)
            this.#onAuthenticationRequired?.();
        });
        return;
      }
      // Surface network failures (CORS, DNS, server down) as a console
      // error so unreachable instances are visible in the dev console.
      // Don't throw — the caller treats this as a per-instance soft
      // failure, not a global crash.
      console.error('[auth] failed to load current user', err);
      batch(() => {
        this.#loadErrorSignal.set(err);
        this.loading = false;
      });
    } finally {
      if (generation === this.#generation) this.loading = false;
    }
  }
}
