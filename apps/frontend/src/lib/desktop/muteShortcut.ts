/** Bind one call's microphone control to the optional desktop host. */
import { m } from '$lib/i18n/messages';
import { toastError } from '$lib/utils/errorMessage';

/** Browsers and earlier desktop hosts have only in-app shortcuts. */
export function desktopMuteShortcut() {
  return typeof window !== 'undefined' ? window.chattoDesktop?.muteShortcut : undefined;
}

/** Dispose the binding on call exit or edit. Ignore deliveries from a previous call. */
export function bindDesktopMuteShortcut(chord: string, toggle: () => void) {
  const bridge = desktopMuteShortcut();
  if (!bridge || !chord) return;
  const token = crypto.randomUUID();
  let active = true;
  const unsubscribe = bridge.onPress((pressedToken) => {
    if (active && pressedToken === token) toggle();
  });
  function reportUnavailable() {
    if (active) toastError(undefined, m('common.keyboard_shortcuts.unavailable'));
  }
  void bridge.set(chord, token).then((ok) => {
    if (!ok) reportUnavailable();
  }, reportUnavailable);
  return () => {
    active = false;
    unsubscribe();
    void bridge.set('', token).catch(reportUnavailable);
  };
}
