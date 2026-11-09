/** Native Chatto tokens. Keep semantic names aligned with the web theme. */
import { get_native, register_font, set_css_vars } from 'gpuix-svelte';
import { scaled } from './scale.svelte.ts';
import plex from '../../assets/plex.json' with { type: 'json' };

export const colors = {
  background: '#171717',
  surface: '#242424',
  surfaceEmphasized: '#2c2c2c',
  border: '#2a2a2a',
  text: '#d1d1d1',
  textTop: '#eeeeee',
  muted: '#969696',
  action: '#00b8e5',
  selected: '#172d33',
  error: '#efabab',
  online: '#25bb87'
} as const;

/** Logical pixel sizes shared by native widgets and Svelte text. */
export const typography = {
  body: 14,
  small: 12,
  badge: 10,
  heading: 16,
  title: 20,
  display: 24
} as const;

export const inputTheme = {
  appearance: 'dark' as const,
  bg: colors.surface,
  text: colors.text,
  accent: colors.action,
  fontSans: 'IBM Plex Sans',
  fontMono: 'IBM Plex Mono'
};
const baseMarkdownTheme = {
  appearance: 'dark' as const,
  bg: colors.background,
  text: colors.text,
  fontSans: 'IBM Plex Sans',
  fontMono: 'IBM Plex Mono',
  accent: colors.action,
  codeText: colors.text,
  codeWash: colors.surfaceEmphasized,
  metrics: {
    mdTextSize: typography.body,
    mdLineHeight: 21,
    mdBlockGap: 12,
    mdTableCellPadding: 8,
    mdTableMinColumnWidth: 120,
    mdTableMinColumnContent: 40,
    mdInlineCodeRadius: 4,
    mdCodePaddingX: 12,
    mdCodePaddingY: 8,
    mdCodeRadius: 8,
    mdCodeHeaderPaddingY: 6,
    mdCodeHeaderTextSize: 12,
    mdHeadingSizes: [24, 20, 18, 16],
    mdHeadingLineHeights: [32, 28, 26, 24],
    codeTextSize: 13,
    codeLineHeight: 20
  }
};
/** Resolve native text metrics at the current UI scale. */
export function markdownTheme() {
  return {
    ...baseMarkdownTheme,
    metrics: Object.fromEntries(
      Object.entries(baseMarkdownTheme.metrics).map(([key, value]) => [
        key,
        Array.isArray(value) ? value.map(scaled) : scaled(value)
      ])
    )
  };
}

/** Base lengths used by component CSS; tokens are updated together on zoom. */
const lengths = [
  1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 13, 14, 16, 18, 20, 24, 28, 30, 32, 35, 38, 48, 49, 50, 56, 60,
  220, 238, 246, 250, 420, 430
];
const registered = new WeakSet<object>();

/** Register bundled fonts before text layout, and apply the process-wide GPUix palette. */
export function applyTheme(): void {
  const native = get_native();
  if (native && !registered.has(native)) {
    for (const font of Object.values(plex)) register_font(Buffer.from(font, 'base64'));
    registered.add(native);
  }
  set_css_vars({
    ...Object.fromEntries(
      Object.entries(typography).map(([role, size]) => [`text-${role}`, `${scaled(size)}px`])
    ),
    ...Object.fromEntries(
      lengths.map((size) => [`size-${String(size).replace('.', '-')}`, `${scaled(size)}px`])
    ),
    'color-background': colors.background,
    'color-surface': colors.surface,
    'color-surface-emphasized': colors.surfaceEmphasized,
    'color-border': colors.border,
    'color-text': colors.text,
    'color-text-top': colors.textTop,
    'color-muted': colors.muted,
    'color-action': colors.action,
    'color-selected': colors.selected,
    'color-error': colors.error,
    'font-sans': 'IBM Plex Sans',
    'font-mono': 'IBM Plex Mono',
    'pane-header-height': `${scaled(50)}px`,
    'pane-header-padding': `${scaled(16)}px`,
    'pane-header-gap': `${scaled(10)}px`,
    'radius-control': `${scaled(5)}px`,
    'radius-panel': `${scaled(10)}px`
  });
}
