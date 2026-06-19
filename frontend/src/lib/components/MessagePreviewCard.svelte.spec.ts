import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { Timestamp } from '@bufbuild/protobuf';
import MessagePreviewCard from './MessagePreviewCard.svelte';
import type { MessageLink } from '$lib/messageLinks';
import {
  AssetFitMode,
  RefreshMessageAttachmentUrlsRequest,
  RefreshMessageAttachmentUrlsResponse
} from '$lib/pb/chatto/api/v1/chat_pb';

const { fetchMock, getRoomMock, getRoomEventMock } = vi.hoisted(() => ({
  fetchMock: vi.fn(),
  getRoomMock: vi.fn(),
  getRoomEventMock: vi.fn()
}));

vi.mock('$lib/state/server/wireEventBus.svelte', () => ({
  wireEventBusManager: {
    getClient: () => ({
      getRoom: getRoomMock,
      getRoomEvent: getRoomEventMock
    })
  }
}));

vi.mock('$lib/state/server/registry.svelte', () => ({
  serverRegistry: {
    getServer: (id: string) =>
      id === 'server_1'
        ? { id: 'server_1', url: window.location.origin, name: 'Test Server', token: null }
        : undefined,
    isOriginServer: (id: string) => id === 'server_1',
    get originServer() {
      return { id: 'server_1', url: window.location.origin, name: 'Test Server', token: null };
    },
    servers: [{ id: 'server_1', url: window.location.origin, name: 'Test Server', token: null }]
  }
}));

function link(): MessageLink {
  return {
    serverSegment: '-',
    serverId: 'server_1',
    roomId: 'room_1',
    messageId: 'event_1'
  };
}

function previewEventResult(thumbnailUrl: string) {
  return {
    event: {
      id: 'event_1',
      actor: undefined,
      event: {
        payload: {
          case: 'messagePosted',
          value: {
            roomId: 'room_1',
            body: undefined,
            attachments: [
              {
                id: 'att_1',
                filename: 'photo.jpg',
                contentType: 'image/jpeg',
                thumbnailAssetUrl: {
                  url: thumbnailUrl,
                  expiresAt: timestamp('2027-05-29T15:00:00Z')
                }
              }
            ],
            reactions: [],
            replyCount: 0,
            threadParticipants: []
          }
        }
      }
    }
  };
}

function timestamp(iso: string) {
  return {
    toDate: () => new Date(iso)
  };
}

function refreshResult(thumbnailUrl: string) {
  const bytes = new RefreshMessageAttachmentUrlsResponse({
    attachments: [
      {
        id: 'att_1',
        assetUrl: {
          url: '/assets/files/att_1?access=fresh-original',
          expiresAt: Timestamp.fromDate(new Date('2027-05-29T15:00:00Z'))
        },
        thumbnailAssetUrl: {
          url: thumbnailUrl,
          expiresAt: Timestamp.fromDate(new Date('2027-05-29T15:00:00Z'))
        }
      }
    ]
  }).toBinary();
  return new Response(
    bytes.slice().buffer as ArrayBuffer,
    {
      status: 200,
      headers: { 'Content-Type': 'application/protobuf' }
    }
  );
}

beforeEach(() => {
  fetchMock.mockReset();
  getRoomMock.mockReset();
  getRoomEventMock.mockReset();
  vi.stubGlobal('fetch', fetchMock);
});

describe('MessagePreviewCard', () => {
  it('refreshes attachment thumbnail asset URLs after image load failure', async () => {
    fetchMock.mockResolvedValue(
      refreshResult('/assets/files/att_1/image/120x120/cover?access=fresh')
    );
    getRoomMock.mockResolvedValue({
      serverName: 'Test Server',
      room: { name: 'general' }
    });
    getRoomEventMock.mockResolvedValue(
      previewEventResult('data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==')
    );

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"]')).not.toBeNull();
    });

    const img = container.querySelector<HTMLImageElement>('img[alt="photo.jpg"]');
    expect(img).not.toBeNull();
    img?.dispatchEvent(new Event('error'));

    await vi.waitFor(() => {
      const refreshed = container.querySelector<HTMLImageElement>('img[alt="photo.jpg"]');
      expect(refreshed?.getAttribute('src')).toContain(
        '/assets/files/att_1/image/120x120/cover?access=fresh'
      );
    });
    const refreshCalls = fetchMock.mock.calls.filter((call) =>
      String(call[0]).includes('/attachments/urls/refresh')
    );
    expect(refreshCalls.length).toBeGreaterThanOrEqual(1);
    for (const call of refreshCalls) {
      const init = call[1] as RequestInit;
      const request = RefreshMessageAttachmentUrlsRequest.fromBinary(
        new Uint8Array(init.body as ArrayBuffer)
      );
      expect(request).toMatchObject({
        roomId: 'room_1',
        eventId: 'event_1',
        thumbnailWidth: 120,
        thumbnailHeight: 120,
        thumbnailFit: AssetFitMode.COVER
      });
    }
  });
});
