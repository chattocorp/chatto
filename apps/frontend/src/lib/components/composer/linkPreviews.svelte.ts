import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { LinkPreviewInput } from '$lib/render/types';
import { extractURLs } from '$lib/linkPreview';
import { parseMessageLink } from '$lib/messageLinks';
import type {
  ComposerImportedAttachment,
  ComposerLinkPreview,
  ComposerLinkResult
} from '$lib/api-client/linkPreviews';

type LinkPreviewAPI = {
  fetchLinkPreview(url: string, roomId?: string): Promise<ComposerLinkResult | null>;
};

export class LinkPreviewState {
  detectedURLs = $state<string[]>([]);
  previews = new SvelteMap<string, ComposerLinkPreview | null>();
  importedAttachments = new SvelteMap<string, ComposerImportedAttachment>();
  dismissedURLs = new SvelteSet<string>();
  fetchingURLs = new SvelteSet<string>();
  #urlDetectionTimeout: ReturnType<typeof setTimeout> | undefined;
  #attachmentRoomId: string | undefined;
  #generation = 0;

  constructor(private readonly getAPI: () => LinkPreviewAPI) {}

  get activeURL(): string | undefined {
    return this.detectedURLs[0];
  }

  get activeImportedAttachment(): ComposerImportedAttachment | null {
    const url = this.activeURL;
    return url ? (this.importedAttachments.get(url) ?? null) : null;
  }

  scheduleDetection(message: string, isEditing: boolean, roomId?: string): () => void {
    clearTimeout(this.#urlDetectionTimeout);

    if (roomId !== this.#attachmentRoomId) {
      this.clear();
      this.#attachmentRoomId = roomId;
    }

    if (isEditing) {
      this.detectedURLs = [];
      return () => clearTimeout(this.#urlDetectionTimeout);
    }

    this.#urlDetectionTimeout = setTimeout(() => {
      const url = extractURLs(message)[0];
      this.detectedURLs = url && !this.dismissedURLs.has(url) ? [url] : [];
      if (
        url &&
        !this.dismissedURLs.has(url) &&
        !parseMessageLink(url) &&
        !this.previews.has(url) &&
        !this.importedAttachments.has(url) &&
        !this.fetchingURLs.has(url)
      ) {
        void this.fetchPreview(url, roomId);
      }
    }, 500);

    return () => clearTimeout(this.#urlDetectionTimeout);
  }

  async fetchPreview(url: string, roomId?: string): Promise<void> {
    const generation = this.#generation;
    this.fetchingURLs.add(url);
    try {
      const result = await this.getAPI().fetchLinkPreview(url, roomId);
      if (generation !== this.#generation) return;
      if (result?.kind === 'attachment') {
        this.importedAttachments.set(url, result.attachment);
        this.previews.set(url, null);
        return;
      }
      this.previews.set(url, result?.preview ?? null);
    } catch {
      if (generation === this.#generation) this.previews.set(url, null);
    } finally {
      if (generation === this.#generation) this.fetchingURLs.delete(url);
    }
  }

  dismissPreview(url: string): void {
    this.dismissedURLs.add(url);
    this.detectedURLs = this.detectedURLs.filter((u) => u !== url);
  }

  clear(): void {
    this.#generation += 1;
    this.detectedURLs = [];
    this.previews.clear();
    this.importedAttachments.clear();
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
      previewToken: activePreview.previewToken
    };
  }

  buildAttachmentAssetIds(): string[] {
    const attachment = this.activeImportedAttachment;
    return attachment ? [attachment.assetId] : [];
  }

  restoreImportedAttachment(url: string, attachment: ComposerImportedAttachment): void {
    this.detectedURLs = [url];
    this.importedAttachments.set(url, attachment);
    this.previews.set(url, null);
  }
}
