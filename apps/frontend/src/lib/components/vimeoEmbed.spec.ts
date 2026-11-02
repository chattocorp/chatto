import { describe, expect, it } from 'vitest';
import { vimeoEmbedUrl } from './vimeoEmbed';

describe('vimeoEmbedUrl', () => {
  it.each([
    'https://vimeo.com/76979871',
    'https://www.vimeo.com/76979871/',
    'http://vimeo.com/76979871',
    'https://player.vimeo.com/video/76979871',
    'https://vimeo.com/channels/staffpicks/76979871',
    'https://vimeo.com/groups/shortfilms/videos/76979871',
    'https://vimeo.com/showcase/123/video/76979871'
  ])('embeds a supported video link: %s', (url) => {
    expect(vimeoEmbedUrl(url)).toBe('https://player.vimeo.com/video/76979871?dnt=1');
  });

  it.each([
    'https://vimeo.com/76979871/abc123def0',
    'https://vimeo.com/76979871/abc123def0/',
    'https://player.vimeo.com/video/76979871?h=abc123def0',
    'https://vimeo.com/76979871?h=abc123def0'
  ])('preserves the unlisted privacy hash: %s', (url) => {
    expect(vimeoEmbedUrl(url)).toBe('https://player.vimeo.com/video/76979871?h=abc123def0&dnt=1');
  });

  it('discards tracking and automatic playback parameters', () => {
    expect(
      vimeoEmbedUrl(
        'https://player.vimeo.com/video/76979871?autoplay=1&background=1&dnt=0&utm_source=share#t=5s'
      )
    ).toBe('https://player.vimeo.com/video/76979871?dnt=1');
  });

  it.each([
    'not a URL',
    'javascript:alert(1)',
    'ftp://vimeo.com/76979871',
    'https://vimeo.com.evil.example/76979871',
    'https://evil.example/vimeo.com/76979871',
    'https://vimeo.com@evil.example/76979871',
    'https://user:password@vimeo.com/76979871',
    'https://vimeo.com:8443/76979871',
    'https://vimeo.com/',
    'https://vimeo.com/someone',
    'https://vimeo.com/channels/staffpicks',
    'https://player.vimeo.com/76979871',
    'https://vimeo.com/video/76979871',
    'https://vimeo.com/76979871/abc/extra',
    'https://player.vimeo.com/video/76979871?h=bad%26autoplay%3D1'
  ])('keeps unsupported links out of the player: %s', (url) => {
    expect(vimeoEmbedUrl(url)).toBeNull();
  });
});
