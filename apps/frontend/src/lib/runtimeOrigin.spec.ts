import { describe, expect, it } from 'vitest';
import { isBackendCapableOrigin } from './runtimeOrigin';

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
