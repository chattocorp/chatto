import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { Codecs, type StorageSlot, serverSlot } from '$lib/storage/slot';
import type { PresenceCacheScope } from '../presenceCache.svelte';

// Keep the old storage strings at this boundary so existing local choices
// remain readable. Runtime state and API calls use PresenceStatus directly.
const statusCodec = {
  serialize(status: PresenceStatus | null): string {
    switch (status) {
      case PresenceStatus.ONLINE:
        return 'online';
      case PresenceStatus.AWAY:
        return 'away';
      case PresenceStatus.DO_NOT_DISTURB:
        return 'doNotDisturb';
      case PresenceStatus.OFFLINE:
        return 'invisible';
      default:
        return '';
    }
  },
  parse(raw: string): PresenceStatus | undefined {
    switch (raw) {
      case 'online':
        return PresenceStatus.ONLINE;
      case 'away':
        return PresenceStatus.AWAY;
      case 'doNotDisturb':
        return PresenceStatus.DO_NOT_DISTURB;
      case 'invisible':
        return PresenceStatus.OFFLINE;
      default:
        return undefined;
    }
  }
};

/** Retained only to migrate choices from clients with one global preference. */
export const LEGACY_PRESENCE_MODE_STORAGE_KEY = 'chatto.presence.mode';

/** An unreadable preference must not make a possibly invisible account visible. */
function readStatus(key: string, legacy = false): PresenceStatus | null {
  try {
    if (typeof localStorage === 'undefined') return PresenceStatus.OFFLINE;
    const raw = localStorage.getItem(key);
    if (raw === null) return null;
    return (
      statusCodec.parse(raw) ??
      (legacy && raw === 'auto' ? PresenceStatus.ONLINE : PresenceStatus.OFFLINE)
    );
  } catch {
    return PresenceStatus.OFFLINE;
  }
}

/** Private server choice with a device-local fallback used only for migration. */
class PresencePreference {
  revision = $state('');
  ready = $state(false);
  /** Shared choice, updated only from acknowledged server state. */
  status = $state<PresenceStatus>(PresenceStatus.ONLINE);
  readonly slot: StorageSlot<PresenceStatus | null>;
  readonly migrated: StorageSlot<boolean>;

  constructor(scope: PresenceCacheScope) {
    this.migrated = serverSlot(
      scope.serverId,
      `presence-synced:${encodeURIComponent(scope.userId)}`,
      false,
      Codecs.boolean
    );
    this.slot = serverSlot(
      scope.serverId,
      `presence:${encodeURIComponent(scope.userId)}`,
      null,
      statusCodec
    );
    const stored = readStatus(this.slot.key);
    this.status =
      stored ?? readStatus(LEGACY_PRESENCE_MODE_STORAGE_KEY, true) ?? PresenceStatus.ONLINE;
    // Freeze the legacy choice per account, including invisible. Future choices
    // never write the old key or change any other account's migration fallback.
    if (stored === null) this.slot.set(this.status);
  }

  /** Server state is authoritative; local storage is only a migration fallback. */
  accept(status: PresenceStatus, revision: string) {
    this.revision = revision;
    this.ready = true;
    // Unknown server values must not display as Online or seed an Online choice.
    this.status =
      status >= PresenceStatus.ONLINE && status <= PresenceStatus.OFFLINE
        ? status
        : PresenceStatus.OFFLINE;
    this.slot.set(this.status);
    this.migrated.set(true);
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
