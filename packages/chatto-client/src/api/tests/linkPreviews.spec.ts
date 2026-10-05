import { beforeEach, describe, expect, it, vi } from 'vitest';
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
import { createLinkPreviewAPI } from '../linkPreviews.js';
import { MessageService } from '@chatto/api-types/api/v1/messages_connect';
import { fakeServer, mockService, receivedRequest } from '../../testing/fakeServer.js';

const mocks = mockService(MessageService);

function linkPreviewAPI() {
  return createLinkPreviewAPI(fakeServer((router) => router.service(MessageService, mocks)));
}

describe('createLinkPreviewAPI', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it('fetches a preview and maps optional fields', async () => {
    mocks.fetchLinkPreview.mockReturnValue(
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

    const api = linkPreviewAPI();

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
    expect(receivedRequest(mocks.fetchLinkPreview)).toMatchObject({
      url: 'https://example.com/story'
    });
  });

  it('returns null when the server has no preview', async () => {
    mocks.fetchLinkPreview.mockReturnValue(new FetchLinkPreviewResponse());

    const api = linkPreviewAPI();

    await expect(api.fetchLinkPreview('https://example.com/missing')).resolves.toBeNull();
  });

  it('maps a native Bluesky post snapshot', async () => {
    mocks.fetchLinkPreview.mockReturnValue(
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

    const api = linkPreviewAPI();

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

  it('propagates Connect errors', async () => {
    mocks.fetchLinkPreview.mockImplementation(() => {
      throw new ConnectError('auth required', Code.Unauthenticated);
    });

    await expect(
      linkPreviewAPI().fetchLinkPreview('https://example.com/story')
    ).rejects.toMatchObject({ code: Code.Unauthenticated, rawMessage: 'auth required' });
  });
});
