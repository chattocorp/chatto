import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessagePreviewCard from './MessagePreviewCard.svelte';
import type { MessageLink } from '$lib/messageLinks';
import { FitMode } from '$lib/gql/graphql';

const { queryMock, queryResults } = vi.hoisted(() => ({
  queryMock: vi.fn(),
  queryResults: [] as unknown[]
}));

vi.mock('$lib/state/server/graphqlClient.svelte', () => ({
  graphqlClientManager: {
    getClient: () => ({
      client: {
        query: queryMock
      }
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

function previewResult(thumbnailUrl: string) {
  return {
    data: {
      server: {
        profile: {
          name: 'Test Server'
        }
      },
      room: {
        name: 'general',
        event: {
          actor: null,
          event: {
            __typename: 'MessagePostedEvent',
            body: null,
            attachments: [
              {
                id: 'att_1',
                filename: 'photo.jpg',
                contentType: 'image/jpeg',
                thumbnailAssetUrl: {
                  url: thumbnailUrl,
                  expiresAt: '2027-05-29T15:00:00Z'
                }
              }
            ]
          }
        }
      }
    }
  };
}

function refreshResult(thumbnailUrl: string) {
  return {
    data: {
      room: {
        event: {
          event: {
            __typename: 'MessagePostedEvent',
            attachments: [
              {
                id: 'att_1',
                assetUrl: {
                  url: '/assets/files/att_1?access=fresh-original',
                  expiresAt: '2027-05-29T15:00:00Z'
                },
                thumbnailAssetUrl: {
                  url: thumbnailUrl,
                  expiresAt: '2027-05-29T15:00:00Z'
                },
                videoProcessing: null
              }
            ]
          }
        }
      }
    }
  };
}

beforeEach(() => {
  queryMock.mockReset();
  queryResults.length = 0;
  queryMock.mockImplementation(() => ({
    toPromise: () => Promise.resolve(queryResults.shift())
  }));
});

describe('MessagePreviewCard', () => {
  it('refreshes attachment thumbnail asset URLs after image load failure', async () => {
    queryResults.push(
      previewResult('/assets/files/att_1/image/120x120/cover?access=old'),
      refreshResult('/assets/files/att_1/image/120x120/cover?access=fresh')
    );

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"]')).not.toBeNull();
    });

    const img = container.querySelector<HTMLImageElement>('img[alt="photo.jpg"]');
    if (img?.getAttribute('src')?.includes('access=old')) {
      img.dispatchEvent(new Event('error'));
    }

    await vi.waitFor(() => {
      const refreshed = container.querySelector<HTMLImageElement>('img[alt="photo.jpg"]');
      expect(refreshed?.getAttribute('src')).toContain(
        '/assets/files/att_1/image/120x120/cover?access=fresh'
      );
    });
    expect(queryMock).toHaveBeenCalledTimes(2);
    expect(queryMock.mock.calls[1]?.[1]).toMatchObject({
      roomId: 'room_1',
      eventId: 'event_1',
      thumbnailWidth: 120,
      thumbnailHeight: 120,
      thumbnailFit: FitMode.Cover
    });
  });
});
