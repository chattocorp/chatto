/**
 * Handles virtual keyboard and scroll-to-focus fixes for iOS Safari.
 *
 * The body uses `position: fixed; inset: 0` which fills the layout viewport
 * correctly in all modes — including standalone PWA (not affected by WebKit
 * bug 237961, unlike dvh/vh/100% which are all short by safe-area-inset-top).
 *
 * However, iOS Safari does NOT resize the layout viewport when the virtual
 * keyboard opens (and does not support `interactive-widget=resizes-content`).
 * This means `inset: 0` keeps the body at full height and the keyboard covers
 * the bottom content (like the chat composer).
 *
 * This hook detects the keyboard by comparing `visualViewport.height` to a
 * stored reference height (captured when no keyboard is visible). When the
 * keyboard is open, it sets an explicit body height to shrink above the
 * keyboard. When closed, it clears the override and lets CSS handle sizing.
 *
 * Also counteracts iOS Safari's scroll-to-focus behavior that shifts the
 * document even when the body is `position: fixed`.
 *
 * Call once from the root layout.
 */
export function useVisualViewport() {
  $effect(() => {
    const vv = window.visualViewport;
    if (!vv) return;

    // Reference height = visual viewport height when no keyboard is visible.
    let fullHeight = vv.height;
    let lastWidth = vv.width;

    function update() {
      // Orientation change: width changed, so reset reference and let CSS handle it.
      if (vv!.width !== lastWidth) {
        fullHeight = vv!.height;
        lastWidth = vv!.width;
        document.body.style.height = '';
        return;
      }

      // Keyboard detection: if visual viewport is significantly shorter than
      // the reference height, the keyboard is open.
      const keyboardLikelyOpen = vv!.height < fullHeight * 0.75;

      if (keyboardLikelyOpen) {
        // Override body height to shrink above the keyboard.
        document.body.style.height = `${vv!.height}px`;
      } else {
        // No keyboard — update reference, clear override, let CSS inset:0 handle sizing.
        fullHeight = vv!.height;
        document.body.style.height = '';
      }

      // Prevent iOS Safari from scrolling the document when focusing inputs.
      if (vv!.offsetTop > 0) {
        window.scrollTo(0, 0);
      }
    }

    vv.addEventListener('resize', update);
    vv.addEventListener('scroll', update);
    update();

    return () => {
      vv.removeEventListener('resize', update);
      vv.removeEventListener('scroll', update);
      document.body.style.height = '';
    };
  });
}
