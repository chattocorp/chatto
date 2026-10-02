import { describe, expect, it } from 'vitest';
import { describePushDevice } from './pushDevice';

describe('describePushDevice', () => {
  it.each([
    [
      'Chrome on macOS',
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36',
      0,
      { browser: 'Chrome', platform: 'macOS' }
    ],
    [
      'Edge on Windows',
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0',
      0,
      { browser: 'Edge', platform: 'Windows' }
    ],
    [
      'Firefox on Linux',
      'Mozilla/5.0 (X11; Linux x86_64; rv:131.0) Gecko/20100101 Firefox/131.0',
      0,
      { browser: 'Firefox', platform: 'Linux' }
    ],
    [
      'Chrome on Android',
      'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36',
      0,
      { browser: 'Chrome', platform: 'Android' }
    ],
    [
      'Safari on iOS',
      'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1',
      5,
      { browser: 'Safari', platform: 'iOS' }
    ],
    [
      'Safari on an iPad that reports a Mac',
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15',
      5,
      { browser: 'Safari', platform: 'iPadOS' }
    ],
    ['an unknown agent', 'SomeAgent/1.0', 0, { browser: null, platform: null }]
  ])('describes %s', (_name, userAgent, maxTouchPoints, expected) => {
    expect(describePushDevice(userAgent, maxTouchPoints)).toEqual(expected);
  });
});
