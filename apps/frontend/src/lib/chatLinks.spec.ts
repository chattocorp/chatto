import { copyMessageLinkToClipboard, copyRoomLinkToClipboard } from './copyChatLink';
import { afterEach, beforeEach, describe, expect, it, onTestFinished, vi } from 'vitest';
import { getToasts, toast } from '$lib/ui/toast';
import { serverRegistry } from '$lib/client';
import { createClient } from '@chatto/client';
import { RoomWithViewerState } from '@chatto/api-types/api/v1/room_directory_pb';
import { RoomKind } from '@chatto/client/api/roomDirectory';
import { type RegisteredServer } from '@chatto/client/server/registry';
import {
  buildMessageLinkURL,
  buildRoomLinkURL,
  resolveChatRoute,
  parseMessageLink
} from './chatLinks';

const origin = 'https://chat.example.test';
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

describe('copied chat URLs', () => {
  beforeEach(() => {
    vi.stubGlobal('window', { location: { origin } });
    vi.spyOn(serverRegistry, 'getServer').mockReturnValue(remoteServer);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('copies and previews user-page message links through the viewer conversation', () => {
    vi.stubGlobal('window', {
      location: { origin },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn()
    });
    const client = createClient();
    onTestFinished(() => client.close());
    client.registry.addServer(remoteServer);
    const store = client.registry.getStore('remote');
    vi.spyOn(store, 'accountId', 'get').mockReturnValue('viewer');
    store.projection.rooms.set(
      dmRoomId,
      new RoomWithViewerState({
        room: { id: dmRoomId, kind: RoomKind.DM },
        memberUserIds: ['viewer', 'peer']
      })
    );
    vi.spyOn(serverRegistry, 'tryGetStore').mockReturnValue(store);
    vi.spyOn(serverRegistry, 'servers', 'get').mockReturnValue([remoteServer]);
    const url = buildMessageLinkURL('remote', dmRoomId, messageId, threadRootEventId);
    expect(url).toBe(`${remoteServer.url}/chat/-/user/peer/${threadRootEventId}/m/${messageId}`);
    expect(parseMessageLink(url)).toEqual({
      serverId: 'remote',
      roomId: dmRoomId,
      threadRootEventId,
      messageId
    });
    expect(parseMessageLink(`${remoteServer.url}/chat/-/user/unknown/m/${messageId}`)).toBeNull();
    expect(buildRoomLinkURL('remote', dmRoomId)).toBe(`${remoteServer.url}/chat/-/user/peer`);
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
    expect(resolveChatRoute(url)).toBe(
      `/chat/remote.example.test/${channelRoomId}/${threadRootEventId}/m/${messageId}`
    );
  });

  it.each(['http://remote.example.test', 'https://remote.example.test:8443'])(
    'resolves registered hostnames independently of scheme and port: %s',
    (urlOrigin) => {
      const url = `${urlOrigin}/chat/-/${channelRoomId}/m/${messageId}`;
      expect(parseMessageLink(url)?.serverId).toBe('remote');
      expect(resolveChatRoute(url)).toBe(
        `/chat/remote.example.test/${channelRoomId}/m/${messageId}`
      );
    }
  );

  it('keeps old frontend-hosted links to a remote server readable', () => {
    const url = `${origin}/chat/remote.example.test/${channelRoomId}/m/${messageId}`;
    expect(parseMessageLink(url)?.serverId).toBe('remote');
    expect(resolveChatRoute(url)).toBe(`/chat/remote.example.test/${channelRoomId}/m/${messageId}`);
  });

  it.each([
    '',
    '/',
    '/overview',
    '/search?q=hello#results',
    '/threads',
    '/settings/appearance',
    '/manage/rooms',
    `/${channelRoomId}`,
    `/${dmRoomId}`,
    `/${channelRoomId}/${threadRootEventId}`
  ])('rewrites general server routes without treating them as message previews: %s', (suffix) => {
    const url = `https://remote.example.test/chat/-${suffix}`;
    expect(resolveChatRoute(url)).toBe(`/chat/remote.example.test${suffix}`);
    expect(parseMessageLink(url)).toBeNull();
  });

  it.each(['https://remote.example.test', 'https://remote.example.test/'])(
    'rewrites a registered server root into this frontend: %s',
    (url) => {
      expect(resolveChatRoute(url)).toBe('/chat/remote.example.test');
      expect(parseMessageLink(url)).toBeNull();
    }
  );

  it.each([
    'https://unknown.test/chat/-/overview',
    'https://unknown.test/',
    'https://remote.example.test/?token=example',
    'https://remote.example.test/chat/unknown.test/overview',
    'https://remote.example.test/docs',
    'https://remote.example.test/chat/%2F/overview',
    'https://remote.example.test/chat/-/%broken'
  ])('keeps unresolvable routes external: %s', (url) => {
    expect(resolveChatRoute(url)).toBeNull();
  });

  it('does not trust an unknown origin even when its path names a known server', () => {
    const url = `https://unknown.test/chat/remote.example.test/${channelRoomId}/m/${messageId}`;
    expect(resolveChatRoute(url)).toBeNull();
    expect(parseMessageLink(url)).toMatchObject({
      serverId: null,
      roomId: channelRoomId,
      messageId
    });
  });

  it('accepts root-relative links on the desktop app origin', () => {
    vi.stubGlobal('window', { location: { origin: 'chatto://desktop' } });
    const path = `/chat/remote.example.test/${channelRoomId}/m/${messageId}`;
    expect(resolveChatRoute(path)).toBe(path);
    expect(parseMessageLink(path)?.serverId).toBe('remote');
    expect(parseMessageLink(`chatto://other${path}`)).toBeNull();
  });

  it('decodes valid IDs and keeps query and hash when producing a local route', () => {
    const url = `${remoteServer.url}/chat/-/%52${channelRoomId.slice(1)}/m/${messageId}/?focus=1#target`;
    expect(resolveChatRoute(url)).toBe(
      `/chat/remote.example.test/%52${channelRoomId.slice(1)}/m/${messageId}/?focus=1#target`
    );
    expect(parseMessageLink(url)).toMatchObject({
      serverId: 'remote',
      roomId: channelRoomId,
      messageId
    });
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

  it.each(['https://remote.example.test:8443', 'http://remote.example.test', 'http://[::1]:8080'])(
    'maps an owning-origin share URL to a hostname-only local route: %s',
    (url) => {
      const server = { ...remoteServer, url };
      vi.mocked(serverRegistry.getServer).mockReturnValue(server);
      vi.spyOn(serverRegistry, 'servers', 'get').mockReturnValue([server]);
      const local = `/chat/${new URL(url).hostname}/${channelRoomId}/m/${messageId}`;
      const shared = buildMessageLinkURL('remote', channelRoomId, messageId);
      expect(resolveChatRoute(shared)).toBe(local);
      expect(resolveChatRoute(local)).toBe(local);
      expect(parseMessageLink(local)?.serverId).toBe('remote');
    }
  );

  it.each([
    '/chat/-/room-1/m/Eabc123DEF456gh',
    '/chat/-/R123456789abcde/m/message-1',
    '/chat//-/R123456789abcde/m/Eabc123DEF456gh',
    '/chat/-/R123456789abcde/m/Eabc123DEF456gh/extra',
    '/chat/-/R123456789abcde/m/%broken',
    '/chat/-/R123456789abcde/m/Eabc123DEF456gh%2F',
    'ftp://remote.example.test/chat/-/R123456789abcde/m/Eabc123DEF456gh',
    'https://user:password@remote.example.test/chat/-/R123456789abcde/m/Eabc123DEF456gh'
  ])('rejects malformed message previews: %s', (input) => {
    expect(parseMessageLink(input)).toBeNull();
  });
});
