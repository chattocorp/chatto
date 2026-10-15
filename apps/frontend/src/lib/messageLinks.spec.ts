import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { getToasts, toast } from '$lib/ui/toast';
import { serverRegistry } from '$lib/client';
import { type RegisteredServer } from '@chatto/client/server/registry';
import {
  buildMessageLinkURL,
  buildRoomLinkURL,
  classifyMessageBodyChatLink,
  copyMessageLinkToClipboard,
  copyRoomLinkToClipboard,
  parseMessageLink
} from './messageLinks';

const origin = 'https://chat.example.test';
const registeredRemoteOrigin = 'https://chat.chatto.run';
const channelRoomId = 'R123456789abcde';
const dmRoomId = 'abcdef12345678';
const messageId = 'Eabc123DEF456gh';
const threadRootEventId = 'Ethread12345678';

const remoteServer: RegisteredServer = {
  id: 'remote',
  url: 'https://remote.example.test',
  name: 'Remote',
  iconUrl: null,
  token: null,
  userId: null,
  userLogin: null,
  userDisplayName: null,
  userAvatarUrl: null,
  reauthRequiredAt: null,
  addedAt: 1
};

function resolveServerSegment(segment: string): string | null {
  if (segment === '-') return 'origin';
  if (segment === 'remote.example.test') return 'remote';
  return null;
}

function resolveUrlOrigin(urlOrigin: string): string | null {
  if (urlOrigin === registeredRemoteOrigin) return 'chatto-run';
  return null;
}

function serverSegmentForId(serverId: string): string {
  if (serverId === 'origin') return '-';
  if (serverId === 'remote') return 'remote.example.test';
  if (serverId === 'chatto-run') return 'chat.chatto.run';
  return serverId;
}

function classify(input: string) {
  return classifyMessageBodyChatLink(input, {
    origin,
    resolveServerSegment,
    resolveUrlOrigin,
    serverSegmentForId
  });
}

describe('copied chat URLs', () => {
  beforeEach(() => {
    vi.stubGlobal('window', { location: { origin } });
    vi.spyOn(serverRegistry, 'getServer').mockReturnValue(remoteServer);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('uses the owning origin and home segment for an origin-server message', () => {
    vi.mocked(serverRegistry.getServer).mockReturnValue({ ...remoteServer, url: origin });

    expect(buildMessageLinkURL('origin', 'room-1', 'message-1')).toBe(
      `${origin}/chat/-/room-1/m/message-1`
    );
  });

  it('uses the remote origin and home segment for a remote-server message', () => {
    expect(buildMessageLinkURL('remote', 'room-1', 'message-1')).toBe(
      `${remoteServer.url}/chat/-/room-1/m/message-1`
    );
  });

  it('preserves the thread root in a remote-server message link', () => {
    expect(buildMessageLinkURL('remote', 'room-1', 'message-1', 'thread-root-1')).toBe(
      `${remoteServer.url}/chat/-/room-1/thread-root-1/m/message-1`
    );
  });

  it.each(['https://remote.example.test:8443', 'http://localhost:8080'])(
    'preserves the owning server scheme and port: %s',
    (url) => {
      vi.mocked(serverRegistry.getServer).mockReturnValue({ ...remoteServer, url });
      expect(buildRoomLinkURL('remote', channelRoomId)).toBe(`${url}/chat/-/${channelRoomId}`);
      expect(buildMessageLinkURL('remote', channelRoomId, messageId)).toBe(
        `${url}/chat/-/${channelRoomId}/m/${messageId}`
      );
    }
  );

  it('builds absolute links without a browser', () => {
    vi.stubGlobal('window', undefined);
    expect(buildRoomLinkURL('remote', channelRoomId)).toBe(
      `${remoteServer.url}/chat/-/${channelRoomId}`
    );
  });

  it('does not substitute the frontend when the owning server is missing', () => {
    vi.mocked(serverRegistry.getServer).mockReturnValue(undefined);
    expect(() => buildRoomLinkURL('missing', channelRoomId)).toThrow('unknown server');
    expect(() => buildMessageLinkURL('missing', channelRoomId, messageId)).toThrow(
      'unknown server'
    );
  });
});

describe('copyMessageLinkToClipboard', () => {
  const writeText = vi.fn();

  beforeEach(() => {
    vi.spyOn(serverRegistry, 'getServer').mockReturnValue(remoteServer);
    toast.clear();
    writeText.mockReset();
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true
    });
  });

  afterEach(() => vi.restoreAllMocks());

  it('copies a room link on the owning server', async () => {
    await copyRoomLinkToClipboard('remote', channelRoomId);
    expect(writeText).toHaveBeenCalledWith(`${remoteServer.url}/chat/-/${channelRoomId}`);
    expect(getToasts().map((t) => t.message)).toContain('Link copied');
  });

  it('reports room clipboard failures', async () => {
    writeText.mockRejectedValue(new Error('denied'));
    await copyRoomLinkToClipboard('remote', channelRoomId);
    expect(getToasts().map((t) => t.message)).toContain('Failed to copy link');
  });

  it('reports a missing owning server without copying a fallback URL', async () => {
    vi.mocked(serverRegistry.getServer).mockReturnValue(undefined);
    await copyRoomLinkToClipboard('missing', channelRoomId);
    await copyMessageLinkToClipboard('missing', channelRoomId, messageId);
    expect(writeText).not.toHaveBeenCalled();
    expect(getToasts().map((t) => t.message)).toContain('Failed to copy link');
  });

  it('copies the message link and shows a success toast', async () => {
    writeText.mockResolvedValue(undefined);

    await copyMessageLinkToClipboard('server-1', 'room-1', 'message-1');

    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('/chat/-/room-1/m/message-1'));
    expect(getToasts().map((t) => t.message)).toContain('Message link copied');
  });

  it('shows an error toast when clipboard copy fails', async () => {
    writeText.mockRejectedValue(new Error('denied'));

    await copyMessageLinkToClipboard('server-1', 'room-1', 'message-1');

    expect(getToasts().map((t) => t.message)).toContain('Failed to copy link');
  });

  it('copies a nested thread message link when a thread root is provided', async () => {
    writeText.mockResolvedValue(undefined);

    await copyMessageLinkToClipboard('server-1', 'room-1', 'message-1', 'thread-root-1');

    expect(writeText).toHaveBeenCalledWith(
      expect.stringContaining('/chat/-/room-1/thread-root-1/m/message-1')
    );
  });
});

