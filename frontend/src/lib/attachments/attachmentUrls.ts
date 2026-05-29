import type { Client } from '@urql/svelte';
import { graphql } from '$lib/gql';
import { RefreshMessageAttachmentUrlsDocument } from '$lib/gql/graphql';

export type ExpiringAssetUrl = {
  url: string;
  expiresAt: string;
};

export type RefreshedAttachmentUrls = {
  assetUrl: ExpiringAssetUrl;
  thumbnailAssetUrl: ExpiringAssetUrl | null;
  videoThumbnailAssetUrl: ExpiringAssetUrl | null;
  variantAssetUrls: Map<string, ExpiringAssetUrl>;
};

// Re-fetch a message event's attachment URLs just before the user actually
// needs them. Asset URL fields re-sign on every resolve; the staleness lives
// in already-rendered query/subscription data, not the server.
//
// This source query is intentionally kept next to the helper for codegen.
// Runtime uses the generated document below so TypeScript doesn't depend on
// the exact whitespace of the typed graphql() overload.
void graphql(`
  query RefreshMessageAttachmentUrls($roomId: ID!, $eventId: ID!) {
    room(roomId: $roomId) {
      event(eventId: $eventId) {
        event {
          __typename
          ... on MessagePostedEvent {
            attachments {
              id
              assetUrl {
                url
                expiresAt
              }
              thumbnailAssetUrl(width: 960, height: 800, fit: CONTAIN) {
                url
                expiresAt
              }
              videoProcessing {
                thumbnailAssetUrl {
                  url
                  expiresAt
                }
                variants {
                  quality
                  assetUrl {
                    url
                    expiresAt
                  }
                }
              }
            }
          }
        }
      }
    }
  }
`);

export async function refreshAttachmentUrlsForMessage(
  client: Client,
  roomId: string,
  eventId: string
): Promise<Map<string, RefreshedAttachmentUrls>> {
  const fresh = new Map<string, RefreshedAttachmentUrls>();
  const result = await client
    .query(RefreshMessageAttachmentUrlsDocument, { roomId, eventId })
    .toPromise();
  if (result.error) {
    console.warn('Failed to refresh attachment URLs', result.error);
    return fresh;
  }
  const inner = result.data?.room?.event?.event;
  if (inner && 'attachments' in inner) {
    for (const att of inner.attachments) {
      fresh.set(att.id, {
        assetUrl: att.assetUrl,
        thumbnailAssetUrl: att.thumbnailAssetUrl ?? null,
        videoThumbnailAssetUrl: att.videoProcessing?.thumbnailAssetUrl ?? null,
        variantAssetUrls: new Map(
          att.videoProcessing?.variants.map((variant) => [variant.quality, variant.assetUrl]) ?? []
        )
      });
    }
  }
  return fresh;
}
