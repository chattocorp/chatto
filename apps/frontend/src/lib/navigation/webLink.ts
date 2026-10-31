/** Parse automatic web links for compact display without contacting their destination. */
export function parseWebLink(href: string): URL | null {
  try {
    const url = new URL(href);
    return url.protocol === 'http:' || url.protocol === 'https:' ? url : null;
  } catch {
    return null;
  }
}
