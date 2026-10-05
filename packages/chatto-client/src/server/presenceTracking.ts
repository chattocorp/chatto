import { untrack } from '../reactivity/index.js';
import { Code, ConnectError } from '@connectrpc/connect';
import type { PresenceAPI } from '../api/presence.js';
import { PresenceStatus, type PresencePreference } from '@chatto/api-types/api/v1/presence_pb';
import type { PresencePreferences, PresenceScope } from './presencePreference.js';

/** Interval of presence heartbeats. A heartbeat keeps the account present. */
const PRESENCE_REFRESH_MS = 30_000;

/** The presence requests of one account on one server. */
export type PresenceReporter = PresenceScope &
  Pick<PresenceAPI, 'getPreference' | 'setPreference' | 'refreshPresence'>;

/** Keeps the presence of a host's accounts current. See `createPresenceTracker`. */
export type PresenceTracker = {
  /** Start tracking new reporters and stop tracking removed ones. */
  sync(): void;
  /**
   * Save a deliberate selection on one server. Resolves only after the server
   * acknowledges it. Rejects while the account's choice is not loaded, while
   * another selection for the account is in progress, and when a newer request
   * or an authentication change discards the acknowledgement.
   */
  select(scope: PresenceScope, status: PresenceStatus): Promise<void>;
  /**
   * Read the choice again, for example after another device changed it.
   * Event payloads are invalidations, not choices.
   */
  refresh(scope: PresenceScope): void;
  /**
   * Stop the heartbeat and clear the `PresencePreferences` instance that the
   * tracker received, including entries of other trackers that share it.
   * Saved choices remain in device storage and on the server.
   */
  stop(): void;
};

function identity(scope: PresenceScope) {
  return JSON.stringify([scope.serverId, scope.userId]);
}

/**
 * Owns private preference recovery and liveness for interactive hosts. Each
 * account loads its shared choice, then sends a heartbeat every 30 seconds.
 * Each account has an independent request generation; auth changes and newer
 * choices invalidate older replies. A failed initial read never falls back to
 * a legacy Online report. `getReporters` returns the accounts to track; call
 * `sync()` when that list changes.
 */
export function createPresenceTracker(
  presencePreferences: PresencePreferences,
  getReporters: () => PresenceReporter[]
): PresenceTracker {
  type Account = { reporter: PresenceReporter; sequence: number; busy: boolean };
  const accounts = new Map<string, Account>();
  let stopped = false;

  function current(account: Account, sequence: number) {
    return (
      !stopped &&
      sequence === account.sequence &&
      accounts.get(identity(account.reporter)) === account &&
      getReporters().some((r) => identity(r) === identity(account.reporter))
    );
  }

  function accept(account: Account, sequence: number, value: PresencePreference | undefined) {
    if (!current(account, sequence) || !value?.revision) return;
    presencePreferences.get(account.reporter).accept(value.status, value.revision);
  }

  async function reconcile(account: Account, retryConflict = true) {
    if (account.busy || !current(account, account.sequence)) return;
    const sequence = ++account.sequence;
    const preference = presencePreferences.get(account.reporter);
    try {
      // Once initialized, the heartbeat also recovers missed device updates.
      // Every new authenticated reporter still reads before its first heartbeat.
      if (preference.ready) {
        const value = await account.reporter.refreshPresence();
        if (!current(account, sequence)) return;
        if (value?.revision) accept(account, sequence, value);
        else preference.ready = false;
        return;
      }
      let value = await account.reporter.getPreference();
      if (!current(account, sequence)) return;
      if (!value) {
        value = await account.reporter.setPreference(preference.status, '');
      } else if (
        !preference.revision &&
        !preference.migrated.get() &&
        preference.status === PresenceStatus.OFFLINE &&
        value.status !== PresenceStatus.OFFLINE
      ) {
        // An older device's explicit invisible choice must not become public
        // during its first upgrade to the shared preference.
        value = await account.reporter.setPreference(PresenceStatus.OFFLINE, value.revision);
      }
      if (!current(account, sequence)) return;
      accept(account, sequence, value);
      if (!value?.revision) return;
      accept(account, sequence, await account.reporter.refreshPresence());
    } catch (error) {
      if (
        retryConflict &&
        current(account, sequence) &&
        (ConnectError.from(error).code === Code.Aborted ||
          ConnectError.from(error).code === Code.Canceled)
      ) {
        await reconcile(account, false);
      }
      // Retry on the next tick; never replace a saved privacy choice on failure.
    }
  }

  async function choose(scope: PresenceScope, status: PresenceStatus) {
    const account = accounts.get(identity(scope));
    if (!account || account.busy) throw new Error('Presence is not ready');
    const preference = presencePreferences.get(scope);
    if (!preference.ready) throw new Error('Presence is not ready');
    const sequence = ++account.sequence;
    account.busy = true;
    try {
      const value = await account.reporter.setPreference(status, preference.revision);
      if (!current(account, sequence) || !value?.revision)
        throw new Error('Presence selection interrupted');
      accept(account, sequence, value);
    } finally {
      account.busy = false;
      // Recover both conflicts and lost acknowledgements without retrying user intent.
      void reconcile(account);
    }
  }

  function sync() {
    const reporters = getReporters();
    untrack(() => {
      if (stopped) return;
      const retained = new Set(reporters.map(identity));
      for (const key of accounts.keys()) if (!retained.has(key)) accounts.delete(key);
      for (const reporter of reporters) {
        const existing = accounts.get(identity(reporter));
        if (existing) existing.reporter = reporter;
        else {
          presencePreferences.get(reporter).ready = false;
          const account = { reporter, sequence: 0, busy: false };
          accounts.set(identity(reporter), account);
          void reconcile(account);
        }
      }
    });
  }

  function refresh(scope: PresenceScope) {
    const account = accounts.get(identity(scope));
    if (account) void reconcile(account);
  }
  const timer = setInterval(() => {
    const existing = new Set(accounts.values());
    sync();
    for (const account of accounts.values()) if (existing.has(account)) void reconcile(account);
  }, PRESENCE_REFRESH_MS);

  return {
    sync,
    select: choose,
    refresh,
    stop() {
      stopped = true;
      clearInterval(timer);
      accounts.clear();
      presencePreferences.clear();
    }
  };
}
