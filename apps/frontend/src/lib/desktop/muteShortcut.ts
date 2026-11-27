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

/** Release an active hold when its call, shortcut, or native listener goes away. */
export function bindDesktopPushToTalk(chord: string, setHeld: (held: boolean) => void) {
  const bridge = window.chattoDesktop?.pushToTalk;
  if (!bridge || !chord) return;
  const token = crypto.randomUUID();
  let active = true;
  function failed() {
    if (!active) return;
    setHeld(false);
    toastError(undefined, m('common.keyboard_shortcuts.unavailable'));
  }
  const unsubscribe = bridge.onEdge((owner, edge) => {
    if (!active || owner !== token) return;
    if (edge === 'failed') failed();
    else setHeld(edge === 'down');
  });
  void bridge.set(chord, token).then((ok) => {
    if (!ok) failed();
  }, failed);
  return () => {
    active = false;
    unsubscribe();
    setHeld(false);
    void bridge.set('', token).catch(() => {});
  };
}
