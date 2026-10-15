import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { normalizeSidebarLinkURL, sidebarLinkTarget } from './sidebarLinkTarget';

const { registeredURLs } = vi.hoisted(() => ({ registeredURLs: [] as string[] }));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    isOriginServer: () => false,
    getServer: (id: string) => ({ id, url: id }),
    get servers() {
      return registeredURLs.map((url) => ({ id: url, url }));
    }
  }
}));

beforeEach(() => {
  vi.stubGlobal('window', { location: { origin: 'https://frontend.example.test' } });
  registeredURLs.splice(
    0,
    registeredURLs.length,
    'https://remote.example.test',
    'http://localhost:4000'
  );
});
afterEach(() => vi.unstubAllGlobals());

describe('sidebarLinkTarget', () => {
  it.each(['https://remote.example.test/docs', 'http://remote.example.test:8080/docs'])(
    'opens non-chat paths externally even on a registered hostname: %s',
    (url) => {
      expect(sidebarLinkTarget(url, 'https://active.example.test')).toEqual({
        valid: true,
        href: url,
        target: '_blank',
        rel: 'noopener noreferrer'
      });
    }
  );

  it('requires registration even when the hostname matches the active server URL', () => {
    expect(
      sidebarLinkTarget('https://unknown.example.test/docs', 'https://unknown.example.test')
    ).toMatchObject({ target: '_blank', rel: 'noopener noreferrer' });
  });

  it('uses the current catalogue and ignores invalid server URLs', () => {
    registeredURLs.unshift('not a URL');
    const url = 'https://remote.example.test/chat/-/R123456789abcde';
    expect(sidebarLinkTarget(url, null)).toEqual({
      valid: true,
      href: '/chat/remote.example.test/R123456789abcde'
    });
    registeredURLs.splice(0);
    expect(sidebarLinkTarget(url, null)).toMatchObject({
      target: '_blank'
    });
  });

  it('rewrites server-local conversation paths for the active remote server', () => {
    expect(sidebarLinkTarget('/chat/-/R123456789abcde', 'https://remote.example.test')).toEqual({
      valid: true,
      href: '/chat/remote.example.test/R123456789abcde'
    });
  });
  it('resolves server-local paths against the active server base URL', () => {
    expect(sidebarLinkTarget('/docs', 'https://remote.example.test')).toEqual({
      valid: true,
      href: 'https://remote.example.test/docs',
      target: '_blank',
      rel: 'noopener noreferrer'
    });
  });

  it('opens non-chat URLs on the active server externally', () => {
    expect(
      sidebarLinkTarget('https://remote.example.test/docs', 'https://remote.example.test')
    ).toEqual({
      valid: true,
      href: 'https://remote.example.test/docs',
      target: '_blank',
      rel: 'noopener noreferrer'
    });
  });

  it('keeps non-chat links external on every port', () => {
    expect(sidebarLinkTarget('http://localhost:4000/docs', 'http://localhost:4000')).toEqual({
      valid: true,
      href: 'http://localhost:4000/docs',
      target: '_blank',
      rel: 'noopener noreferrer'
    });
    expect(sidebarLinkTarget('http://localhost:4001/docs', 'http://localhost:4000')).toEqual({
      valid: true,
      href: 'http://localhost:4001/docs',
      target: '_blank',
      rel: 'noopener noreferrer'
    });
  });

  it('opens external absolute URLs in a new tab', () => {
    expect(sidebarLinkTarget('https://docs.example.test', 'https://remote.example.test')).toEqual({
      valid: true,
      href: 'https://docs.example.test/',
      target: '_blank',
      rel: 'noopener noreferrer'
    });
  });

  it('adds HTTPS to hostnames entered without a scheme', () => {
    expect(normalizeSidebarLinkURL('docs.example.test/guide')).toBe(
      'https://docs.example.test/guide'
    );
    expect(normalizeSidebarLinkURL('localhost:4173/status')).toBe('https://localhost:4173/status');
    expect(sidebarLinkTarget('docs.example.test/guide', 'https://remote.example.test')).toEqual({
      valid: true,
      href: 'https://docs.example.test/guide',
      target: '_blank',
      rel: 'noopener noreferrer'
    });
  });

  it('does not rewrite local paths or explicit URI schemes', () => {
    expect(normalizeSidebarLinkURL('/docs')).toBe('/docs');
    expect(normalizeSidebarLinkURL('https://docs.example.test')).toBe('https://docs.example.test');
    expect(normalizeSidebarLinkURL('javascript:alert(1)')).toBe('javascript:alert(1)');
  });

  it.each([
    'not a host',
    '//evil.example',
    '/\\evil.example/path',
    'javascript:alert(1)',
    'mailto:hello@example.test'
  ])('treats %s as invalid', (rawURL) => {
    expect(sidebarLinkTarget(rawURL, 'https://remote.example.test')).toEqual({
      valid: false,
      href: '#'
    });
  });
});
