import { addons } from 'storybook/manager-api';
import { themes } from 'storybook/theming';

// Storybook ships light/dark presets but no auto-switcher. Pick the matching
// preset from the OS preference at load time so the manager UI lines up
// with the docs chrome (set the same way in preview.ts).
const prefersDark =
  typeof window !== 'undefined' &&
  window.matchMedia?.('(prefers-color-scheme: dark)').matches;

addons.setConfig({
  theme: prefersDark ? themes.dark : themes.light
});

// Keyboard shortcut: Shift+T cycles the theme toolbar Auto → Light → Dark → Auto.
// Manager-side keydown handles focus in the sidebar/toolbar/addon panel; the
// preview iframe forwards Shift+T over the addons channel from
// `preview-head.html`.
const THEME_ORDER = ['auto', 'light', 'dark'] as const;
type Theme = (typeof THEME_ORDER)[number];

addons.register('theme-cycle', (api) => {
  function isShiftT(e: { key?: string; shiftKey?: boolean; metaKey?: boolean; ctrlKey?: boolean; altKey?: boolean }): boolean {
    if (!e.shiftKey || e.metaKey || e.ctrlKey || e.altKey) return false;
    return e.key?.toLowerCase() === 't';
  }

  function isTypingTarget(target: EventTarget | null): boolean {
    if (!(target instanceof HTMLElement)) return false;
    if (target.isContentEditable) return true;
    const tag = target.tagName;
    return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT';
  }

  function cycle() {
    const globals = api.getGlobals();
    const current = (globals.theme as Theme) ?? 'auto';
    const idx = THEME_ORDER.indexOf(current);
    const next = THEME_ORDER[(idx + 1) % THEME_ORDER.length];
    api.updateGlobals({ theme: next });
  }

  document.addEventListener('keydown', (e) => {
    if (!isShiftT(e) || isTypingTarget(e.target)) return;
    e.preventDefault();
    cycle();
  });

  addons.getChannel().on('theme-cycle/request', cycle);
});
