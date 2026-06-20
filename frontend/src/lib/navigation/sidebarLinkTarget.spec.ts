import { describe, expect, it } from 'vitest';
import { sidebarLinkTarget } from './sidebarLinkTarget';

describe('sidebarLinkTarget', () => {
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

  it('compares hosts with ports intact', () => {
    expect(sidebarLinkTarget('http://localhost:4000/docs', 'http://localhost:4000')).toEqual({
      valid: true,
      href: 'http://localhost:4000/docs'
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

  it.each([
    'docs',
    '//evil.example',
    '/\\evil.example/path',
    'javascript:alert(1)',
    'mailto:hello@example.test'
  ])(
    'treats %s as invalid',
    (rawURL) => {
      expect(sidebarLinkTarget(rawURL, 'https://remote.example.test')).toEqual({
        valid: false,
        href: '#'
      });
    }
  );
});
