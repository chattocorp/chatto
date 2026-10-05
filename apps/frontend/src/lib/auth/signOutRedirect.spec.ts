import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  cancelExplicitSignOutRedirect,
  isExplicitSignOutRedirectInProgress
} from '@chatto/client/auth/signOut';
import { hardRedirectAfterSignOut } from './signOutRedirect';

const replace = vi.fn();

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal('window', {
    location: { href: 'https://chat.example/chat/-/room', origin: 'https://chat.example', replace },
    setTimeout: globalThis.setTimeout
  });
});

afterEach(() => {
  cancelExplicitSignOutRedirect();
  replace.mockReset();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('hardRedirectAfterSignOut', () => {
  it('marks the redirect and replaces the document with a same-origin path', () => {
    hardRedirectAfterSignOut('/chat/remote.example?x=1#top');
    expect(isExplicitSignOutRedirectInProgress()).toBe(true);
    expect(replace).not.toHaveBeenCalled();
    vi.runAllTimers();
    expect(replace).toHaveBeenCalledWith('/chat/remote.example?x=1#top');
  });

  it('navigates to another origin as given', () => {
    hardRedirectAfterSignOut('https://login.example/start');
    vi.runAllTimers();
    expect(replace).toHaveBeenCalledWith('https://login.example/start');
  });

  it('navigates to an unreadable target as given', () => {
    hardRedirectAfterSignOut('http://[invalid');
    vi.runAllTimers();
    expect(replace).toHaveBeenCalledWith('http://[invalid');
  });

  it('goes to the root by default', () => {
    hardRedirectAfterSignOut();
    vi.runAllTimers();
    expect(replace).toHaveBeenCalledWith('/');
  });
});
