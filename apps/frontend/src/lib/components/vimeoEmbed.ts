/** Builds a Vimeo player URL for supported video links, including stored generic previews. */
export function vimeoEmbedUrl(rawUrl: string): string | null {
  let url: URL;
  try {
    url = new URL(rawUrl);
  } catch {
    return null;
  }
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.port) {
    return null;
  }

  let match: RegExpMatchArray | null;
  if (url.hostname === 'player.vimeo.com') {
    match = url.pathname.match(/^\/video\/(\d+)\/?$/);
  } else if (url.hostname === 'vimeo.com' || url.hostname === 'www.vimeo.com') {
    match =
      url.pathname.match(/^\/(\d+)(?:\/([a-zA-Z0-9]+))?\/?$/) ??
      url.pathname.match(/^\/channels\/[^/]+\/(\d+)\/?$/) ??
      url.pathname.match(/^\/groups\/[^/]+\/videos\/(\d+)\/?$/) ??
      url.pathname.match(/^\/showcase\/\d+\/video\/(\d+)\/?$/);
  } else {
    return null;
  }
  if (!match) return null;

  // Unlisted videos need the privacy hash from the share link or player query.
  const hash = match[2] ?? url.searchParams.get('h');
  if (hash !== null && hash !== undefined && !/^[a-zA-Z0-9]+$/.test(hash)) return null;

  const player = new URL(`https://player.vimeo.com/video/${match[1]}`);
  if (hash) player.searchParams.set('h', hash);
  player.searchParams.set('dnt', '1');
  return player.href;
}
