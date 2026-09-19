import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { type StorageSlot, serverSlot } from '$lib/storage/slot';
import type { PresenceCacheScope } from '../presenceCache.svelte';

/** Explicit availability choices. Activity never changes the selected mode. */
export type PresenceMode = 'online' | 'away' | 'doNotDisturb' | 'invisible';

/** Validate selections read from browser storage events. */
export function isPresenceMode(value: unknown): value is PresenceMode {
  return (
    value === 'online' || value === 'away' || value === 'doNotDisturb' || value === 'invisible'
  );
}

function presenceModeStatus(mode: PresenceMode): PresenceStatus {
  switch (mode) {
    case 'away':
      return PresenceStatus.AWAY;
    case 'doNotDisturb':
      return PresenceStatus.DO_NOT_DISTURB;
    case 'invisible':
      return PresenceStatus.OFFLINE;
    default:
      return PresenceStatus.ONLINE;
  }
}

const modeCodec = {
  serialize: (mode: PresenceMode | null) => mode ?? '',
  parse: (raw: string) => (isPresenceMode(raw) ? raw : undefined)
};

/** Retained only to migrate choices from clients with one global preference. */
export const LEGACY_PRESENCE_MODE_STORAGE_KEY = 'chatto.presence.mode';

/** An unreadable preference must not make a possibly invisible account visible. */
function readMode(key: string, legacy = false): PresenceMode | null {
  try {
    if (typeof localStorage === 'undefined') return 'invisible';
    const raw = localStorage.getItem(key);
    if (raw === null) return null;
    if (isPresenceMode(raw)) return raw;
    return legacy && raw === 'auto' ? 'online' : 'invisible';
  } catch {
    return 'invisible';
  }
}

/** Device-local choice for one account on one server, loaded before reporting. */
class PresencePreference {
  /** Saved user choice; a report response does not replace this selection. */
  mode = $state<PresenceMode>('online');
  /** Local display and DND state, reconciled with this account's accepted report. */
  effectiveStatus = $state<PresenceStatus>(PresenceStatus.ONLINE);
  readonly slot: StorageSlot<PresenceMode | null>;

  constructor(scope: PresenceCacheScope) {
    this.slot = serverSlot(
      scope.serverId,
      `presence:${encodeURIComponent(scope.userId)}`,
      null,
      modeCodec
    );
    const stored = readMode(this.slot.key);
    this.mode = stored ?? readMode(LEGACY_PRESENCE_MODE_STORAGE_KEY, true) ?? 'online';
    this.effectiveStatus = presenceModeStatus(this.mode);
    // Freeze the legacy choice per account, including invisible. Future choices
    // never write the old key or change any other account's migration fallback.
    if (stored === null) this.slot.set(this.mode);
  }

  /** Save and verify before applying a choice; callers must show save failures. */
  select(mode: PresenceMode) {
    this.slot.set(mode);
    let saved = false;
    try {
      saved = typeof localStorage !== 'undefined' && localStorage.getItem(this.slot.key) === mode;
    } catch {
      // Do not claim a privacy choice was saved when storage cannot confirm it.
    }
    if (!saved) throw new Error('Could not save presence preference');
    this.apply(mode);
  }

  /** Read the latest saved choice on activation or a cross-tab notification. */
  reload() {
    const stored = readMode(this.slot.key);
    if (stored !== null) this.apply(stored);
  }

  /** Apply a selection from this tab or another tab without an echo write. */
  apply(mode: PresenceMode) {
    this.mode = mode;
    this.effectiveStatus = presenceModeStatus(mode);
  }
}

class PresencePreferences {
  #entries = new Map<string, PresencePreference>();

  /** Obtain the account's shared reactive preference, hydrating it on first use. */
  get(scope: PresenceCacheScope): PresencePreference {
    const key = JSON.stringify([scope.serverId, scope.userId]);
    let preference = this.#entries.get(key);
    if (!preference) {
      preference = new PresencePreference(scope);
      this.#entries.set(key, preference);
    }
    return preference;
  }

  /** Release runtime state when the chat root stops; saved choices remain. */
  clear() {
    this.#entries.clear();
  }
}

export const presencePreferences = new PresencePreferences();
