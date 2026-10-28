/** Keep top-layer modal geometry above the software keyboard, including viewport panning. */
export function modalViewport(node: HTMLDialogElement): (() => void) | undefined {
  const viewport = window.visualViewport;
  if (!viewport) return;
  function update() {
    node.style.setProperty('--modal-viewport-height', `${viewport!.height}px`);
    node.style.setProperty('--modal-viewport-top', `${viewport!.offsetTop}px`);
    node.style.setProperty(
      '--modal-viewport-bottom',
      `${Math.max(0, window.innerHeight - viewport!.height - viewport!.offsetTop)}px`
    );
  }
  update();
  viewport.addEventListener('resize', update);
  viewport.addEventListener('scroll', update);
  return () => {
    viewport.removeEventListener('resize', update);
    viewport.removeEventListener('scroll', update);
  };
}
