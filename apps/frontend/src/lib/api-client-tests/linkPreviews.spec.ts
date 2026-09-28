import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import {
  SocialPostAuthorSchema,
  SocialPostExternalLinkSchema,
  SocialPostImageSchema,
  SocialPostPreviewSchema,
  LinkPreviewSchema,
  FetchLinkPreviewResponseSchema
} from '@chatto/api-types/api/v1/link_previews_pb';
import { createLinkPreviewAPI } from '$lib/api-client/linkPreviews';
import { MessageService } from '@chatto/api-types/api/v1/messages_pb';
import { fakeServer, mockService, receivedRequest } from '$lib/test-utils';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { create } from '@bufbuild/protobuf';

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
      create(FetchLinkPreviewResponseSchema, {
        preview: create(LinkPreviewSchema, {
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
    mocks.fetchLinkPreview.mockReturnValue(create(FetchLinkPreviewResponseSchema));

    const api = linkPreviewAPI();

    await expect(api.fetchLinkPreview('https://example.com/missing')).resolves.toBeNull();
  });

  it('maps a native Bluesky post snapshot', async () => {
    mocks.fetchLinkPreview.mockReturnValue(
      create(FetchLinkPreviewResponseSchema, {
        preview: create(LinkPreviewSchema, {
          url: 'https://bsky.app/profile/bsky.app/post/example',
          title: 'Bluesky (@bsky.app)',
          description: 'A post rendered by Chatto.',
          embedType: 'bluesky',
          embedId: 'at://did:plc:example/app.bsky.feed.post/example',
          socialPost: create(SocialPostPreviewSchema, {
            provider: 'bluesky',
            url: 'https://bsky.app/profile/bsky.app/post/example',
            author: create(SocialPostAuthorSchema, {
              displayName: 'Bluesky',
              handle: 'bsky.app',
              avatarUrl: '/assets/avatar.webp'
            }),
            text: 'A post rendered by Chatto.',
            publishedAt: timestampFromDate(new Date('2026-07-15T12:00:00Z')),
            externalLink: create(SocialPostExternalLinkSchema, {
              url: 'https://example.com/story',
              title: 'Story'
            }),
            images: [
              create(SocialPostImageSchema, {
                url: '/assets/post.webp',
                alt: 'A blue sky',
                width: 1200,
                height: 800
              })
            ],
            quotedPost: create(SocialPostPreviewSchema, {
              provider: 'bluesky',
              url: 'https://bsky.app/profile/quoted.example/post/quoted',
              author: create(SocialPostAuthorSchema, {
                displayName: 'Quoted Author',
                handle: 'quoted.example'
              }),
              text: 'Quoted words.',
              images: [
                create(SocialPostImageSchema, {
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
