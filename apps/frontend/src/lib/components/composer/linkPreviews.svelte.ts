import type { Client } from '@urql/svelte';
import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { graphql } from '$lib/gql';
import { type LinkPreviewForComposerQuery, type LinkPreviewInput } from '$lib/gql/graphql';
import { useFragment } from '$lib/gql/fragment-masking';
import { extractURLs } from '$lib/linkPreview';
import { parseMessageLink } from '$lib/messageLinks';
import { LinkPreviewFragment } from '$lib/components/LinkPreviewCard.svelte';

const LinkPreviewForComposerDocument = graphql(`
  query LinkPreviewForComposer($url: String!) {
    linkPreview(url: $url) {
      ...LinkPreviewView
      imageAssetId
    }
  }
`);

type PreviewData = NonNullable<LinkPreviewForComposerQuery['linkPreview']>;

export class LinkPreviewState {
  detectedURLs = $state<string[]>([]);
  previews = new SvelteMap<string, PreviewData | null>();
  dismissedURLs = new SvelteSet<string>();
  fetchingURLs = new SvelteSet<string>();
  #urlDetectionTimeout: ReturnType<typeof setTimeout> | undefined;

  constructor(private readonly getClient: () => Client) {}

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

    const result = await this.getClient().query(LinkPreviewForComposerDocument, { url });

    this.fetchingURLs.delete(url);

    if (result.data?.linkPreview) {
      this.previews.set(url, result.data.linkPreview);
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

  buildInput(): LinkPreviewInput | null {
    const previewURL = this.activeURL;
    const activePreview = previewURL ? this.previews.get(previewURL) : null;
    const previewFields = activePreview ? useFragment(LinkPreviewFragment, activePreview) : null;

    if (!previewURL || !activePreview || !previewFields || this.dismissedURLs.has(previewURL)) {
      return null;
    }

    return {
      url: previewFields.url,
      title: previewFields.title,
      description: previewFields.description,
      siteName: previewFields.siteName,
      imageAssetId: activePreview.imageAssetId,
      embedType: previewFields.embedType,
      embedId: previewFields.embedId
    };
  }
}
