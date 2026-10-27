/** Apply the shared scroll fades to code viewports inside generated Markdown. */
import { trackScrollEdges } from './ui/scrollEdges';

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

      const fades = ['top', 'bottom', 'left', 'right'].map((edge) => {
        const fade = document.createElement('span');
        fade.className = `markdown-code-fade markdown-code-fade-${edge} opacity-0`;
        fade.setAttribute('aria-hidden', 'true');
        frame.append(fade);
        return fade;
      });
      for (const [axis, offset] of [
        ['y', 0],
        ['x', 2]
      ] as const) {
        const cleanup = trackScrollEdges(axis, ({ start, end }) => {
          fades[offset].classList.toggle('opacity-0', !start);
          fades[offset + 1].classList.toggle('opacity-0', !end);
        })(content);
        if (cleanup) cleanups.push(cleanup);
      }
      cleanups.push(() => {
        fades.forEach((fade) => fade.remove());
        while (content.firstChild) viewport.insertBefore(content.firstChild, content);
        content.remove();
      });
    });
    return () => cleanups.forEach((cleanup) => cleanup());
  };
}
