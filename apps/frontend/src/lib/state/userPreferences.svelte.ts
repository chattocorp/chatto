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
/** Curated app-wide accents. Keep the first-paint allowlist in app.html in sync. */
export const accentColors = [
  'blue',
  'cyan',
  'teal',
  'green',
  'amber',
  'orange',
  'pink',
  'violet',
  'grey'
] as const;

export type AccentColor = (typeof accentColors)[number];

/** Preserve the familiar cyan when the stored choice is absent or invalid. */
export const defaultAccentColor: AccentColor = 'cyan';

/** Only known palette names may select an application accent. */
export function isAccentColor(value: unknown): value is AccentColor {
  return typeof value === 'string' && accentColors.includes(value as AccentColor);
}

/** Apply the palette without changing the user's light/dark theme. */
export function applyAccentColor(value: AccentColor): void {
  if (typeof document === 'undefined') return;
  document.documentElement.dataset.accent = isAccentColor(value) ? value : defaultAccentColor;
}

/** App-wide bevel modes. Keep the first-paint allowlist in app.html in sync. */
export const surfaceDepths = ['flat', '3d', 'very-3d'] as const;
export type SurfaceDepth = (typeof surfaceDepths)[number];
export const defaultSurfaceDepth: SurfaceDepth = '3d';

/** Reject unknown modes from stored preferences or external callers. */
export function isSurfaceDepth(value: unknown): value is SurfaceDepth {
  return typeof value === 'string' && surfaceDepths.includes(value as SurfaceDepth);
}

/** Apply bevel strength without changing the palette or light/dark theme. */
export function applySurfaceDepth(value: SurfaceDepth): void {
  if (typeof document === 'undefined') return;
  document.documentElement.dataset.depth = isSurfaceDepth(value) ? value : defaultSurfaceDepth;
}

export type DisplayTheme = 'system' | 'light' | 'dark';
export type ComposerEditorKind = 'visual' | 'markdown';
export type ComposerSendMode = 'enter' | 'modifier-enter';
export type ThreadPanePresentation = 'overlay' | 'split';
type EffectiveTheme = 'light' | 'dark';

interface AppPreferences {
  displayTheme: DisplayTheme;
  accentColor: AccentColor;
  surfaceDepth: SurfaceDepth;
  composerEditor: ComposerEditorKind;
  composerSendMode: ComposerSendMode;
  composerFormattingToolbarVisible: boolean;
  threadPanePresentation: ThreadPanePresentation;
}

export interface LegacyNotificationSoundPreferences {
  notificationSound: NotificationSoundId;
  notificationSoundFilters: NotificationSoundFilters;
}

interface StoredPreferences extends AppPreferences, LegacyNotificationSoundPreferences {}

const defaultAppPreferences: AppPreferences = {
  displayTheme: 'system',
  accentColor: defaultAccentColor,
  surfaceDepth: defaultSurfaceDepth,
  composerEditor: 'markdown',
  composerSendMode: 'enter',
  composerFormattingToolbarVisible: false,
  threadPanePresentation: 'overlay'
};

const defaultStoredPreferences: StoredPreferences = {
  ...defaultAppPreferences,
  notificationSound: defaultSoundId,
  notificationSoundFilters: defaultNotificationSoundFilters
};

const slot = globalSlot(
  'preferences',
  defaultStoredPreferences,
  Codecs.json<StoredPreferences>((value): value is StoredPreferences => isRecord(value))
);

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

function isThreadPanePresentation(value: unknown): value is ThreadPanePresentation {
  return value === 'overlay' || value === 'split';
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
  // Keep the system chrome's page-background sample aligned with the app frame.
  root.style.backgroundColor = effective === 'dark' ? '#262626' : '#e5e7eb';
  root.style.colorScheme = effective;
  document
    .querySelector<HTMLMetaElement>('meta[name="theme-color"]')
    ?.setAttribute('content', effective === 'dark' ? '#262626' : '#e5e7eb');
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
    accentColor: isAccentColor(stored.accentColor) ? stored.accentColor : defaultAccentColor,
    surfaceDepth: isSurfaceDepth(stored.surfaceDepth) ? stored.surfaceDepth : defaultSurfaceDepth,
    composerEditor: isComposerEditorKind(stored.composerEditor)
      ? stored.composerEditor
      : defaultAppPreferences.composerEditor,
    composerSendMode: isComposerSendMode(stored.composerSendMode)
      ? stored.composerSendMode
      : defaultAppPreferences.composerSendMode,
    composerFormattingToolbarVisible:
      typeof stored.composerFormattingToolbarVisible === 'boolean'
        ? stored.composerFormattingToolbarVisible
        : defaultAppPreferences.composerFormattingToolbarVisible,
    threadPanePresentation: isThreadPanePresentation(stored.threadPanePresentation)
      ? stored.threadPanePresentation
      : defaultAppPreferences.threadPanePresentation
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

  /** Accent shared by every registered server in this browser. */
  get accentColor(): AccentColor {
    return this.#preferences.accentColor;
  }

  set accentColor(value: AccentColor) {
    const accentColor = isAccentColor(value) ? value : defaultAccentColor;
    this.#preferences.accentColor = accentColor;
    this.#persist();
    applyAccentColor(accentColor);
  }

  /** Bevel strength shared by all servers in this browser. */
  get surfaceDepth(): SurfaceDepth {
    return this.#preferences.surfaceDepth;
  }

  set surfaceDepth(value: SurfaceDepth) {
    const depth = isSurfaceDepth(value) ? value : defaultSurfaceDepth;
    this.#preferences.surfaceDepth = depth;
    this.#persist();
    applySurfaceDepth(depth);
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

  /** How an open thread uses the available room area. */
  get threadPanePresentation(): ThreadPanePresentation {
    return this.#preferences.threadPanePresentation;
  }

  set threadPanePresentation(value: ThreadPanePresentation) {
    this.#preferences.threadPanePresentation = isThreadPanePresentation(value)
      ? value
      : defaultAppPreferences.threadPanePresentation;
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
