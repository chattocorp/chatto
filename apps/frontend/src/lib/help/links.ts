/**
 * Keep an explicitly selected server on local Help links. Callers own route
 * resolution; this helper changes only the query and preserves section anchors.
 */
export function withHelpServer(href: string, serverSegment?: string | null): string {
  if (serverSegment == null) return href;
  const hashAt = href.indexOf('#');
  const pathAndQuery = hashAt < 0 ? href : href.slice(0, hashAt);
  const hash = hashAt < 0 ? '' : href.slice(hashAt);
  const queryAt = pathAndQuery.indexOf('?');
  const path = queryAt < 0 ? pathAndQuery : pathAndQuery.slice(0, queryAt);
  const query = new URLSearchParams(queryAt < 0 ? '' : pathAndQuery.slice(queryAt + 1));
  query.set('server', serverSegment);
  return `${path}?${query}${hash}`;
}
