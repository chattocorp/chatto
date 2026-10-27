/** Limits shared by Markdown placeholders and the isolated diagram renderer. */
export const MAX_MERMAID_SOURCE = 10_000;
export const MAX_MERMAID_PREVIEWS = 4;

/** Reject author configuration before Pele can merge it with application options. */
export function isMermaidSourceAllowed(source: string): boolean {
  return (
    source.length <= MAX_MERMAID_SOURCE && !/^\s*---/u.test(source) && !/%%\s*\{/u.test(source)
  );
}
