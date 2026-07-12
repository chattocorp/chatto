import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getToasts, toast } from '$lib/ui/toast';
import { classifyMessageBodyChatLink, copyMessageLinkToClipboard } from './messageLinks';

const origin = 'https://chat.example.test';
const registeredRemoteOrigin = 'https://chat.chatto.run';
const channelRoomId = 'R123456789abcde';
const dmRoomId = 'abcdef12345678';
const messageId = 'Eabc123DEF456gh';
const threadRootEventId = 'Ethread12345678';

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

describe('copyMessageLinkToClipboard', () => {
  const writeText = vi.fn();

  beforeEach(() => {
    toast.clear();
    writeText.mockReset();
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true
    });
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
      `${origin}/chat/-/server-admin/general`,
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
      `${origin}/chat/-/${channelRoomId}/thread-1`
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
