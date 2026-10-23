import { describe, expect, it } from 'vitest';
import { attachmentUploadRejection } from './attachmentUpload.js';

describe('attachmentUploadRejection', () => {
  it.each(['image/gif', 'image/png', 'video/mp4', 'application/pdf', ''])(
    'uses the same inclusive limit for %s',
    (type) => {
      const limits = { videoProcessingEnabled: true, maxUploadSize: 100 };
      expect(attachmentUploadRejection({ type, size: 100 }, limits)).toBeNull();
      expect(attachmentUploadRejection({ type, size: 101 }, limits)).toBe('too_large');
    }
  );

  it('rejects disabled video before checking size but still allows GIFs', () => {
    const limits = { videoProcessingEnabled: false, maxUploadSize: 100 };
    expect(attachmentUploadRejection({ type: 'video/mp4', size: 101 }, limits)).toBe(
      'video_disabled'
    );
    expect(attachmentUploadRejection({ type: 'image/gif', size: 100 }, limits)).toBeNull();
  });
});
