import { describe, expect, it } from 'vitest';
import { isBackendCapableOrigin, isLoopbackHostname } from './runtimeOrigin.js';

describe('isBackendCapableOrigin', () => {
  it.each(['http://chat.example', 'https://chat.example'])(
    'accepts Chatto web origins: %s',
    (origin) => {
      expect(isBackendCapableOrigin(new URL(origin))).toBe(true);
    }
  );

  it.each(['chatto://desktop', 'file:///Applications/Chatto/index.html'])(
    'rejects application-only origins: %s',
    (origin) => {
      expect(isBackendCapableOrigin(new URL(origin))).toBe(false);
    }
  );
});

describe('isLoopbackHostname', () => {
  it.each(['localhost', 'LOCALHOST', '127.0.0.1', '[::1]', '::1', 'chatto.canberra.localhost'])(
    'accepts loopback hostnames: %s',
    (hostname) => {
      expect(isLoopbackHostname(hostname)).toBe(true);
    }
  );

  it.each([
    'chat.example',
    'localhost.example',
    '.localhost',
    'bad..localhost',
    '-bad.localhost',
    'under_score.localhost',
    '127.0.0.2'
  ])('rejects other hostnames: %s', (hostname) => {
    expect(isLoopbackHostname(hostname)).toBe(false);
  });
});
