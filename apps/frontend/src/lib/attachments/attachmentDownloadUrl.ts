/** Add the server download mode without losing the asset access ticket. */
export function attachmentDownloadUrl(url: string | null | undefined): string | null {
  if (!url) return null;
  const parsed = new URL(url, 'https://attachment.invalid');
  parsed.searchParams.set('download', '1');
  return url.startsWith('/') ? `${parsed.pathname}${parsed.search}${parsed.hash}` : parsed.href;
}
