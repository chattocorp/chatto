/**
 * App Preferences store.
 *
 * Stores user preferences in localStorage for persistence across sessions.
 * These are app-local preferences that don't need server sync.
 */

import {
  type NotificationSoundFilters,
  type NotificationSoundId,
  defaultNotificationSoundFilters,
  defaultSoundId,
  notificationSounds
} from '$lib/audio/notificationSounds';
import { Codecs, globalSlot } from '$lib/storage/slot';

export type DisplayTheme = 'system' | 'light' | 'dark';
export type ComposerEditorKind = 'visual' | 'markdown';
export type ComposerSendMode = 'enter' | 'modifier-enter';
type EffectiveTheme = 'light' | 'dark';

interface AppPreferences {
  displayTheme: DisplayTheme;
  composerEditor: ComposerEditorKind;
  composerSendMode: ComposerSendMode;
  composerFormattingToolbarVisible: boolean;
}

export interface LegacyNotificationSoundPreferences {
  notificationSound: NotificationSoundId;
  notificationSoundFilters: NotificationSoundFilters;
}

interface StoredPreferences extends AppPreferences, LegacyNotificationSoundPreferences {}

const defaultAppPreferences: AppPreferences = {
  displayTheme: 'system',
  composerEditor: 'markdown',
  composerSendMode: 'enter',
  composerFormattingToolbarVisible: false
};

const defaultStoredPreferences: StoredPreferences = {
  ...defaultAppPreferences,
  notificationSound: defaultSoundId,
  notificationSoundFilters: defaultNotificationSoundFilters
};

const slot = globalSlot('preferences', defaultStoredPreferences, Codecs.json<StoredPreferences>());

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function clampNumber(value: unknown, min: number, max: number, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback;
  if (value < min || value > max) return fallback;
  return value;
}

function isDisplayTheme(value: unknown): value is DisplayTheme {
  return value === 'system' || value === 'light' || value === 'dark';
}

function isComposerEditorKind(value: unknown): value is ComposerEditorKind {
  return value === 'visual' || value === 'markdown';
}

function isComposerSendMode(value: unknown): value is ComposerSendMode {
  return value === 'enter' || value === 'modifier-enter';
}

function getLegacyDisplayTheme(): DisplayTheme | null {
  if (typeof localStorage === 'undefined') return null;
  try {
    const legacy = localStorage.getItem('theme');
    return isDisplayTheme(legacy) && legacy !== 'system' ? legacy : null;
  } catch {
    return null;
  }
}

function getStoredDisplayTheme(): DisplayTheme | null {
  if (typeof localStorage === 'undefined') return null;
  try {
    const raw = localStorage.getItem(slot.key);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (!isRecord(parsed)) return null;
    return isDisplayTheme(parsed.displayTheme) ? parsed.displayTheme : null;
  } catch {
    return null;
  }
}

export function resolveDisplayTheme(theme: DisplayTheme): EffectiveTheme {
  if (theme === 'light' || theme === 'dark') return theme;
  if (typeof window === 'undefined') return 'light';
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function applyDisplayTheme(theme: DisplayTheme): void {
  if (typeof document === 'undefined') return;
  const effective = resolveDisplayTheme(theme);
  const root = document.documentElement;
  root.dataset.theme = effective;
  root.style.backgroundColor = effective === 'dark' ? '#171717' : '#f3f4f6';
  root.style.colorScheme = effective;
}

function normalizeNotificationSoundFilters(value: unknown): NotificationSoundFilters {
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

function loadAppPreferences(): AppPreferences {
  const stored = slot.get();
  const displayTheme =
    getStoredDisplayTheme() ?? getLegacyDisplayTheme() ?? defaultAppPreferences.displayTheme;
  return {
    displayTheme,
    composerEditor: isComposerEditorKind(stored.composerEditor)
      ? stored.composerEditor
      : defaultAppPreferences.composerEditor,
    composerSendMode: isComposerSendMode(stored.composerSendMode)
      ? stored.composerSendMode
      : defaultAppPreferences.composerSendMode,
    composerFormattingToolbarVisible:
      typeof stored.composerFormattingToolbarVisible === 'boolean'
        ? stored.composerFormattingToolbarVisible
        : defaultAppPreferences.composerFormattingToolbarVisible
  };
}

/**
 * Read the former global notification-sound fields while per-server slots are
 * being established. New runtime code must use ServerNotificationPreferences.
 */
export function getLegacyNotificationSoundPreferences(): LegacyNotificationSoundPreferences {
  const stored = slot.get();
  const isValidSound = notificationSounds.some((sound) => sound.id === stored.notificationSound);
  return {
    notificationSound: isValidSound ? stored.notificationSound : defaultSoundId,
    notificationSoundFilters: normalizeNotificationSoundFilters(stored.notificationSoundFilters)
  };
}

export class UserPreferencesState {
  #preferences = $state<AppPreferences>(loadAppPreferences());
  // Keep the legacy fields intact whenever App Preferences are saved so a
  // server first opened later can still migrate the user's previous sound.
  readonly #legacyNotificationSoundPreferences = getLegacyNotificationSoundPreferences();

  get displayTheme(): DisplayTheme {
    return this.#preferences.displayTheme;
  }

  set displayTheme(value: DisplayTheme) {
    const displayTheme = isDisplayTheme(value) ? value : defaultAppPreferences.displayTheme;
    this.#preferences.displayTheme = displayTheme;
    this.#persist();
    applyDisplayTheme(displayTheme);
  }

  get effectiveDisplayTheme(): EffectiveTheme {
    return resolveDisplayTheme(this.#preferences.displayTheme);
  }

  get composerEditor(): ComposerEditorKind {
    return this.#preferences.composerEditor;
  }

  set composerEditor(value: ComposerEditorKind) {
    this.#preferences.composerEditor = isComposerEditorKind(value)
      ? value
      : defaultAppPreferences.composerEditor;
    this.#persist();
  }

  get composerSendMode(): ComposerSendMode {
    return this.#preferences.composerSendMode;
  }

  set composerSendMode(value: ComposerSendMode) {
    this.#preferences.composerSendMode = isComposerSendMode(value)
      ? value
      : defaultAppPreferences.composerSendMode;
    this.#persist();
  }

  /** Whether message formatting controls stay visible above each composer. */
  get composerFormattingToolbarVisible(): boolean {
    return this.#preferences.composerFormattingToolbarVisible;
  }

  set composerFormattingToolbarVisible(value: boolean) {
    this.#preferences.composerFormattingToolbarVisible =
      typeof value === 'boolean' ? value : defaultAppPreferences.composerFormattingToolbarVisible;
    this.#persist();
  }

  #persist() {
    slot.set({
      ...this.#preferences,
      ...this.#legacyNotificationSoundPreferences
    });
  }
}

export const userPreferences = new UserPreferencesState();
