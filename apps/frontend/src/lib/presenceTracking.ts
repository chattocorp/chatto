import { untrack } from 'svelte';
import { Code, ConnectError } from '@connectrpc/connect';
import type { PresenceAPI } from '$lib/api-client/presence';
import {
  PresenceMode as APIMode,
  type PresencePreference
} from '@chatto/api-types/api/v1/presence_pb';
import {
  presencePreferences,
  type PresenceMode
} from '$lib/state/server/presencePreference.svelte';
import type { PresenceCacheScope } from '$lib/state/presenceCache.svelte';

const PRESENCE_REFRESH_MS = 30_000;
export type PresenceReporter = PresenceCacheScope &
  Pick<PresenceAPI, 'getPreference' | 'setPreference' | 'refreshPresence'>;

let selectMode: ((scope: PresenceCacheScope, mode: PresenceMode) => Promise<void>) | null = null;
let refreshChoice: ((scope: PresenceCacheScope) => void) | null = null;

/** Save a deliberate selection on this server. Never report success before acknowledgement. */
export async function setPresenceMode(scope: PresenceCacheScope, mode: PresenceMode) {
  if (!selectMode) throw new Error('Presence is not connected');
  await selectMode(scope, mode);
}

/** Reconcile a private device update; event payloads are invalidations, not stale choices. */
export function refreshPresencePreference(scope: PresenceCacheScope) {
  refreshChoice?.(scope);
}

function apiMode(mode: PresenceMode): APIMode {
  switch (mode) {
    case 'online':
      return APIMode.ONLINE;
    case 'away':
      return APIMode.AWAY;
    case 'doNotDisturb':
      return APIMode.DO_NOT_DISTURB;
    case 'invisible':
      return APIMode.INVISIBLE;
  }
}

function localMode(mode: APIMode): PresenceMode {
  switch (mode) {
    case APIMode.ONLINE:
      return 'online';
    case APIMode.AWAY:
      return 'away';
    case APIMode.DO_NOT_DISTURB:
      return 'doNotDisturb';
    default:
      return 'invisible';
  }
}

function identity(scope: PresenceCacheScope) {
  return JSON.stringify([scope.serverId, scope.userId]);
}

/** Owns private preference recovery and liveness. Each account has an independent
 * request generation; auth changes and newer choices invalidate older replies.
 * A failed initial read never falls back to a legacy Online report.
 */
export function initPresenceTracking(getReporters: () => PresenceReporter[]) {
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
    presencePreferences.get(account.reporter).accept(localMode(value.mode), value.revision);
  }

  async function reconcile(account: Account, retryConflict = true) {
    if (account.busy) return;
    const sequence = ++account.sequence;
    const preference = presencePreferences.get(account.reporter);
    try {
      let value = await account.reporter.getPreference();
      if (!current(account, sequence)) return;
      if (!value) {
        value = await account.reporter.setPreference(apiMode(preference.mode), '');
      } else if (
        !preference.migrated.get() &&
        preference.mode === 'invisible' &&
        value.mode !== APIMode.INVISIBLE
      ) {
        // An older device's explicit invisible choice must not become public
        // during its first upgrade to the shared preference.
        value = await account.reporter.setPreference(APIMode.INVISIBLE, value.revision);
      }
      if (!current(account, sequence)) return;
      accept(account, sequence, value);
      if (!value?.revision) return;
      accept(account, sequence, await account.reporter.refreshPresence());
    } catch (error) {
      if (
        retryConflict &&
        current(account, sequence) &&
        (ConnectError.from(error).code === Code.Aborted || ConnectError.from(error).code === Code.Canceled)
      ) {
        await reconcile(account, false);
      }
      // Retry on the next tick; never replace a saved privacy choice on failure.
    }
  }

  async function choose(scope: PresenceCacheScope, mode: PresenceMode) {
    const account = accounts.get(identity(scope));
    if (!account || account.busy) throw new Error('Presence is not ready');
    const preference = presencePreferences.get(scope);
    if (!preference.ready) throw new Error('Presence is not ready');
    const sequence = ++account.sequence;
    account.busy = true;
    try {
      const value = await account.reporter.setPreference(apiMode(mode), preference.revision);
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

  function refresh(scope: PresenceCacheScope) {
    const account = accounts.get(identity(scope));
    if (account) void reconcile(account);
  }
  selectMode = choose;
  refreshChoice = refresh;
  const timer = setInterval(() => {
    const existing = new Set(accounts.values());
    sync();
    for (const account of accounts.values()) if (existing.has(account)) void reconcile(account);
  }, PRESENCE_REFRESH_MS);

  return {
    sync,
    stop() {
      stopped = true;
      clearInterval(timer);
      if (selectMode === choose) selectMode = null;
      if (refreshChoice === refresh) refreshChoice = null;
      accounts.clear();
      presencePreferences.clear();
    }
  };
}
