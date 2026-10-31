/** DOM-owned Mermaid previews: lazy worker loading, image isolation and theme updates. */
import { diagramDepth } from './depth';

const themeTokens: Record<string, string> = {
  bg: 'background',
  fg: 'text',
  muted: 'muted',
  surface: 'surface',
  'surface-alt': 'surface-emphasized',
  border: 'input-border',
  'frame-border': 'depth-border',
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

// Image documents cannot inherit app CSS. Keep this small stylesheet scoped to
// Pele's defaults so explicit colours and shapes in the diagram remain intact.
const diagramStyle = `
:root {
  --pele-radius: 6px;
  --pele-title-weight: 600;
  --pele-heading-weight: 600;
  --pele-bold-weight: 600;
  --pele-line: color-mix(in srgb, var(--pele-muted) 70%, var(--pele-accent));
  --pele-series-7: color-mix(in srgb, var(--pele-series-1), var(--pele-series-4));
  --pele-series-8: color-mix(in srgb, var(--pele-series-2), var(--pele-series-3));
}
.pele-edges {
  stroke-width: 1.6;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.pele-marker[fill="var(--_l)"] {
  fill: var(--pele-accent);
  stroke: var(--pele-accent);
  stroke-width: 0.6;
  stroke-linejoin: round;
}
.pele-marker[stroke="var(--_l)"] { stroke: var(--pele-accent); }
.pele-node > :is(rect, polygon, path, circle, ellipse)[stroke="var(--_b)"] {
  stroke-width: 1.25;
}
.pele-cluster-label { font-weight: 600; }
.chatto-pele-frame {
  fill: var(--pele-surface);
  fill-opacity: 1;
  stroke: color-mix(in srgb, var(--pele-fg) 10%, transparent);
  rx: 8px;
}
.chatto-pele-inset { stroke: var(--pele-frame-border); stroke-width: 1; }
`;

/** Resolve Chatto's CSS variables before the SVG enters an isolated image document. */
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
  return `<style>:root{${colours}}${diagramStyle}</style>${diagramDepth(computed)}`;
}

function imageDescription(source: string): string {
  const title = source.match(/^\s*accTitle\s*:\s*(.+)$/mu)?.[1];
  const description =
    source.match(/^\s*accDescr\s*:\s*(.+)$/mu)?.[1] ??
    source.match(/^\s*accDescr\s*\{([^}]+)\}/mu)?.[1]?.trim();
  return [title, description].filter(Boolean).join('. ') || source;
}

/** Render into an isolated image; the caller owns tab state and failure handling. */
export async function createMermaidPreview(
  source: string,
  signal: AbortSignal
): Promise<HTMLImageElement | null> {
  const { requestMermaid } = await import('./queue');
  if (signal.aborted) return null;
  const svg = await requestMermaid(source, signal);
  if (!svg || signal.aborted) return null;
  const image = new Image();
  image.className = 'markdown-mermaid-preview';
  image.alt = imageDescription(source);
  image.decoding = 'async';
  let palette = '';
  const updateTheme = () => {
    const updated = themeStyle();
    if (updated === palette) return;
    palette = updated;
    const themed = svg.replace(/(<svg\b[^>]*>)/u, `$1${palette}`);
    image.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(themed)}`;
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
