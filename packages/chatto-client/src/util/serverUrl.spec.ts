import { describe, expect, it } from 'vitest';
import { canonicalServerOrigin, serverHost } from './serverUrl.js';

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
