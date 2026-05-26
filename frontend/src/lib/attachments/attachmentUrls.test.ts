import { describe, expect, it, vi } from 'vitest';
import type { Client } from '@urql/svelte';
import { refreshAttachmentUrlsForMessage } from './attachmentUrls';

function clientWith(result: unknown): Client {
  return {
    query: vi.fn(() => ({
      toPromise: () => Promise.resolve(result)
    }))
  } as unknown as Client;
}

describe('refreshAttachmentUrlsForMessage', () => {
  it('extracts fresh URLs from a message event response', async () => {
    const client = clientWith({
      data: {
        room: {
          event: {
            event: {
              attachments: [
                { id: 'att_1', url: 'https://cdn.example.com/fresh-1.jpg' },
                { id: 'att_2', url: 'https://cdn.example.com/fresh-2.jpg' }
              ]
            }
          }
        }
      }
    });

    const urls = await refreshAttachmentUrlsForMessage(client, 'room_1', 'event_1');

    expect(urls.get('att_1')).toBe('https://cdn.example.com/fresh-1.jpg');
    expect(urls.get('att_2')).toBe('https://cdn.example.com/fresh-2.jpg');
  });

  it('returns an empty map when the refresh query fails', async () => {
    const client = clientWith({
      error: new Error('network failed')
    });

    const urls = await refreshAttachmentUrlsForMessage(client, 'room_1', 'event_1');

    expect(urls.size).toBe(0);
  });
});
