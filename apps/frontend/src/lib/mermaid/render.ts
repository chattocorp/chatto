/** Pure, bounded Pele rendering. Production callers run this only in a worker. */
import { parse, render } from 'pele';
import { isMermaidSourceAllowed, MAX_MERMAID_SOURCE } from './policy';
import { frameMermaidGroups } from './groups';

/** Return self-contained SVG, or throw. Never display or log parser errors with message text. */
export function renderMermaid(source: string): string {
  if (!isMermaidSourceAllowed(source)) throw new Error('Unsupported diagram');
  const model = parse(source, { limit: MAX_MERMAID_SOURCE, maxEdges: 250 });
  // Bound all diagram models, including types without Pele's flowchart edge limit.
  // Counting model objects also covers nested groups, participants and statements.
  const pending: unknown[] = [model];
  const seen = new Set<object>();
  while (pending.length) {
    const value = pending.pop();
    if (!value || typeof value !== 'object' || seen.has(value)) continue;
    seen.add(value);
    if (seen.size > 1_000) throw new Error('Diagram too complex');
    pending.push(...Object.values(value));
  }
  const result = render(model, {
    links: false,
    images: false,
    limit: MAX_MERMAID_SOURCE,
    outputLimit: 500_000,
    maxEdges: 250,
    responsive: false,
    autoDirection: false,
    padding: 16,
    fontFamily: 'sans-serif'
  });
  if (
    !Number.isFinite(result.width) ||
    !Number.isFinite(result.height) ||
    result.width <= 0 ||
    result.height <= 0 ||
    result.width > 8_192 ||
    result.height > 8_192 ||
    result.width * result.height > 16_000_000
  )
    throw new Error('Diagram too large');
  const svg = frameMermaidGroups(result.svg);
  if (svg.length > 500_000) throw new Error('Diagram too large');
  return svg;
}
