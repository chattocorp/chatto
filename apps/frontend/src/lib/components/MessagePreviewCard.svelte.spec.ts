import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessagePreviewCard from './MessagePreviewCard.svelte';
import type { MessageLink } from '$lib/messageLinks';
import { FitMode } from '$lib/gql/graphql';

const { queryMock, queryResults } = vi.hoisted(() => ({
  queryMock: vi.fn(),
  queryResults: [] as unknown[]
}));

const transparentGif = 'data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==';

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
    tryGetStore: () => ({
      currentUser: {
        user: { login: 'viewer' }
      }
    }),
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

vi.mock('$lib/state/activeServer.svelte', () => ({
  getActiveServer: () => 'server_1'
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

function bodyPreviewResult(body: string) {
  return {
    data: {
      server: {
        profile: {
          name: 'Test Server'
        }
      },
      room: {
        name: 'announcements',
        event: {
          actor: null,
          event: {
            __typename: 'MessagePostedEvent',
            body,
            attachments: []
          }
        }
      }
    }
  };
}

function videoPreviewResult(videoThumbnailUrl: string | null) {
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
                id: 'att_video',
                filename: 'clip.mp4',
                contentType: 'video/mp4',
                thumbnailAssetUrl: null,
                videoProcessing: videoThumbnailUrl
                  ? {
                      thumbnailAssetUrl: {
                        url: videoThumbnailUrl,
                        expiresAt: '2027-05-29T15:00:00Z'
                      }
                    }
                  : null
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

function videoRefreshResult(videoThumbnailUrl: string) {
  return {
    data: {
      room: {
        event: {
          event: {
            __typename: 'MessagePostedEvent',
            attachments: [
              {
                id: 'att_video',
                assetUrl: {
                  url: '/assets/files/att_video?access=fresh-original',
                  expiresAt: '2027-05-29T15:00:00Z'
                },
                thumbnailAssetUrl: null,
                videoProcessing: {
                  thumbnailAssetUrl: {
                    url: videoThumbnailUrl,
                    expiresAt: '2027-05-29T15:00:00Z'
                  },
                  variants: []
                }
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
  it('renders the linked message body as markdown in a scrollable preview', async () => {
    queryResults.push(
      bodyPreviewResult('# Release notes\n\n- **Breaking** change\n- More details')
    );

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"] h1')?.textContent).toBe(
        'Release notes'
      );
    });

    expect(container.querySelector('[data-testid="message-preview-card"] strong')?.textContent).toBe(
      'Breaking'
    );
    expect(container.querySelector('[data-testid="message-preview-card"] ul')).not.toBeNull();
    expect(container.querySelector('.max-h-52.overflow-y-auto')).not.toBeNull();
    expect(container.querySelector('.bg-gradient-to-b')).not.toBeNull();
    expect(container.querySelector('.bg-gradient-to-t')).not.toBeNull();
  });

  it('refreshes attachment thumbnail asset URLs after image load failure', async () => {
    queryResults.push(
      previewResult(transparentGif),
      refreshResult(`${transparentGif}#fresh-image`)
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
      expect(refreshed?.getAttribute('src')).toContain('#fresh-image');
    });
    const refreshCalls = queryMock.mock.calls.filter((call) => call[1]?.thumbnailWidth === 120);
    expect(refreshCalls.length).toBeGreaterThanOrEqual(1);
    for (const call of refreshCalls) {
      expect(call[1]).toMatchObject({
        roomId: 'room_1',
        eventId: 'event_1',
        thumbnailWidth: 120,
        thumbnailHeight: 120,
        thumbnailFit: FitMode.Cover
      });
    }
  });

  it('renders video attachment thumbnails for linked message previews', async () => {
    queryResults.push(videoPreviewResult(`${transparentGif}#old-video`));

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"]')).not.toBeNull();
    });

    const img = container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]');
    expect(img?.getAttribute('src')).toContain('#old-video');
    expect(container.querySelector('.uil--play')).not.toBeNull();
  });

  it('refreshes video attachment thumbnail asset URLs after image load failure', async () => {
    queryResults.push(
      videoPreviewResult(`${transparentGif}#old-video`),
      videoRefreshResult(`${transparentGif}#fresh-video`)
    );

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('[data-testid="message-preview-card"]')).not.toBeNull();
    });

    const img = container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]');
    expect(img).not.toBeNull();
    img?.dispatchEvent(new Event('error'));

    await vi.waitFor(() => {
      const refreshed = container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]');
      expect(refreshed?.getAttribute('src')).toContain('#fresh-video');
    });
  });

  it('falls back to a video tile when the refreshed video thumbnail also fails', async () => {
    queryResults.push(
      videoPreviewResult(`${transparentGif}#old-video`),
      videoRefreshResult(`${transparentGif}#fresh-video`)
    );

    const { container } = render(MessagePreviewCard, {
      props: { link: link(), showDismiss: false }
    });

    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="clip.mp4"]')).not.toBeNull();
    });

    container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]')?.dispatchEvent(new Event('error'));

    await vi.waitFor(() => {
      expect(container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]')?.src).toContain(
        '#fresh-video'
      );
    });

    container.querySelector<HTMLImageElement>('img[alt="clip.mp4"]')?.dispatchEvent(new Event('error'));

    await vi.waitFor(() => {
      expect(container.querySelector('img[alt="clip.mp4"]')).toBeNull();
    });
    expect(container.querySelector('.uil--play')).not.toBeNull();
    expect(container.textContent).toContain('Video');
  });
});
