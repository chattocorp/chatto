import { beforeEach, describe, expect, it, vi } from 'vitest';
import { normalizeSidebarLinkURL, sidebarLinkTarget } from './sidebarLinkTarget';

const { registeredURLs } = vi.hoisted(() => ({ registeredURLs: [] as string[] }));

vi.mock('$lib/client', async () => ({
  ...(await import('$lib/test-utils/clientMock')).clientMockDefaults,
  serverRegistry: {
    get servers() {
      return registeredURLs.map((url) => ({ url }));
    }
  }
}));

beforeEach(() => {
  registeredURLs.splice(
    0,
    registeredURLs.length,
    'https://remote.example.test',
    'http://localhost:4000'
  );
});

describe('sidebarLinkTarget', () => {
  it.each(['https://remote.example.test/docs', 'http://remote.example.test:8080/docs'])(
    'opens a registered hostname in the current tab even when another server is active: %s',
    (url) => {
      expect(sidebarLinkTarget(url, 'https://active.example.test')).toEqual({
        valid: true,
        href: url
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
    expect(sidebarLinkTarget('https://remote.example.test/docs', null)).toEqual({
      valid: true,
      href: 'https://remote.example.test/docs'
    });
    registeredURLs.splice(0);
    expect(sidebarLinkTarget('https://remote.example.test/docs', null)).toMatchObject({
      target: '_blank'
    });
  });
  it('resolves server-local paths against the active server base URL', () => {
    expect(sidebarLinkTarget('/docs', 'https://remote.example.test')).toEqual({
      valid: true,
      href: 'https://remote.example.test/docs'
    });
  });

  it('opens absolute URLs on the active server host in the same tab', () => {
    expect(
      sidebarLinkTarget('https://remote.example.test/docs', 'https://remote.example.test')
    ).toEqual({
      valid: true,
      href: 'https://remote.example.test/docs'
    });
  });

  it('matches hostnames independently of ports', () => {
    expect(sidebarLinkTarget('http://localhost:4000/docs', 'http://localhost:4000')).toEqual({
      valid: true,
      href: 'http://localhost:4000/docs'
    });
    expect(sidebarLinkTarget('http://localhost:4001/docs', 'http://localhost:4000')).toEqual({
      valid: true,
      href: 'http://localhost:4001/docs'
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
