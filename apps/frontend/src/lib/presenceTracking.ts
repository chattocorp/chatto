import { untrack } from 'svelte';
import { APIPresenceStatus, type PresenceAPI } from '$lib/api-client/presence';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import {
  isPresenceMode,
  presencePreferences,
  type PresenceMode
} from '$lib/state/server/presencePreference.svelte';
import type { PresenceCacheScope } from '$lib/state/presenceCache.svelte';

const PRESENCE_REFRESH_MS = 30_000;

/** Authenticated account identity and its server's presence API. */
export type PresenceReporter = PresenceCacheScope & Pick<PresenceAPI, 'setPresence'>;

let applyModeFromUI: ((scope: PresenceCacheScope) => void) | null = null;

/** Save and report a choice only for the selected server/account. */
export function setPresenceMode(scope: PresenceCacheScope, mode: PresenceMode) {
  presencePreferences.get(scope).select(mode);
  applyModeFromUI?.(scope);
}

function apiStatus(status: PresenceStatus): APIPresenceStatus {
  switch (status) {
    case PresenceStatus.AWAY:
      return APIPresenceStatus.AWAY;
    case PresenceStatus.DO_NOT_DISTURB:
      return APIPresenceStatus.DO_NOT_DISTURB;
    default:
      return APIPresenceStatus.ONLINE;
  }
}

function acceptedStatus(status: APIPresenceStatus): PresenceStatus {
  switch (status) {
    case APIPresenceStatus.AWAY:
      return PresenceStatus.AWAY;
    case APIPresenceStatus.DO_NOT_DISTURB:
      return PresenceStatus.DO_NOT_DISTURB;
    default:
      return PresenceStatus.ONLINE;
  }
}

function identity(scope: PresenceCacheScope): string {
  return JSON.stringify([scope.serverId, scope.userId]);
}

/**
 * Owns reports for the chat root's authenticated accounts. Call sync when the
 * authenticated account list changes, and stop when the root is destroyed.
 * Each account has independent preferences and request ordering. Invisible
 * accounts send no presence reports, including their first report after login.
 */
export function initPresenceTracking(getReporters: () => PresenceReporter[]) {
  const accounts = new Map<string, { reporter: PresenceReporter; sequence: number }>();
  let stopped = false;

  function report(account: { reporter: PresenceReporter; sequence: number }) {
    const { reporter } = account;
    if (!getReporters().some((current) => identity(current) === identity(reporter))) return;
    const preference = presencePreferences.get(reporter);
    const sequence = ++account.sequence;
    if (preference.mode === 'invisible') return;
    void reporter
      .setPresence(apiStatus(preference.effectiveStatus), true)
      .then((accepted) => {
        if (
          stopped ||
          sequence !== account.sequence ||
          accounts.get(identity(reporter)) !== account
        )
          return;
        // Authentication may have changed before the root's next reconciliation.
        if (!getReporters().some((current) => identity(current) === identity(reporter))) return;
        preference.effectiveStatus = acceptedStatus(accepted);
      })
      .catch(() => {});
  }

  function sync() {
    const reporters = getReporters();
    untrack(() => {
      if (stopped) return;
      const retained = new Set(reporters.map(identity));
      for (const key of accounts.keys()) {
        if (!retained.has(key)) accounts.delete(key);
      }
      for (const reporter of reporters) {
        const key = identity(reporter);
        const existing = accounts.get(key);
        if (existing) {
          existing.reporter = reporter;
        } else {
          presencePreferences.get(reporter).reload();
          const account = { reporter, sequence: 0 };
          accounts.set(key, account);
          report(account);
        }
      }
    });
  }

  function applySelection(scope: PresenceCacheScope) {
    const account = accounts.get(identity(scope));
    if (account) report(account);
  }

  function onStorage(event: StorageEvent) {
    if (!isPresenceMode(event.newValue)) return;
    for (const account of accounts.values()) {
      const preference = presencePreferences.get(account.reporter);
      if (event.key !== preference.slot.key) continue;
      // Storage events can arrive after a newer choice in this tab. Read the
      // current value so a delayed Online event cannot expose an invisible user.
      preference.reload();
      report(account);
    }
  }

  applyModeFromUI = applySelection;
  window.addEventListener('storage', onStorage);
  const timer = setInterval(() => {
    // Reconcile before refreshing, so signed-out accounts cannot report again.
    const existing = new Set(accounts.values());
    sync();
    for (const account of accounts.values()) {
      if (existing.has(account)) report(account);
    }
  }, PRESENCE_REFRESH_MS);

  return {
    sync,
    stop() {
      stopped = true;
      clearInterval(timer);
      window.removeEventListener('storage', onStorage);
      if (applyModeFromUI === applySelection) applyModeFromUI = null;
      accounts.clear();
      presencePreferences.clear();
    }
  };
}
