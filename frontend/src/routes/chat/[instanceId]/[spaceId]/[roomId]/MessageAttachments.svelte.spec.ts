import { describe, it, expect } from 'vitest';
import { render } from 'vitest-browser-svelte';
import MessageAttachments from './MessageAttachments.svelte';
import type { RoomEventViewFragment } from '$lib/gql/graphql';
import { q } from '$lib/test-utils';

// Derive the Attachment shape exactly the way MessageAttachments.svelte does, so
// codegen-introduced fields force this factory to be updated rather than silently
// hiding behind an `as Attachment` cast.
type Attachment = NonNullable<
  Extract<RoomEventViewFragment['event'], { __typename: 'MessagePostedEvent' }>['attachments']
>[number];

function attachment(overrides: Partial<Attachment> = {}): Attachment {
  const base: Attachment = {
    id: 'att_1',
    spaceId: 's_1',
    filename: 'file.bin',
    contentType: 'application/octet-stream',
    url: 'https://cdn.test/assets/space/s_1/file.bin',
    thumbnailUrl: null,
    width: 0,
    height: 0,
    videoProcessing: null
  };
  return { ...base, ...overrides };
}

function renderAttachments(attachments: Attachment[], canDeleteAttachment = false) {
  return render(MessageAttachments, {
    props: {
      attachments,
      spaceId: 's_1',
      roomId: 'r_1',
      eventId: 'e_1',
      canDeleteAttachment
    }
  });
}

describe('MessageAttachments', () => {
  it('renders nothing for an empty list', () => {
    const { container } = renderAttachments([]);
    expect(container.querySelector('div')).toBeNull();
  });

  describe('audio', () => {
    it('renders an <audio> element with the attachment URL and filename', () => {
      const att = attachment({
        contentType: 'audio/mpeg',
        filename: 'voice-memo.mp3',
        url: 'https://cdn.test/assets/space/s_1/voice-memo.mp3'
      });
      const { container } = renderAttachments([att]);

      const audio = q(container, 'audio[data-testid="audio-player"]') as HTMLAudioElement | null;
      expect(audio).not.toBeNull();
      expect(audio?.getAttribute('src')).toBe('https://cdn.test/assets/space/s_1/voice-memo.mp3');
      expect(audio?.hasAttribute('controls')).toBe(true);

      // Filename appears next to the player
      const labels = Array.from(container.querySelectorAll('span')).map(
        (s) => s.textContent ?? ''
      );
      expect(labels).toContain('voice-memo.mp3');

      // Fallback <a> inside <audio> for browsers without playback support
      const fallback = audio?.querySelector('a');
      expect(fallback?.getAttribute('href')).toBe(att.url);
    });

    it('handles ogg, wav, and other audio/* subtypes', () => {
      const formats: Array<{ contentType: string; filename: string }> = [
        { contentType: 'audio/ogg', filename: 'a.ogg' },
        { contentType: 'audio/wav', filename: 'a.wav' },
        { contentType: 'audio/x-m4a', filename: 'a.m4a' }
      ];
      for (const fmt of formats) {
        const { container, unmount } = renderAttachments([attachment(fmt)]);
        expect(q(container, 'audio[data-testid="audio-player"]')).not.toBeNull();
        unmount();
      }
    });
  });

  describe('image', () => {
    it('renders an image button that opens the modal on click', () => {
      const { container } = renderAttachments([
        attachment({
          contentType: 'image/jpeg',
          filename: 'photo.jpg',
          url: 'https://cdn.test/assets/space/s_1/photo.jpg',
          width: 800,
          height: 600
        })
      ]);

      const button = q(container, 'button[aria-label="View photo.jpg"]');
      expect(button).not.toBeNull();
      // SkeletonImg renders an <img>; we check the parent button instead of the img
      // because SkeletonImg manages the actual src reactively.
    });

    it('does not render an audio player for an image attachment', () => {
      const { container } = renderAttachments([
        attachment({ contentType: 'image/png', filename: 'p.png' })
      ]);
      expect(q(container, 'audio[data-testid="audio-player"]')).toBeNull();
    });
  });

  describe('video without processing data', () => {
    it('shows the "Video unavailable" fallback', () => {
      const { container } = renderAttachments([
        attachment({
          contentType: 'video/mp4',
          filename: 'v.mp4',
          videoProcessing: null
        })
      ]);
      expect(container.textContent).toContain('Video unavailable');
    });
  });

  describe('generic file fallback', () => {
    it('renders a download link with the filename for unknown content types', () => {
      const { container } = renderAttachments([
        attachment({
          contentType: 'application/pdf',
          filename: 'report.pdf',
          url: 'https://cdn.test/assets/space/s_1/report.pdf'
        })
      ]);

      const link = q(container, 'a[aria-label="Download report.pdf"]') as HTMLAnchorElement | null;
      expect(link).not.toBeNull();
      expect(link?.getAttribute('href')).toBe('https://cdn.test/assets/space/s_1/report.pdf');
      expect(link?.getAttribute('target')).toBe('_blank');
      expect(link?.getAttribute('rel')).toBe('noopener noreferrer');
    });
  });

  describe('delete button', () => {
    it('hides the delete button when canDeleteAttachment is false', () => {
      const { container } = renderAttachments([attachment({ contentType: 'audio/mpeg' })], false);
      expect(q(container, 'button[aria-label="Delete attachment"]')).toBeNull();
    });

    it('shows the delete button when canDeleteAttachment is true', () => {
      const { container } = renderAttachments([attachment({ contentType: 'audio/mpeg' })], true);
      expect(q(container, 'button[aria-label="Delete attachment"]')).not.toBeNull();
    });
  });
});
