/**
 * `querySelector` returns `Element`, but `expect.element()` from
 * `@vitest/browser` and many DOM helpers need `HTMLElement`. This is a
 * tiny cast helper used across the component specs to avoid sprinkling
 * `as HTMLElement` at every call site.
 */
export function q(container: Element, selector: string): HTMLElement | null {
  return container.querySelector(selector) as HTMLElement | null;
}
