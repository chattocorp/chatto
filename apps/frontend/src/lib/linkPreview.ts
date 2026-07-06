/**
 * URL detection utilities for link previews.
 *
 * Uses linkify-it (same library as markdown-it's auto-linker) with the full
 * IANA TLD list so that bare-domain URLs like www.hmans.dev are detected.
 */

import LinkifyIt from 'linkify-it';
import MarkdownIt from 'markdown-it';
import tlds from 'tlds';

/** Shared linkify-it instance configured with the full IANA TLD list. */
export const linkify = new LinkifyIt();
linkify.tlds(tlds);

const markdown = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true
});
markdown.linkify.tlds(tlds);
markdown.disable(['escape']);

/**
 * Extracts unique URLs from text, including bare-domain URLs (e.g. www.hmans.dev).
 * Returns at most maxURLs URLs, in the order they appear.
 * Bare-domain URLs are normalized to https://.
 */
export function extractURLs(text: string, maxURLs = 1): string[] {
  if (maxURLs <= 0 || !text) return [];

  const seen = new Set<string>();
  const result: string[] = [];

  const addURL = (rawUrl: string, rawText = '') => {
    if (result.length >= maxURLs) return;

    let url = rawUrl;
    if (
      rawText &&
      !/^[a-z][a-z0-9+.-]*:/i.test(rawText) &&
      /^http:\/\//i.test(url)
    ) {
      // linkify-it adds http:// to bare domains; upgrade those to https://.
      url = url.replace(/^http:\/\//i, 'https://');
    }

    if (!/^https?:\/\//i.test(url)) return;

    const normalized = normalizeURL(url);

    if (!seen.has(normalized)) {
      seen.add(normalized);
      result.push(url);
    }
  };

  let blockquoteDepth = 0;
  for (const token of markdown.parse(text, {})) {
    if (token.type === 'blockquote_open') {
      blockquoteDepth++;
      continue;
    }
    if (token.type === 'blockquote_close') {
      blockquoteDepth = Math.max(0, blockquoteDepth - 1);
      continue;
    }
    if (blockquoteDepth > 0) continue;
    if (token.type === 'fence' || token.type === 'code_block') continue;
    if (token.type !== 'inline') continue;

    const children = token.children ?? [];
    for (let i = 0; i < children.length; i++) {
      const child = children[i];
      if (child.type !== 'link_open') continue;

      const href = child.attrGet('href');
      if (!href) continue;

      const rawText = child.markup === 'linkify' ? (children[i + 1]?.content ?? '') : '';
      addURL(href, rawText);
    }
  }

  return result;
}

/**
 * Normalizes a URL for deduplication.
 */
function normalizeURL(url: string): string {
  try {
    const parsed = new URL(url);
    // Lowercase scheme and host, remove fragment
    return `${parsed.protocol.toLowerCase()}//${parsed.host.toLowerCase()}${parsed.pathname}${parsed.search}`;
  } catch {
    return url.toLowerCase();
  }
}

// Valid YouTube hostnames
const YOUTUBE_HOSTS = new Set(['youtube.com', 'www.youtube.com', 'm.youtube.com', 'youtu.be']);

// YouTube path/query patterns (applied after hostname validation)
const YOUTUBE_PATH_REGEX = /^\/(?:watch\?(?:.*&)?v=|embed\/|v\/|shorts\/)([a-zA-Z0-9_-]{11})/;

/**
 * Checks if a URL is a YouTube video URL.
 */
export function isYouTubeURL(url: string): boolean {
  return parseYouTubeVideoID(url) !== null;
}

/**
 * Extracts the video ID from a YouTube URL.
 * Returns null if the URL is not a valid YouTube video URL.
 */
export function parseYouTubeVideoID(rawUrl: string): string | null {
  let parsed: URL;
  try {
    parsed = new URL(rawUrl);
  } catch {
    return null;
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    return null;
  }

  const host = parsed.hostname.toLowerCase();
  if (!YOUTUBE_HOSTS.has(host)) {
    return null;
  }

  // For youtu.be short URLs, the video ID is the path
  if (host === 'youtu.be') {
    const id = parsed.pathname.slice(1); // Remove leading /
    return id.length === 11 ? id : null;
  }

  // For youtube.com, match path/query patterns
  const pathAndQuery = parsed.pathname + parsed.search;
  const match = pathAndQuery.match(YOUTUBE_PATH_REGEX);
  return match ? match[1] : null;
}
