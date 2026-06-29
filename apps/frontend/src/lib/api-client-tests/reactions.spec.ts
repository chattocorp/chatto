import { Code, ConnectError } from '@connectrpc/connect';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '@chatto/api-client/hooks';
import { createReactionAPI } from '@chatto/api-client/reactions';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  handleAuthenticationRequired: vi.fn(),
  addReaction: vi.fn(),
  removeReaction: vi.fn()
}));

vi.mock('@connectrpc/connect', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@connectrpc/connect')>();
  return {
    ...actual,
    createClient: mocks.createClient
  };
});

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport
}));

describe('createReactionAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.handleAuthenticationRequired.mockReset();

    configureApiClientHooks({ onAuthenticationRequired: mocks.handleAuthenticationRequired });
    mocks.addReaction.mockReset();
    mocks.removeReaction.mockReset();
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      addReaction: mocks.addReaction,
      removeReaction: mocks.removeReaction
    });
  });

  it('adds a reaction with bearer auth', async () => {
    mocks.addReaction.mockResolvedValue({ added: true });

    const api = createReactionAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });
    const added = await api.addReaction({
      roomId: 'room-1',
      messageEventId: 'event-1',
      emoji: 'thumbsup'
    });

    expect(mocks.createConnectTransport).toHaveBeenCalledWith({
      baseUrl: 'https://remote.example.test/api/connect',
      useBinaryFormat: true
    });
    expect(mocks.addReaction).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        messageEventId: 'event-1',
        emoji: 'thumbsup'
      },
      {
        headers: { Authorization: 'Bearer remote-token' }
      }
    );
    expect(added).toBe(true);
  });

  it('removes a reaction without auth headers when no token is available', async () => {
    mocks.removeReaction.mockResolvedValue({ removed: false });

    const api = createReactionAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });
    const removed = await api.removeReaction({
      roomId: 'room-1',
      messageEventId: 'event-1',
      emoji: 'thumbsup'
    });

    expect(mocks.removeReaction).toHaveBeenCalledWith(
      {
        roomId: 'room-1',
        messageEventId: 'event-1',
        emoji: 'thumbsup'
      },
      {
        headers: undefined
      }
    );
    expect(removed).toBe(false);
  });

  it('marks the server authentication stale on unauthenticated Connect errors', async () => {
    const err = new ConnectError('authentication required', Code.Unauthenticated);
    mocks.addReaction.mockRejectedValue(err);

    const api = createReactionAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'expired-token'
    });

    await expect(
      api.addReaction({ roomId: 'room-1', messageEventId: 'event-1', emoji: 'thumbsup' })
    ).rejects.toBe(err);

    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
  });
});
