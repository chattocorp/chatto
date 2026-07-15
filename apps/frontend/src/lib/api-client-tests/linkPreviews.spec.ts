import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '$lib/api-client/hooks';
import { Code, ConnectError } from '@connectrpc/connect';
import {
  SocialPostAuthor,
  SocialPostExternalLink,
  SocialPostImage,
  SocialPostPreview,
  LinkPreview,
  FetchLinkPreviewResponse
} from '@chatto/api-types/api/v1/link_previews_pb';
import { Timestamp } from '@bufbuild/protobuf';
import { createLinkPreviewAPI } from '$lib/api-client/linkPreviews';

const mocks = vi.hoisted(() => ({
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  fetchLinkPreview: vi.fn(),
  handleAuthenticationRequired: vi.fn()
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

describe('createLinkPreviewAPI', () => {
  beforeEach(() => {
    mocks.createClient.mockReset();
    mocks.createConnectTransport.mockReset();
    mocks.fetchLinkPreview.mockReset();
    mocks.handleAuthenticationRequired.mockReset();

    configureApiClientHooks({ onAuthenticationRequired: mocks.handleAuthenticationRequired });
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' });
    mocks.createClient.mockReturnValue({
      fetchLinkPreview: mocks.fetchLinkPreview
    });
  });

  it('fetches a preview with bearer auth and maps optional fields', async () => {
    mocks.fetchLinkPreview.mockResolvedValue(
      new FetchLinkPreviewResponse({
        preview: new LinkPreview({
          url: 'https://example.com/story',
          title: 'Story',
          description: 'Description',
          imageUrl: '/assets/preview.webp',
          imageAssetId: 'asset_preview',
          siteName: 'Example',
          embedType: 'generic'
        }),
        previewToken: 'cht_LPpreviewtoken'
      })
    );

    const api = createLinkPreviewAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });

    await expect(api.fetchLinkPreview('https://example.com/story')).resolves.toMatchObject({
      url: 'https://example.com/story',
      previewToken: 'cht_LPpreviewtoken',
      title: 'Story',
      description: 'Description',
      imageUrl: '/assets/preview.webp',
      imageAssetId: 'asset_preview',
      siteName: 'Example',
      embedType: 'generic',
      embedId: null
    });
    expect(mocks.fetchLinkPreview).toHaveBeenCalledWith(
      { url: 'https://example.com/story' },
      { headers: { Authorization: 'Bearer remote-token' } }
    );
  });

  it('returns null when the server has no preview', async () => {
    mocks.fetchLinkPreview.mockResolvedValue(new FetchLinkPreviewResponse());

    const api = createLinkPreviewAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await expect(api.fetchLinkPreview('https://example.com/missing')).resolves.toBeNull();
  });

  it('maps a native Bluesky post snapshot', async () => {
    mocks.fetchLinkPreview.mockResolvedValue(
      new FetchLinkPreviewResponse({
        preview: new LinkPreview({
          url: 'https://bsky.app/profile/bsky.app/post/example',
          title: 'Bluesky (@bsky.app)',
          description: 'A post rendered by Chatto.',
          embedType: 'bluesky',
          embedId: 'at://did:plc:example/app.bsky.feed.post/example',
          socialPost: new SocialPostPreview({
            provider: 'bluesky',
            url: 'https://bsky.app/profile/bsky.app/post/example',
            author: new SocialPostAuthor({
              displayName: 'Bluesky',
              handle: 'bsky.app',
              avatarUrl: '/assets/avatar.webp'
            }),
            text: 'A post rendered by Chatto.',
            publishedAt: Timestamp.fromDate(new Date('2026-07-15T12:00:00Z')),
            externalLink: new SocialPostExternalLink({
              url: 'https://example.com/story',
              title: 'Story'
            }),
            images: [
              new SocialPostImage({
                url: '/assets/post.webp',
                alt: 'A blue sky',
                width: 1200,
                height: 800
              })
            ],
            quotedPost: new SocialPostPreview({
              provider: 'bluesky',
              url: 'https://bsky.app/profile/quoted.example/post/quoted',
              author: new SocialPostAuthor({
                displayName: 'Quoted Author',
                handle: 'quoted.example'
              }),
              text: 'Quoted words.',
              images: [
                new SocialPostImage({
                  url: '/assets/quoted.webp',
                  alt: 'Quoted attachment'
                })
              ]
            })
          })
        }),
        previewToken: 'cht_LPpreviewtoken'
      })
    );

    const api = createLinkPreviewAPI({
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: null
    });

    await expect(
      api.fetchLinkPreview('https://bsky.app/profile/bsky.app/post/example')
    ).resolves.toMatchObject({
      embedType: 'bluesky',
      socialPost: {
        provider: 'bluesky',
        url: 'https://bsky.app/profile/bsky.app/post/example',
        author: {
          displayName: 'Bluesky',
          handle: 'bsky.app',
          avatarUrl: '/assets/avatar.webp'
        },
        text: 'A post rendered by Chatto.',
        publishedAt: '2026-07-15T12:00:00.000Z',
        externalLink: { url: 'https://example.com/story', title: 'Story' },
        images: [{ url: '/assets/post.webp', alt: 'A blue sky', width: 1200, height: 800 }],
        quotedPost: {
          provider: 'bluesky',
          url: 'https://bsky.app/profile/quoted.example/post/quoted',
          author: { displayName: 'Quoted Author', handle: 'quoted.example' },
          text: 'Quoted words.',
          images: [{ url: '/assets/quoted.webp', alt: 'Quoted attachment' }]
        }
      }
    });
  });

  it('notifies the server registry when authentication expires', async () => {
    const err = new ConnectError('auth required', Code.Unauthenticated);
    mocks.fetchLinkPreview.mockRejectedValue(err);

    const api = createLinkPreviewAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'expired'
    });

    await expect(api.fetchLinkPreview('https://example.com/story')).rejects.toBe(err);
    expect(mocks.handleAuthenticationRequired).toHaveBeenCalledWith('remote');
  });
});
