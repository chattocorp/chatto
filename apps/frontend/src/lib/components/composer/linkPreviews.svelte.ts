import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { extractURLs } from '$lib/linkPreview';
import { parseEntityLink, parseMessageLink } from '$lib/chatLinks';
import type { ComposerLinkPreview } from '@chatto/client/api/linkPreviews';

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
      // Entity pills already show these resources. Skip them before selecting
      // the first URL so a later web link can still receive an attachment.
      const urls = extractURLs(message, Infinity)
        .filter((url) => !parseEntityLink(url))
        .slice(0, 1)
        .filter((url) => !this.dismissedURLs.has(url));
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
    try {
      const preview = await this.getAPI().fetchLinkPreview(url);
      this.previews.set(url, preview);
    } catch {
      // Metadata is optional. Keep the URL usable and avoid retries on each edit.
      this.previews.set(url, null);
    } finally {
      this.fetchingURLs.delete(url);
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

  buildToken(): string | null {
    const previewURL = this.activeURL;
    const activePreview = previewURL ? this.previews.get(previewURL) : null;

    if (!previewURL || !activePreview || this.dismissedURLs.has(previewURL)) {
      return null;
    }

    return activePreview.previewToken;
  }
}
