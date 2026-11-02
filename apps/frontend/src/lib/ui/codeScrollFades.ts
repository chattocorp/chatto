import { trackScrollEdges } from './scrollEdges';

/** Add shared code fades outside the editable content. The caller owns cleanup. */
export function attachCodeFades(content: HTMLElement, frame: HTMLElement): () => void {
  const fades = ['top', 'bottom', 'left', 'right'].map((edge) => {
    const fade = document.createElement('span');
    fade.className = `markdown-code-fade markdown-code-fade-${edge} opacity-0`;
    fade.setAttribute('aria-hidden', 'true');
    frame.append(fade);
    return fade;
  });
  // Fade opacity does not affect geometry. Let resize delivery initialize it
  // after all code wrappers are in place, without forcing layout per block.
  const cleanups = (['y', 'x'] as const).map((axis, index) =>
    trackScrollEdges(
      axis,
      ({ start, end }) => {
        fades[index * 2].classList.toggle('opacity-0', !start);
        fades[index * 2 + 1].classList.toggle('opacity-0', !end);
      },
      { initialMeasurement: 'resize' }
    )(content)
  );
  return () => {
    cleanups.forEach((cleanup) => cleanup?.());
    fades.forEach((fade) => fade.remove());
  };
}
