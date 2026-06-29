import { beforeEach, describe, expect, it, vi } from 'vitest';
import { configureApiClientHooks } from '@chatto/api-client/hooks';
import { Code, ConnectError } from '@connectrpc/connect';
import { LinkPreview, FetchLinkPreviewResponse } from '@chatto/api-types/api/v1/link_previews_pb';
import { createLinkPreviewAPI } from '@chatto/api-client/linkPreviews';

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
        })
      })
    );

    const api = createLinkPreviewAPI({
      serverId: 'remote',
      baseUrl: 'https://remote.example.test/api/connect',
      bearerToken: 'remote-token'
    });

    await expect(api.fetchLinkPreview('https://example.com/story')).resolves.toMatchObject({
      url: 'https://example.com/story',
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
