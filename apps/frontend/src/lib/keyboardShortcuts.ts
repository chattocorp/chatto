/** Component-owned shortcut descriptions, scoped to one app root. */
import { createContext } from 'svelte';
import { usesAppleShortcutModifier } from '$lib/utils/platformShortcuts';

/** One binding displayed in shortcut help. Editor bindings remain owned by their editor. */
export type ShortcutDescription = {
  label: string;
  key: string;
  command?: boolean;
  alt?: boolean;
  shift?: boolean;
  group: 'app' | 'editor';
};

/** Keep live getters so opening help reads current props, locale, and eligibility. */
export class KeyboardShortcutRegistry {
  readonly #sources = new Set<() => ShortcutDescription[]>();

  /** Register a mounted component's descriptions; the returned function removes them. */
  register(source: () => ShortcutDescription[]): () => void {
    this.#sources.add(source);
    return () => {
      this.#sources.delete(source);
    };
  }

  /** Snapshot before the dialog moves focus; duplicate room/thread composers share rows. */
  snapshot(): ShortcutDescription[] {
    const unique = new Map<string, ShortcutDescription>();
    for (const source of this.#sources) {
      for (const entry of source()) {
        unique.set(JSON.stringify(entry), entry);
      }
    }
    return [...unique.values()];
  }
}

const [getRegistry, setRegistry, hasRegistry] = createContext<KeyboardShortcutRegistry>();

/** Provide one catalogue to the app shell and its routes. */
export function provideKeyboardShortcuts(): KeyboardShortcutRegistry {
  return setRegistry(new KeyboardShortcutRegistry());
}

/** Standalone controls work without an app help catalogue, including in stories. */
export function getKeyboardShortcuts(): KeyboardShortcutRegistry | undefined {
  return hasRegistry() ? getRegistry() : undefined;
}

/** Display the platform's modifier names without changing which modifiers are accepted. */
export function shortcutKeys(shortcut: ShortcutDescription): string[] {
  const apple = usesAppleShortcutModifier();
  return [
    ...(shortcut.command ? [apple ? '⌘' : 'Ctrl'] : []),
    ...(shortcut.alt ? [apple ? '⌥' : 'Alt'] : []),
    ...(shortcut.shift ? ['⇧'] : []),
    shortcut.key === 'ArrowUp'
      ? '↑'
      : shortcut.key === 'ArrowDown'
        ? '↓'
        : shortcut.key.toUpperCase()
  ];
}
