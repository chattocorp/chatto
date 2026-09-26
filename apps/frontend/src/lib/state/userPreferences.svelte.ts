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
import { Capacitor } from '@capacitor/core';
import { MediaQuery } from 'svelte/reactivity';
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

/**
 * Curated neutral ramps for backgrounds, surfaces, borders, and text. Keep the
 * first-paint allowlist in app.html and the CSS tone blocks in app.css in sync.
 */
export const surfaceTones = [
  'neutral',
  'stone',
  'taupe',
  'clay',
  'olive',
  'forest',
  'mist',
  'gray',
  'slate',
  'midnight',
  'mauve',
  'plum'
] as const;

export type SurfaceTone = (typeof surfaceTones)[number];

/** Defaults reproduce the original palette: cool gray in light, pure neutral in dark. */
export const defaultSurfaceTones: Readonly<Record<EffectiveTheme, SurfaceTone>> = {
  light: 'gray',
  dark: 'neutral'
};

/** Only known tone names may select the application surface ramp. */
export function isSurfaceTone(value: unknown): value is SurfaceTone {
  return typeof value === 'string' && surfaceTones.includes(value as SurfaceTone);
}

/**
 * Apply both tone choices. CSS selects the one that matches the active theme,
 * so theme changes need no further call.
 */
export function applySurfaceTones(tones: Record<EffectiveTheme, SurfaceTone>): void {
  if (typeof document === 'undefined') return;
  const root = document.documentElement;
  root.dataset.lightTone = isSurfaceTone(tones.light) ? tones.light : defaultSurfaceTones.light;
  root.dataset.darkTone = isSurfaceTone(tones.dark) ? tones.dark : defaultSurfaceTones.dark;
  syncShellColor();
  syncLoadingPalettes();
}

/** Resolved tone colours that app.html paints before the stylesheet loads. */
export interface LoadingPalette {
  background: string;
  highlight: string;
  text: string;
  /** Base surface, used as the first-paint browser theme colour. */
  surface: string;
}

type LoadingPalettes = Record<EffectiveTheme, LoadingPalette>;

/** Tone steps behind each startup colour. Keep them aligned with app.html. */
const loadingPaletteSteps: Readonly<Record<EffectiveTheme, Record<keyof LoadingPalette, number>>> =
  {
    light: { background: 100, highlight: 400, text: 600, surface: 200 },
    dark: { background: 900, highlight: 700, text: 300, surface: 800 }
  };

const hexColorPattern = /^#[0-9a-f]{6}$/;

function isLoadingPalettes(value: unknown): value is LoadingPalettes {
  return (['light', 'dark'] as const).every((theme) => {
    const palette = isRecord(value) ? value[theme] : undefined;
    return (
      isRecord(palette) &&
      Object.keys(loadingPaletteSteps[theme]).every((name) => {
        const color = palette[name];
        return typeof color === 'string' && hexColorPattern.test(color);
      })
    );
  });
}

/**
 * app.html reads this slot directly, so keep its key and shape stable. The
 * stored colours are derived from the tones and the stylesheet, not user input.
 */
const loadingPaletteSlot = globalSlot<LoadingPalettes | null>(
  'loading-palette',
  null,
  Codecs.json<LoadingPalettes | null>(isLoadingPalettes)
);

/** Read the saved startup colours, for example to check what app.html will paint. */
export function getLoadingPalettes(): LoadingPalettes | null {
  return loadingPaletteSlot.get();
}

/**
 * Resolve the startup colours of both chosen tones through a hidden sample,
 * the same scope that the tone picker uses. Does nothing before the stylesheet
 * defines the tone ramp.
 */
function syncLoadingPalettes(): void {
  const root = document.documentElement;
  const probe = document.createElement('span');
  probe.hidden = true;
  (document.body ?? root).append(probe);
  try {
    const palettes = {} as LoadingPalettes;
    for (const theme of ['light', 'dark'] as const) {
      probe.dataset.toneTheme = theme;
      probe.dataset.tone = theme === 'dark' ? root.dataset.darkTone : root.dataset.lightTone;
      const palette = {} as LoadingPalette;
      for (const [name, step] of Object.entries(loadingPaletteSteps[theme])) {
        if (!getComputedStyle(probe).getPropertyValue(`--tone-${step}`).trim()) return;
        probe.style.color = `var(--tone-${step})`;
        const color = resolvedHexColor(getComputedStyle(probe).color);
        if (!color) return;
        palette[name as keyof LoadingPalette] = color;
      }
      palettes[theme] = palette;
    }
    loadingPaletteSlot.set(palettes);
  } finally {
    probe.remove();
  }
}