describe('classifyMessageBodyChatLink', () => {
  it('accepts same-origin room URLs with channel room IDs', () => {
    expect(classify(`${origin}/chat/-/${channelRoomId}`)).toMatchObject({
      kind: 'room',
      serverId: 'origin',
      roomId: channelRoomId,
      path: `/chat/-/${channelRoomId}`
    });
  });

  it('accepts same-origin room URLs with DM room IDs', () => {
    expect(classify(`${origin}/chat/-/${dmRoomId}`)).toMatchObject({
      kind: 'room',
      roomId: dmRoomId,
      path: `/chat/-/${dmRoomId}`
    });
  });

  it('accepts same-origin thread URLs', () => {
    expect(classify(`${origin}/chat/-/${channelRoomId}/${threadRootEventId}`)).toMatchObject({
      kind: 'thread',
      roomId: channelRoomId,
      threadRootEventId,
      path: `/chat/-/${channelRoomId}/${threadRootEventId}`
    });
  });

  it('accepts same-origin message URLs', () => {
    expect(classify(`${origin}/chat/-/${channelRoomId}/m/${messageId}`)).toMatchObject({
      kind: 'message',
      roomId: channelRoomId,
      messageId,
      path: `/chat/-/${channelRoomId}/m/${messageId}`
    });
  });

  it('accepts same-origin thread message URLs', () => {
    expect(
      classify(
        `${origin}/chat/-/${channelRoomId}/${threadRootEventId}/m/${messageId}?focus=1#message`
      )
    ).toMatchObject({
      kind: 'thread-message',
      roomId: channelRoomId,
      threadRootEventId,
      messageId,
      path: `/chat/-/${channelRoomId}/${threadRootEventId}/m/${messageId}?focus=1#message`
    });
  });

  it('accepts known remote server segments on the same origin', () => {
    expect(classify(`${origin}/chat/remote.example.test/${channelRoomId}`)).toMatchObject({
      kind: 'room',
      serverId: 'remote',
      serverSegment: 'remote.example.test'
    });
  });

  it('maps registered-server absolute room URLs back to local app routes', () => {
    expect(classify(`${registeredRemoteOrigin}/chat/-/${channelRoomId}`)).toMatchObject({
      kind: 'room',
      serverId: 'chatto-run',
      serverSegment: 'chat.chatto.run',
      roomId: channelRoomId,
      path: `/chat/chat.chatto.run/${channelRoomId}`
    });
  });

  it('maps registered-server absolute message URLs back to local app routes', () => {
    expect(
      classify(`${registeredRemoteOrigin}/chat/-/${channelRoomId}/m/${messageId}`)
    ).toMatchObject({
      kind: 'message',
      serverId: 'chatto-run',
      serverSegment: 'chat.chatto.run',
      roomId: channelRoomId,
      messageId,
      path: `/chat/chat.chatto.run/${channelRoomId}/m/${messageId}`
    });
  });

  it('preserves search and hash when mapping registered-server links', () => {
    expect(classify(`${registeredRemoteOrigin}/chat/-/${channelRoomId}?a=1#bottom`)).toMatchObject({
      path: `/chat/chat.chatto.run/${channelRoomId}?a=1#bottom`
    });
  });

  it('rejects same-origin non-chat and reserved chat URLs', () => {
    const rejected = [
      `${origin}/chat/-/settings`,
      `${origin}/chat/-/overview`,
      `${origin}/chat/-/threads`,
      `${origin}/chat/-/manage/server/general`,
      `${origin}/chat/notifications`,
      `${origin}/login`
    ];

    for (const url of rejected) {
      expect(classify(url)).toBeNull();
    }
  });

  it('rejects malformed room and event IDs', () => {
    const rejected = [
      `${origin}/chat/-/room-1`,
      `${origin}/chat/-/R123/m/${messageId}`,
      `${origin}/chat/-/${channelRoomId}/m/message-1`,
      `${origin}/chat/-/${channelRoomId}/thread-1`,
      `${origin}/chat/-/${channelRoomId}/${threadRootEventId}/m/message-1`
    ];

    for (const url of rejected) {
      expect(classify(url)).toBeNull();
    }
  });

  it('rejects unknown server segments', () => {
    expect(classify(`${origin}/chat/unknown.example.test/${channelRoomId}`)).toBeNull();
  });

  it('rejects unregistered cross-origin URLs', () => {
    expect(classify(`https://other.example.test/chat/-/${channelRoomId}`)).toBeNull();
  });
});

