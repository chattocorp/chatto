import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { LinkPreviewInput } from '$lib/render/types';
import { extractURLs } from '$lib/linkPreview';
import { parseMessageLink } from '$lib/messageLinks';
import type { ComposerLinkPreview } from '@chatto/api-client/linkPreviews';

type LinkPreviewAPI = {
  fetchLinkPreview(url: string): Promise<ComposerLinkPreview | null>;
};

export class LinkPreviewState {
  detectedURLs = $state<string[]>([]);
  previews = new SvelteMap<string, ComposerLinkPreview | null>();
  dismissedURLs = new SvelteSet<string>();
  fetchingURLs = new SvelteSet<string>();
  #urlDetectionTimeout: ReturnType<typeof setTimeout> | undefined;

  constructor(private readonly getAPI: () => LinkPreviewAPI) {}

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

    const preview = await this.getAPI().fetchLinkPreview(url);

    this.fetchingURLs.delete(url);
    this.previews.set(url, preview);
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

  buildInput(): LinkPreviewInput | null {
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
