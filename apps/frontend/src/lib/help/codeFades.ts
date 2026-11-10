import { attachCodeFades } from '$lib/ui/codeScrollFades';

/** Add shared edge cues to an authored code block; restore its DOM on unmount. */
export function attachHelpCodeFades(frame: HTMLElement): () => void {
  const viewport = frame.querySelector('code');
  if (!viewport) return () => {};
  const previousTabIndex = viewport.getAttribute('tabindex');
  viewport.tabIndex = 0;
  const content = document.createElement('span');
  content.className = 'inline-block min-w-full';
  while (viewport.firstChild) content.append(viewport.firstChild);
  viewport.append(content);
  const cleanup = attachCodeFades(content, frame);
  return () => {
    cleanup();
    while (content.firstChild) viewport.insertBefore(content.firstChild, content);
    content.remove();
    if (previousTabIndex === null) viewport.removeAttribute('tabindex');
    else viewport.setAttribute('tabindex', previousTabIndex);
  };
}
