/** Apply the shared scroll fades to code viewports inside generated Markdown. */
import { attachCodeFades } from './ui/codeScrollFades';

/** Each HTML version owns its content wrappers, overlays and resize observers. */
export function attachCodeScrollFades(html: string) {
  return (element: HTMLElement) => {
    if (!html.includes('markdown-code-frame')) return;
    const cleanups: (() => void)[] = [];
    element.querySelectorAll<HTMLElement>('pre.markdown-code > code').forEach((viewport) => {
      const frame = viewport.parentElement!;
      const content = document.createElement('span');
      content.className = 'markdown-code-scroll-content';
      while (viewport.firstChild) content.appendChild(viewport.firstChild);
      viewport.append(content);

      cleanups.push(attachCodeFades(content, frame));
      cleanups.push(() => {
        while (content.firstChild) viewport.insertBefore(content.firstChild, content);
        content.remove();
      });
    });
    return () => cleanups.forEach((cleanup) => cleanup());
  };
}
