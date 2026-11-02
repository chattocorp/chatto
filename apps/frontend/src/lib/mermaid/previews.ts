/** DOM-owned Mermaid previews: lazy worker loading, image isolation and theme updates. */
import { getMermaidLayout, rememberMermaidLayout } from './cache';

const themeTokens: Record<string, string> = {
  bg: 'background',
  fg: 'text',
  muted: 'muted',
  line: 'muted',
  surface: 'surface',
  'surface-alt': 'surface-emphasized',
  border: 'input-border',
  accent: 'action',
  critical: 'danger',
  error: 'danger',
  'series-1': 'action',
  'series-2': 'success',
  'series-3': 'warning',
  'series-4': 'server',
  'series-5': 'danger',
  'series-6': 'neutral-action'
};

/** Copy theme colours into the image; leave diagram shapes and effects to Pele. */
function themeStyle(): string {
  const computed = getComputedStyle(document.documentElement);
  const colours = Object.entries(themeTokens)
    .map(([name, token]) => {
      const color = computed.getPropertyValue(`--color-${token}`).trim();
      return color && CSS.supports('color', color) && !/[;{}<>]/u.test(color)
        ? `--pele-${name}:${color};`
        : '';
    })
    .join('');
  return `<style>:root{${colours}}</style>`;
}

function imageDescription(source: string): string {
  const title = source.match(/^\s*accTitle\s*:\s*(.+)$/mu)?.[1];
  const description =
    source.match(/^\s*accDescr\s*:\s*(.+)$/mu)?.[1] ??
    source.match(/^\s*accDescr\s*\{([^}]+)\}/mu)?.[1]?.trim();
  return [title, description].filter(Boolean).join('. ') || source;
}

/**
 * Render into an isolated image; the caller owns tab state and failure handling.
 * An optional loaded message owner retains successful layouts across row remounts.
 * Each caller owns its image and theme listeners, disposed with its signal.
 * A detached loaded image can be reused without restarting image decoding.
 */
export async function createMermaidPreview(
  source: string,
  signal: AbortSignal,
  owner?: object,
  retainedImage?: HTMLImageElement
): Promise<HTMLImageElement | null> {
  if (signal.aborted) return null;
  let svg = owner && getMermaidLayout(owner, source);
  if (!svg) {
    const { requestMermaid } = await import('./queue');
    if (signal.aborted) return null;
    svg = (await requestMermaid(source, signal)) ?? undefined;
    if (svg && owner && !signal.aborted) rememberMermaidLayout(owner, source, svg);
  }
  if (!svg || signal.aborted) return null;
  const image = retainedImage ?? new Image();
  image.className = 'markdown-mermaid-preview';
  image.alt = imageDescription(source);
  image.decoding = 'async';
  let palette = '';
  const updateTheme = () => {
    const updated = themeStyle();
    if (updated === palette) return;
    palette = updated;
    const themed = svg.replace(/(<svg\b[^>]*>)/u, `$1${palette}`);
    const url = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(themed)}`;
    if (image.src !== url) image.src = url;
  };
  const observer = new MutationObserver(updateTheme);
  observer.observe(document.documentElement, {
    attributeFilter: ['data-theme', 'data-accent', 'data-light-tone', 'data-dark-tone', 'style']
  });
  const onThemeTransitionEnd = (event: TransitionEvent) => {
    // Descendant transitions bubble here, but cannot change the root palette.
    if (event.target !== document.documentElement) return;
    updateTheme();
  };
  document.documentElement.addEventListener('transitionend', onThemeTransitionEnd);
  signal.addEventListener(
    'abort',
    () => {
      observer.disconnect();
      document.documentElement.removeEventListener('transitionend', onThemeTransitionEnd);
    },
    { once: true }
  );
  updateTheme();
  return image;
}
