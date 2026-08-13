/** Whether the frontend origin can also host a Chatto HTTP backend. */
export function isBackendCapableOrigin(url: Pick<URL, 'protocol'>): boolean {
  return url.protocol === 'http:' || url.protocol === 'https:';
}
