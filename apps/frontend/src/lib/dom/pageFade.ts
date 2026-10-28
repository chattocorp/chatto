import { motionPreference } from './motionPreference';

/**
 * Fade in the new route's main content without moving or retaining its DOM.
 * Nested targets keep room sidebars outside the fade. The caller stops the
 * previous fade before navigation and releases it when the root unmounts.
 */
export function fadePageContent(): () => void {
  const motion = motionPreference();
  const element = document.querySelector<HTMLElement>(
    '[data-navigation-content]:not(:has([data-navigation-content]))'
  );
  if (motion.matches || !element) return () => {};
  const animation = element.animate([{ opacity: 0.55 }, { opacity: 1 }], {
    duration: 140,
    easing: 'ease-out'
  });
  const stop = () => {
    animation.cancel();
    motion.removeEventListener('change', stop);
  };
  motion.addEventListener('change', stop);
  void animation.finished.then(stop, stop);
  return stop;
}
