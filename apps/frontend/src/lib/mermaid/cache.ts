/** Successful layouts follow a loaded message body's identity, not its virtualized DOM row. */
import { MAX_MERMAID_PREVIEWS } from './policy';

const layouts = new WeakMap<object, Map<string, string>>();

/** Read an unthemed SVG without keeping its message owner alive. */
export function getMermaidLayout(owner: object, source: string): string | undefined {
  return layouts.get(owner)?.get(source);
}

/** Bound each owner's retained output to the number of diagrams allowed in a message. */
export function rememberMermaidLayout(owner: object, source: string, svg: string): void {
  let entries = layouts.get(owner);
  if (!entries) layouts.set(owner, (entries = new Map()));
  entries.delete(source);
  entries.set(source, svg);
  if (entries.size > MAX_MERMAID_PREVIEWS) entries.delete(entries.keys().next().value!);
}
