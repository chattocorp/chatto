import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { GetLinkPreviewRequest } from '$lib/pb/chatto/api/v1/chat_pb';
import { extractURLs } from '$lib/linkPreview';
import { parseMessageLink } from '$lib/messageLinks';
import { activeWireClient } from '$lib/wire';

export type ComposerLinkPreviewInput = {
  url: string;
  title?: string | null;
  description?: string | null;
  siteName?: string | null;
  imageAssetId?: string | null;
  embedType?: string | null;
  embedId?: string | null;
};

type PreviewData = ComposerLinkPreviewInput & {
  imageUrl?: string | null;
};

type PreviewFetcher = (url: string) => Promise<PreviewData | null>;

export class LinkPreviewState {
  detectedURLs = $state<string[]>([]);
  previews = new SvelteMap<string, PreviewData | null>();
  dismissedURLs = new SvelteSet<string>();
  fetchingURLs = new SvelteSet<string>();
  #urlDetectionTimeout: ReturnType<typeof setTimeout> | undefined;

  constructor(private readonly fetchPreviewData: PreviewFetcher = fetchWireLinkPreview) {}

  get activeURL(): string | undefined {
    return this.detectedURLs[0];
  }

  scheduleDetection(message: string, isEditing: boolean): () => void {
    clearTimeout(this.#urlDetectionTimeout);

    if (isEditing) {
      this.detectedURLs = [];
      return () => clearTimeout(this.#urlDetectionTimeout);
    }

    this.#urlDetectionTimeout = setTimeout(() => {
      const urls = extractURLs(message).filter((u) => !this.dismissedURLs.has(u));
      this.detectedURLs = urls;

      for (const url of urls) {
        if (parseMessageLink(url)) continue;
        if (!this.previews.has(url) && !this.fetchingURLs.has(url)) {
          void this.fetchPreview(url);
        }
      }
    }, 500);

    return () => clearTimeout(this.#urlDetectionTimeout);
  }

  async fetchPreview(url: string): Promise<void> {
    this.fetchingURLs.add(url);

    const result = await this.fetchPreviewData(url);

    this.fetchingURLs.delete(url);

    if (result) {
      this.previews.set(url, result);
    } else {
      this.previews.set(url, null);
    }
  }

  dismissPreview(url: string): void {
    this.dismissedURLs.add(url);
    this.detectedURLs = this.detectedURLs.filter((u) => u !== url);
  }

  clear(): void {
    this.detectedURLs = [];
    this.previews.clear();
    this.dismissedURLs.clear();
    this.fetchingURLs.clear();
  }

  buildInput(): ComposerLinkPreviewInput | null {
    const previewURL = this.activeURL;
    const activePreview = previewURL ? this.previews.get(previewURL) : null;

    if (!previewURL || !activePreview || this.dismissedURLs.has(previewURL)) {
      return null;
    }

    return {
      url: activePreview.url,
      title: activePreview.title,
      description: activePreview.description,
      siteName: activePreview.siteName,
      imageAssetId: activePreview.imageAssetId,
      embedType: activePreview.embedType,
      embedId: activePreview.embedId
    };
  }
}

async function fetchWireLinkPreview(url: string): Promise<PreviewData | null> {
  const client = activeWireClient();
  if (!client) return null;

  const response = await client.getLinkPreview(new GetLinkPreviewRequest({ url }));
  const preview = response.preview;
  if (!preview?.url) return null;

  return {
    url: preview.url,
    title: preview.title,
    description: preview.description,
    siteName: preview.siteName,
    imageUrl: preview.imageUrl ?? null,
    imageAssetId: preview.imageAssetId ?? null,
    embedType: preview.embedType,
    embedId: preview.embedId ?? null
  };
}