/**
 * App-wide bevel strength in percent, in 10% steps. 0, 50, and 100 match the
 * former Flat, Kinda 3D, and Very 3D modes. Keep app.html in sync.
 */
export const surfaceDepthStep = 10;
/** Native iOS starts flat; browser/PWA and other hosts retain the Kinda 3D default. */
export const defaultSurfaceDepth = Capacitor.getPlatform() === 'ios' ? 0 : 50;

/** Former named modes, migrated from earlier saved preferences. */
const legacySurfaceDepths: Readonly<Record<string, number>> = {
  flat: 0,
  '3d': 50,
  'very-3d': 100
};

/** Reject unknown levels from stored preferences or external callers. */
export function isSurfaceDepth(value: unknown): value is number {
  return (
    typeof value === 'number' &&
    Number.isInteger(value) &&
    value >= 0 &&
    value <= 100 &&
    value % surfaceDepthStep === 0
  );
}

/** Read a saved depth level, migrating the former named modes. */
function storedSurfaceDepth(value: unknown): number {
  if (isSurfaceDepth(value)) return value;
  if (typeof value === 'string' && Object.hasOwn(legacySurfaceDepths, value)) {
    return legacySurfaceDepths[value];
  }
  return defaultSurfaceDepth;
}

/** Apply bevel strength without changing the palette or light/dark theme. */
export function applySurfaceDepth(value: number): void {
  if (typeof document === 'undefined') return;
  const level = isSurfaceDepth(value) ? value : defaultSurfaceDepth;
  document.documentElement.style.setProperty('--depth-level', String(level));
}

/** Keep the saved 20–40 scale so existing browser choices survive the UI label change. */
export const defaultContrastAge = 30;
/** The slider moves in 10% steps; saved half steps from earlier versions stay valid. */
export const contrastAgeStep = 2;

/** Reject invalid values from storage and callers before applying CSS percentages. */
export function isContrastAge(value: unknown): value is number {
  return (
    typeof value === 'number' &&
    Number.isFinite(value) &&
    value >= 20 &&
    value <= 40 &&
    Number.isInteger(value * 2)
  );
}

/** Apply the two palette mixes without changing theme, accent, or depth. */
export function applyContrastAge(value: number): void {
  if (typeof document === 'undefined') return;
  const age = isContrastAge(value) ? value : defaultContrastAge;
  const root = document.documentElement;
  root.style.setProperty('--contrast-soft-mix', `${Math.max(0, 30 - age) * 10}%`);
  root.style.setProperty('--contrast-strong-mix', `${Math.max(0, age - 30) * 10}%`);
  syncShellColor();
}

/** Keep the browser frame and system theme colour aligned with the active palette. */
function syncShellColor(): void {
  const root = document.documentElement;
  root.style.backgroundColor = 'var(--color-surface)';
  const dark = root.dataset.theme === 'dark';
  const veryHigh = root.style.getPropertyValue('--contrast-strong-mix') === '100%';
  // The fallback matches the default tones before the stylesheet is available.
  const shellColor =
    resolvedHexColor(getComputedStyle(root).backgroundColor) ??
    (dark ? (veryHigh ? '#000000' : '#262626') : veryHigh ? '#ffffff' : '#e5e7eb');
  document
    .querySelector<HTMLMetaElement>('meta[name="theme-color"]')
    ?.setAttribute('content', shellColor);
}

/**
 * Convert a computed CSS colour to `#rrggbb` for `theme-color`, which some
 * browsers only accept in legacy sRGB syntax. Returns null for transparent or
 * unresolvable colours, such as before the stylesheet loads.
 */
function resolvedHexColor(color: string): string | null {
  if (!color || color === 'transparent' || color.includes('var(')) return null;
  const context = document.createElement('canvas').getContext('2d', { willReadFrequently: true });
  if (!context) return null;
  context.fillStyle = color;
  context.fillRect(0, 0, 1, 1);
  const [red, green, blue, alpha] = context.getImageData(0, 0, 1, 1).data;
  if (alpha < 255) return null;
  return `#${[red, green, blue].map((channel) => channel.toString(16).padStart(2, '0')).join('')}`;
}

export type DisplayTheme = 'system' | 'light' | 'dark';
export type ComposerEditorKind = 'visual' | 'markdown';
export type ComposerSendMode = 'enter' | 'modifier-enter';
export type ThreadPanePresentation = 'overlay' | 'split';
export type EffectiveTheme = 'light' | 'dark';

