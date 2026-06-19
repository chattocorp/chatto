import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  CurrentUserPresenceStatus,
  CurrentUserView,
  GetCurrentUserResponse
} from '$lib/pb/chatto/api/v1/chat_pb';
import { User } from '$lib/pb/chatto/core/v1/models_pb';
import {
  ServerUserPreferences,
  TimeFormat as WireTimeFormat
} from '$lib/pb/chatto/core/v1/user_preferences_pb';
import { ErrorCode, WireError } from '$lib/pb/chatto/wire/v1/protocol_pb';

const { getCurrentUserMock, disposeMock, clearOriginAuthenticationMock } = vi.hoisted(() => ({
  getCurrentUserMock: vi.fn(),
  disposeMock: vi.fn(),
  clearOriginAuthenticationMock: vi.fn()
}));

vi.mock('$app/environment', () => ({
  browser: true
}));

vi.mock('$app/paths', () => ({
  resolve: (path: string) => path
}));

vi.mock('$lib/state/server/serverConnection.svelte', () => ({
  serverConnectionManager: {
    originClient: {
      wireUrl: '/api/wire',
      token: 'origin-token'
    }
  }
}));

vi.mock('$lib/wire/client', () => {
  class WireProtocolError extends Error {
    wireError?: { code: ErrorCode };

    constructor(message: string, wireError?: { code: ErrorCode }) {
      super(message);
      this.name = 'WireProtocolError';
      this.wireError = wireError;
    }
  }

  return {
    WireProtocolError,
    WireClient: vi.fn().mockImplementation(function () {
      return {
        getCurrentUser: getCurrentUserMock,
        dispose: disposeMock
      };
    })
  };
});

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    clearOriginAuthentication: clearOriginAuthenticationMock
  }
}));

const user = {
  id: 'U1',
  login: 'alice',
  displayName: 'Alice',
  avatarUrl: null,
  presenceStatus: 'ONLINE',
  hasVerifiedEmail: true,
  settings: { timezone: 'UTC', timeFormat: 'TWENTY_FOUR_HOUR' }
};

function currentUserResponse(overrides: Partial<typeof user> = {}): GetCurrentUserResponse {
  const merged = { ...user, ...overrides };
  return new GetCurrentUserResponse({
    user: new CurrentUserView({
      user: new User({
        id: merged.id,
        login: merged.login,
        displayName: merged.displayName
      }),
      avatarUrl: merged.avatarUrl ?? '',
      presenceStatus:
        merged.presenceStatus === 'AWAY'
          ? CurrentUserPresenceStatus.AWAY
          : merged.presenceStatus === 'DO_NOT_DISTURB'
            ? CurrentUserPresenceStatus.DO_NOT_DISTURB
            : merged.presenceStatus === 'OFFLINE'
              ? CurrentUserPresenceStatus.OFFLINE
              : CurrentUserPresenceStatus.ONLINE,
      hasVerifiedEmail: merged.hasVerifiedEmail,
      settings: new ServerUserPreferences({
        timezone: merged.settings?.timezone ?? '',
        timeFormat:
          merged.settings?.timeFormat === 'TWELVE_HOUR'
            ? WireTimeFormat.TIME_FORMAT_12H
            : merged.settings?.timeFormat === 'AUTO'
              ? WireTimeFormat.TIME_FORMAT_UNSPECIFIED
              : WireTimeFormat.TIME_FORMAT_24H
      })
    })
  });
}

async function loadModule() {
  vi.resetModules();
  return import('./loadAuth');
}

describe('loadCurrentUser', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('refreshes from the server on each call', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserMock
      .mockResolvedValueOnce(currentUserResponse())
      .mockResolvedValueOnce(currentUserResponse({ displayName: 'Alice Fresh' }));

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toEqual({ ...user, displayName: 'Alice Fresh' });
    expect(getCurrentUserMock).toHaveBeenCalledTimes(2);
    expect(disposeMock).toHaveBeenCalledTimes(2);
  });

  it('keeps the cached user when a later refresh errors', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserMock
      .mockResolvedValueOnce(currentUserResponse())
      .mockRejectedValueOnce(new Error('not found'))
      .mockRejectedValueOnce(new Error('not found'));

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toEqual(user);
  });

  it('clears the cached user on a clean viewer=null response', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserMock
      .mockResolvedValueOnce(currentUserResponse())
      .mockResolvedValueOnce(new GetCurrentUserResponse());

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toBeNull();
  });

  it('returns null when the first load cannot determine a user', async () => {
    const { loadCurrentUser } = await loadModule();
    getCurrentUserMock.mockRejectedValue(new Error('unreachable'));

    expect(await loadCurrentUser()).toBeNull();
  });

  it('clears origin auth on authentication-required errors', async () => {
    const { loadCurrentUser } = await loadModule();
    const { WireProtocolError } = await import('$lib/wire/client');
    getCurrentUserMock
      .mockResolvedValueOnce(currentUserResponse())
      .mockRejectedValueOnce(
        new WireProtocolError(
          'authentication required',
          new WireError({ code: ErrorCode.UNAUTHENTICATED })
        )
      );

    expect(await loadCurrentUser()).toEqual(user);
    expect(await loadCurrentUser()).toBeNull();
    expect(clearOriginAuthenticationMock).toHaveBeenCalledOnce();
  });
});
