import { describe, expect, it } from 'vitest';
import { canonicalServerOrigin, handoffHost, serverHost } from './serverUrl';

describe('canonicalServerOrigin', () => {
  it('normalizes case, default ports, and paths of HTTP(S) URLs', () => {
    expect(canonicalServerOrigin('HTTPS://Example.COM:443/path/')).toBe('https://example.com');
    expect(canonicalServerOrigin('http://example.com:8080')).toBe('http://example.com:8080');
  });

  it('rejects other schemes, credentials, and invalid URLs', () => {
    expect(canonicalServerOrigin('ftp://example.com')).toBeNull();
    expect(canonicalServerOrigin('https://user@example.com')).toBeNull();
    expect(canonicalServerOrigin('not a url')).toBeNull();
  });
});

describe('serverHost', () => {
  it('shows the host with a non-default port', () => {
    expect(serverHost('https://chat.example.test:8443/path')).toBe('chat.example.test:8443');
    expect(serverHost('https://chat.example.test:443')).toBe('chat.example.test');
  });

  it('returns a value that is not a URL unchanged', () => {
    expect(serverHost('not a url')).toBe('not a url');
  });
});

describe('handoffHost', () => {
  it('accepts a single DNS hostname, including an ASCII IDN', () => {
    expect(handoffHost('Book-Club.Example.COM', false)?.hostname).toBe('book-club.example.com');
    expect(handoffHost('xn--bcher-kva.example', false)?.hostname).toBe('xn--bcher-kva.example');
  });

  it.each([
    '',
    'https://host.example',
    'host.example:443',
    'host.example/',
    'host.example?x',
    'host.example#section',
    'person@host.example',
    'host.example\\other',
    ' host.example',
    '.example',
    'a..example',
    '127.0.0.1',
    '0x7f.0x1',
    '[::1]',
    'localhost',
    'host-.example',
    'K.example',
    'a'.repeat(64) + '.example'
  ])('rejects %s', (server) => {
    expect(handoffHost(server, false)).toBeNull();
  });

  it('accepts explicit ports only during development', () => {
    expect(handoffHost('host.example:8443', false)).toBeNull();
    expect(handoffHost('host.example:8443', true)?.origin).toBe('https://host.example:8443');
    expect(handoffHost('localhost:8080', true)?.origin).toBe('http://localhost:8080');
    expect(handoffHost('localhost:8080', false)).toBeNull();
    for (const port of ['', '0', '65536', '443:80', '443/path']) {
      expect(handoffHost('host.example:' + port, true)).toBeNull();
    }
  });
});