interface AppPreferences {
  displayTheme: DisplayTheme;
  accentColor: AccentColor;
  /** Surface tone for light appearance. */
  lightSurfaceTone: SurfaceTone;
  /** Surface tone for dark appearance. */
  darkSurfaceTone: SurfaceTone;
  /** Bevel strength from 0 to 100 percent. */
  surfaceDepth: number;
  /** Neutral palette contrast; 30 preserves the original light and dark colours. */
  contrastAge: number;
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
  lightSurfaceTone: defaultSurfaceTones.light,
  darkSurfaceTone: defaultSurfaceTones.dark,
  surfaceDepth: defaultSurfaceDepth,
  contrastAge: defaultContrastAge,
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
  root.style.colorScheme = effective;
  syncShellColor();
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
    lightSurfaceTone: isSurfaceTone(stored.lightSurfaceTone)
      ? stored.lightSurfaceTone
      : defaultSurfaceTones.light,
    darkSurfaceTone: isSurfaceTone(stored.darkSurfaceTone)
      ? stored.darkSurfaceTone
      : defaultSurfaceTones.dark,
    surfaceDepth: storedSurfaceDepth(stored.surfaceDepth),
    contrastAge: isContrastAge(stored.contrastAge) ? stored.contrastAge : defaultContrastAge,
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
  readonly #prefersDark = new MediaQuery('(prefers-color-scheme: dark)', false);
  // Keep the legacy fields intact whenever App Preferences are saved so a
  // server first opened later can still migrate the user's previous sound.
  readonly #legacyNotificationSoundPreferences = getLegacyNotificationSoundPreferences();

  constructor() {
    // The HTML bootstrap handles first paint. Reapply the saved value when
    // the client store starts so hydration cannot leave the palette at default.
    applyContrastAge(this.#preferences.contrastAge);
    applySurfaceDepth(this.#preferences.surfaceDepth);
    this.#applySurfaceTones();
    // app.html swaps the theme on system changes; resolve the shell colour
    // afterwards because only the loaded stylesheet knows the tone's surface.
    // Palette changes fade, so resolve it again once the surface settles.
    if (typeof window !== 'undefined') {
      window
        .matchMedia('(prefers-color-scheme: dark)')
        .addEventListener('change', () => syncShellColor());
      const root = document.documentElement;
      root.addEventListener('transitionend', (event) => {
        if (event.target === root && event.propertyName === '--color-surface') syncShellColor();
      });
    }
  }

  get displayTheme(): DisplayTheme {
    return this.#preferences.displayTheme;
  }

  set displayTheme(value: DisplayTheme) {
    const displayTheme = isDisplayTheme(value) ? value : defaultAppPreferences.displayTheme;
    this.#preferences.displayTheme = displayTheme;
    this.#persist();
    applyDisplayTheme(displayTheme);
  }

  /** The applied theme; follows system changes reactively while set to System. */
  get effectiveDisplayTheme(): EffectiveTheme {
    const theme = this.#preferences.displayTheme;
    if (theme !== 'system') return theme;
    return this.#prefersDark.current ? 'dark' : 'light';
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

  /** Surface tone used while the light theme is active. */
  get lightSurfaceTone(): SurfaceTone {
    return this.#preferences.lightSurfaceTone;
  }

  set lightSurfaceTone(value: SurfaceTone) {
    this.#preferences.lightSurfaceTone = isSurfaceTone(value) ? value : defaultSurfaceTones.light;
    this.#persist();
    this.#applySurfaceTones();
  }

  /** Surface tone used while the dark theme is active. */
  get darkSurfaceTone(): SurfaceTone {
    return this.#preferences.darkSurfaceTone;
  }

  set darkSurfaceTone(value: SurfaceTone) {
    this.#preferences.darkSurfaceTone = isSurfaceTone(value) ? value : defaultSurfaceTones.dark;
    this.#persist();
    this.#applySurfaceTones();
  }

  /** Bevel strength shared by all servers in this browser. */
  get surfaceDepth(): number {
    return this.#preferences.surfaceDepth;
  }

  set surfaceDepth(value: number) {
    const depth = isSurfaceDepth(value) ? value : defaultSurfaceDepth;
    this.#preferences.surfaceDepth = depth;
    this.#persist();
    applySurfaceDepth(depth);
  }

  /** App-wide palette contrast; 30 preserves the original appearance. */
  get contrastAge(): number {
    return this.#preferences.contrastAge;
  }

  set contrastAge(value: number) {
    const age = isContrastAge(value) ? value : defaultContrastAge;
    this.#preferences.contrastAge = age;
    this.#persist();
    applyContrastAge(age);
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

  #applySurfaceTones() {
    applySurfaceTones({
      light: this.#preferences.lightSurfaceTone,
      dark: this.#preferences.darkSurfaceTone
    });
  }

  #persist() {
    slot.set({
      ...this.#preferences,
      ...this.#legacyNotificationSoundPreferences
    });
  }
}

export const userPreferences = new UserPreferencesState();