describe('parseMessageLink', () => {
  beforeEach(() => {
    vi.stubGlobal('window', { location: { origin } });
    vi.spyOn(serverRegistry, 'servers', 'get').mockReturnValue([remoteServer]);
    vi.spyOn(serverRegistry, 'getServer').mockReturnValue(remoteServer);
    vi.spyOn(serverRegistry, 'isOriginServer').mockReturnValue(false);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('resolves copied message links by their full registered origin', () => {
    const url = buildMessageLinkURL('remote', channelRoomId, messageId, threadRootEventId);
    expect(parseMessageLink(url)).toMatchObject({
      serverId: 'remote',
      roomId: channelRoomId,
      messageId,
      threadRootEventId
    });
    expect(classifyMessageBodyChatLink(url)).toMatchObject({
      serverId: 'remote',
      path: `/chat/remote.example.test/${channelRoomId}/${threadRootEventId}/m/${messageId}`
    });
  });

  it.each(['http://remote.example.test', 'https://remote.example.test:8443'])(
    'does not confuse a different origin with a known server: %s',
    (urlOrigin) => {
      const url = `${urlOrigin}/chat/-/${channelRoomId}/m/${messageId}`;
      expect(parseMessageLink(url)?.serverId).toBeNull();
      expect(classifyMessageBodyChatLink(url)).toBeNull();
    }
  );

  it('keeps old frontend-hosted links to a remote server readable', () => {
    const url = `${origin}/chat/remote.example.test/${channelRoomId}/m/${messageId}`;
    expect(parseMessageLink(url)?.serverId).toBe('remote');
    expect(classifyMessageBodyChatLink(url)?.serverId).toBe('remote');
  });

  it('preserves the thread root from a nested message link', () => {
    expect(
      parseMessageLink(`/chat/-/${channelRoomId}/${threadRootEventId}/m/${messageId}`)
    ).toMatchObject({
      roomId: channelRoomId,
      threadRootEventId,
      messageId
    });
  });
});
