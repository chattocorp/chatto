import { createRawSnippet } from 'svelte';

/**
 * Build a `Snippet` from a static HTML string for use as the `children`
 * (or any snippet) prop in component tests. The HTML is inserted as-is,
 * so escape user-controlled values yourself. Components rendered through
 * `vitest-browser-svelte`'s `render()` accept the result directly:
 *
 * ```ts
 * render(Dialog, {
 *   props: { visible: true, children: testSnippet('<span>Body</span>') }
 * });
 * ```
 */
export function testSnippet(html: string) {
  return createRawSnippet(() => ({
    render: () => html
  }));
}
