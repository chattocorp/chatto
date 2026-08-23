/**
 * Client-rendered notification preferences scoped to one registered server.
 *
 * Notification delivery is already server-synced. Sound playback still happens
 * in this client, but its choice follows the server that produced the
 * notification instead of applying globally across every registered server.
 */

import {
  type NotificationSoundFilters,
  type NotificationSoundId,
  defaultNotificationSoundFilters,
  defaultSoundId,
  notificationSounds
} from '$lib/audio/notificationSounds';
import { Codecs, serverSlot, type StorageSlot } from '$lib/storage/slot';
import { getLegacyNotificationSoundPreferences } from '$lib/state/userPreferences.svelte';
import { SvelteMap } from 'svelte/reactivity';

interface ServerNotificationPreferences {
  notificationSound: NotificationSoundId;
  notificationSoundFilters: NotificationSoundFilters;
}

const states = new SvelteMap<string, ServerNotificationPreferencesState>();

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function clampNumber(value: unknown, min: number, max: number, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback;
  if (value < min || value > max) return fallback;
  return value;
}

function normalizeFilters(value: unknown): NotificationSoundFilters {
  const stored = isRecord(value) ? value : {};
  return {
    volume: clampNumber(stored.volume, 0, 2, defaultNotificationSoundFilters.volume),
    highPassHz: clampNumber(
      stored.highPassHz,
      20,
      2000,
      defaultNotificationSoundFilters.highPassHz
    ),
    lowPassHz: clampNumber(stored.lowPassHz, 800, 20000, defaultNotificationSoundFilters.lowPassHz),
    echo: clampNumber(stored.echo, 0, 100, defaultNotificationSoundFilters.echo),
    reverb: clampNumber(stored.reverb, 0, 100, defaultNotificationSoundFilters.reverb),
    crunch: clampNumber(stored.crunch, 0, 100, defaultNotificationSoundFilters.crunch)
  };
}

function normalize(value: unknown, fallback: ServerNotificationPreferences) {
  const stored = isRecord(value) ? value : {};
  const notificationSound = notificationSounds.some(
    (sound) => sound.id === stored.notificationSound
  )
    ? (stored.notificationSound as NotificationSoundId)
    : fallback.notificationSound;

  return {
    notificationSound,
    notificationSoundFilters: normalizeFilters(stored.notificationSoundFilters)
  };
}

export class ServerNotificationPreferencesState {
  readonly #slot: StorageSlot<ServerNotificationPreferences>;
  #preferences: ServerNotificationPreferences;

  constructor(serverId: string) {
    // Seed a new server slot from the former global preference so upgrading
    // users keep the sound they selected before notification sounds became
    // server-specific.
    const migrationFallback = getLegacyNotificationSoundPreferences();
    this.#slot = serverSlot(
      serverId,
      'notificationPreferences',
      migrationFallback,
      Codecs.json<ServerNotificationPreferences>()
    );
    this.#preferences = $state(normalize(this.#slot.get(), migrationFallback));
    this.#slot.set(this.#preferences);
  }

  get notificationSound(): NotificationSoundId {
    return this.#preferences.notificationSound;
  }

  set notificationSound(value: NotificationSoundId) {
    this.#preferences.notificationSound = notificationSounds.some((sound) => sound.id === value)
      ? value
      : defaultSoundId;
    this.#persist();
  }

  get notificationSoundFilters(): NotificationSoundFilters {
    return this.#preferences.notificationSoundFilters;
  }

  set notificationSoundFilters(value: NotificationSoundFilters) {
    this.#preferences.notificationSoundFilters = normalizeFilters(value);
    this.#persist();
  }

  setNotificationSoundFilter(key: keyof NotificationSoundFilters, value: number) {
    this.notificationSoundFilters = {
      ...this.#preferences.notificationSoundFilters,
      [key]: value
    };
  }

  resetNotificationSoundFilters() {
    this.notificationSoundFilters = defaultNotificationSoundFilters;
  }

  get isMuted(): boolean {
    return this.#preferences.notificationSound === 'silent';
  }

  #persist() {
    this.#slot.set(this.#preferences);
  }
}

export function getServerNotificationPreferences(
  serverId: string
): ServerNotificationPreferencesState {
  let state = states.get(serverId);
  if (!state) {
    state = new ServerNotificationPreferencesState(serverId);
    states.set(serverId, state);
  }
  return state;
}

/** Test-only cache reset so localStorage scenarios remain isolated. */
export function resetServerNotificationPreferencesForTests(): void {
  states.clear();
}
