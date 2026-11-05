/** Native Chatto tokens. Keep semantic names aligned with the web theme. */
import { get_native, register_font, set_css_vars } from 'gpuix-svelte';
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

export const inputTheme = {
  appearance: 'dark' as const,
  bg: colors.surface,
  text: colors.text,
  accent: colors.action,
  fontSans: 'IBM Plex Sans',
  fontMono: 'IBM Plex Mono'
};
export const markdownTheme = {
  appearance: 'dark' as const,
  bg: colors.background,
  text: colors.text,
  fontSans: 'IBM Plex Sans',
  fontMono: 'IBM Plex Mono',
  accent: colors.action,
  codeText: colors.text,
  codeWash: colors.surfaceEmphasized
};
const registered = new WeakSet<object>();

/** Register bundled fonts before text layout, and apply the process-wide GPUix palette. */
export function applyTheme(): void {
  const native = get_native();
  if (native && !registered.has(native)) {
    for (const font of Object.values(plex)) register_font(Buffer.from(font, 'base64'));
    registered.add(native);
  }
  set_css_vars({
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
    'radius-control': '5px',
    'radius-panel': '10px'
  });
}
