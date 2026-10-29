import type { Attachment } from 'svelte/attachments';
import { motionPreference } from './motionPreference';

/**
 * Animate a panel selection without remounting its content. Each owner creates
 * one attachment. Initial render and scope changes stay immediate; changes to
 * the selected panel reveal its new DOM without retaining the previous view.
 */
export function createPanelReveal(
  selection: () => { scope: string; panel: string }
): Attachment<HTMLElement> {
  let previous: { scope: string; panel: string } | undefined;
  return (element) => {
    const current = selection();
    const changed = previous?.scope === current.scope && previous.panel !== current.panel;
    previous = current;
    if (!changed) return;

    const reducedMotion = motionPreference();
    if (reducedMotion.matches) return;
    const offset = getComputedStyle(element).direction === 'rtl' ? -8 : 8;
    const animation = element.animate(
      [
        { opacity: 0, transform: `translateX(${offset}px)` },
        { opacity: 1, transform: 'none' }
      ],
      { duration: 140, easing: 'ease-out' }
    );
    const stop = () => {
      if (reducedMotion.matches) animation.cancel();
    };
    reducedMotion.addEventListener('change', stop);
    return () => {
      animation.cancel();
      reducedMotion.removeEventListener('change', stop);
    };
  };
}
