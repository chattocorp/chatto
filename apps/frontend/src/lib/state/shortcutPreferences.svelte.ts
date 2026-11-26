/** Device-local keyboard bindings shared by preferences, shortcut help, and handlers. */
import { Codecs, globalSlot } from '@chatto/client/storage/slot';
import { m } from '$lib/i18n/messages';
import { usesAppleShortcutModifier } from '$lib/utils/platformShortcuts';

export const defaultBindings = {
  switcher: 'CommandOrControl+K',
  help: 'CommandOrControl+Shift+K',
  settings: 'CommandOrControl+,',
  unread: 'Alt+Shift+Down',
  submit: 'CommandOrControl+Enter',
  search: 'CommandOrControl+/',
  mute: ''
} as const;
export type ShortcutAction = keyof typeof defaultBindings;
export const appShortcutActions = [
  'switcher',
  'help',
  'settings',
  'unread',
  'submit',
  'search'
] as const;

/** Localised action name used in App Preferences. */
export function shortcutLabel(action: ShortcutAction): string {
  const labels = {
    switcher: 'ui.open_quick_switcher',
    help: 'common.keyboard_shortcuts.title',
    settings: 'settings.app_preferences.title',
    unread: 'chat.notifications.next_unread_source',
    submit: 'common.submit',
    search: 'search.action',
    mute: 'common.keyboard_shortcuts.toggle_mute'
  } as const;
  return m(labels[action]);
}

/**
 * Prefer the layout-resolved letter. Option and Shift can produce a symbol,
 * and system chords can produce Dead, Unidentified, or a control character.
 * Recover the trigger from the key position in those cases.
 */
export function shortcutEventKey(event: KeyboardEvent, systemWide = false): string {
  if (
    (systemWide || ((event.altKey || event.shiftKey) && event.key.length === 1)) &&
    !/^[a-z0-9/,]$/i.test(event.key)
  ) {
    if (/^Key[A-Z]$/.test(event.code)) return event.code.slice(3);
    if (/^Digit[0-9]$/.test(event.code)) return event.code.slice(5);
    if (event.code === 'Comma') return ',';
    if (event.code === 'Slash') return '/';
  }
  return event.key === ' ' ? 'Space' : event.key;
}

const validAppChord =
  /^(?=CommandOrControl\+|Alt\+)(?:CommandOrControl\+)?(?:Alt\+)?(?:Shift\+)?(?:[A-Z0-9/,]|Space|Enter|Up|Down|Left|Right|F(?:[1-9]|1[0-2]))$/;

// Keep the host validator in apps/desktop/keyboard_shortcuts.mjs in sync.
// CommandOrControl also accepts saved shortcuts from earlier Desktop builds.
const validSystemChord =
  /^(?=CommandOrControl\+|Control\+|Super\+|Alt\+)(?:CommandOrControl\+|(?:Control\+)?(?:Super\+)?)?(?:Alt\+)?(?:Shift\+)?(?:[A-Z0-9/,]|Space|Enter|Up|Down|Left|Right|F(?:[1-9]|1[0-2]))$/;

function validBinding(action: ShortcutAction, chord: string) {
  return (action === 'mute' ? validSystemChord : validAppChord).test(chord);
}

/** Return a canonical chord only for supported keys. Keep plain typing available. */
export function eventChord(event: KeyboardEvent, systemWide = false): string | undefined {
  // In the explicit system recorder the modifier flags describe the accelerator.
  // The input method can also flag Option as AltGraph or start composing text.
  if (
    !systemWide &&
    (event.isComposing || event.getModifierState('AltGraph') || (event.ctrlKey && event.metaKey))
  )
    return;
  const trigger = shortcutEventKey(event, systemWide);
  const key = trigger.replace(/^Arrow/, '');
  const chord = [
    systemWide && event.ctrlKey ? 'Control' : '',
    systemWide && event.metaKey ? 'Super' : '',
    !systemWide && (event.ctrlKey || event.metaKey) ? 'CommandOrControl' : '',
    event.altKey ? 'Alt' : '',
    event.shiftKey ? 'Shift' : '',
    key.length === 1 ? key.toUpperCase() : key
  ]
    .filter(Boolean)
    .join('+');
  return (systemWide ? validSystemChord : validAppChord).test(chord) ? chord : undefined;
}

function slot(action: ShortcutAction) {
  return globalSlot<string>(`shortcut:${action}`, defaultBindings[action], Codecs.string);
}

function loadBindings(): Record<ShortcutAction, string> {
  const bindings: Record<ShortcutAction, string> = { ...defaultBindings };
  for (const action of [...appShortcutActions, 'mute'] as const) {
    const saved = slot(action).get();
    if (saved === '' || validBinding(action, saved)) bindings[action] = saved;
  }
  return bindings;
}

/** In-app command shortcuts accept either Control or Meta. Check both aliases. */
function bindingsOverlap(a: string, b: string): boolean {
  return ['Control', 'Super'].some(
    (modifier) =>
      a.replace('CommandOrControl', modifier) === b.replace('CommandOrControl', modifier)
  );
}

/** An empty binding disables an action. A conflicting edit keeps the previous value. */
class ShortcutPreferences {
  bindings = $state(loadBindings());
  recording = $state(false);

  set(action: ShortcutAction, chord: string): boolean {
    if (
      chord &&
      (!validBinding(action, chord) ||
        Object.entries(this.bindings).some(
          ([id, value]) => id !== action && bindingsOverlap(value, chord)
        ))
    )
      return false;
    this.bindings[action] = chord;
    slot(action).set(chord);
    return true;
  }
}

export const shortcutPreferences = new ShortcutPreferences();

/** Props for KeyboardShortcut and its help entry. */
export function shortcutProps(chord: string) {
  const parts = chord.split('+');
  const key = parts.pop() ?? '';
  return {
    key: /^(Up|Down|Left|Right)$/.test(key) ? 'Arrow' + key : key,
    command: parts.includes('CommandOrControl'),
    alt: parts.includes('Alt'),
    shift: parts.includes('Shift')
  };
}

/** Show the key names used on this platform. */
export function bindingText(chord: string): string {
  const apple = usesAppleShortcutModifier();
  return chord
    .split('+')
    .map((part) => {
      if (part === 'Control') return apple ? '⌃' : 'Ctrl';
      if (part === 'Super') return apple ? '⌘' : 'Super';
      if (part === 'CommandOrControl') return apple ? '⌘' : 'Ctrl';
      if (part === 'Alt' && apple) return '⌥';
      return part;
    })
    .join('+');
}
