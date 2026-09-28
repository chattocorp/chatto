import type { Attachment } from 'svelte/attachments';

/** Split the initial app into sections, without animating both a parent and its children. */
function revealSections(element: Element): Element[] {
  if (element.querySelector('[data-page-reveal]')) {
    return Array.from(element.children).flatMap(revealSections);
  }
  return element.getBoundingClientRect().height > 0 ? [element] : [];
}

/**
 * Reveal the initial app once per root-layout mount. Later route changes and
 * asynchronously loaded content stay immediately visible. Mark sections with
 * data-page-reveal to control the stagger; unmarked siblings remain whole.
 */
export const initialPageReveal: Attachment = (element) => {
  // The HTML shell is visible while SvelteKit resolves the initial route.
  const loadingShell = document.getElementById('app-loading');
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  if (reducedMotion.matches) {
    loadingShell?.remove();
    return;
  }

  const loadingAnimation = loadingShell?.animate(
    [
      { opacity: 1, transform: 'scale(1)' },
      { opacity: 0, transform: 'scale(1.02)' }
    ],
    { duration: 540, easing: 'ease-out', fill: 'forwards' }
  );
  const removeLoadingShell = () => {
    if (!loadingShell) return;
    loadingAnimation?.cancel();
    loadingShell.remove();
  };
  if (loadingAnimation) loadingAnimation.onfinish = removeLoadingShell;

  const sections = Array.from(element.children).flatMap(revealSections);
  const stagger = Math.min(90, 270 / Math.max(1, sections.length - 1));
  const animations = sections.map((part, index) =>
    part.animate(
      [
        { opacity: 0, scale: '0.98' },
        { opacity: 1, scale: '1' }
      ],
      { duration: 540, delay: index * stagger, easing: 'ease-out', fill: 'backwards' }
    )
  );
  const showAll = () => {
    if (loadingAnimation) loadingAnimation.onfinish = null;
    removeLoadingShell();
    for (const animation of animations) {
      animation.onfinish = null;
      animation.cancel();
    }
    // Release initial route elements when the reveal ends, before later navigation.
    animations.length = 0;
    window.removeEventListener('keydown', showAll, true);
    element.removeEventListener('pointerdown', showAll, true);
    reducedMotion.removeEventListener('change', stopForReducedMotion);
  };
  const stopForReducedMotion = () => {
    if (reducedMotion.matches) showAll();
  };
  // Automatic form focus must not cancel the reveal. Real input makes every
  // control visible before the browser handles the key or pointer action.
  window.addEventListener('keydown', showAll, true);
  element.addEventListener('pointerdown', showAll, true);
  reducedMotion.addEventListener('change', stopForReducedMotion);
  let remaining = animations.length;
  for (const animation of animations) {
    animation.onfinish = () => {
      if (--remaining === 0) showAll();
    };
  }
  return showAll;
};
