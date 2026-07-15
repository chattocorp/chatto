function mediaMatches(query: string): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false;
  return window.matchMedia(query).matches;
}

/**
 * True when the primary pointer is coarse. Use this for behavior that should
 * avoid surprising touch-primary users, such as autofocus that opens a virtual
 * keyboard or Enter-to-send in the composer.
 */
export function prefersTouchActions(): boolean {
  return mediaMatches('(pointer: coarse)');
}

/**
 * True when at least one available input can use a fine pointer.
 */
export function supportsAnyFinePointer(): boolean {
  return mediaMatches('(any-pointer: fine)') || mediaMatches('(pointer: fine)');
}

/**
 * True when the current device can reasonably use hover-only affordances such
 * as message action toolbars. This intentionally does not depend on viewport
 * width; a small desktop window should still get desktop input behavior.
 */
export function supportsHoverActions(): boolean {
  return (
    (mediaMatches('(any-hover: hover)') && supportsAnyFinePointer()) ||
    mediaMatches('(hover: hover) and (pointer: fine)')
  );
}
