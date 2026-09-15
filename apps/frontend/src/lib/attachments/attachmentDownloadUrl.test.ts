import { describe, expect, it } from 'vitest';
import { attachmentDownloadUrl } from './attachmentDownloadUrl';

describe('attachment download URLs', () => {
  it('preserves a relative asset path, access ticket, and fragment', () => {
    expect(attachmentDownloadUrl('/assets/files/a?access=a%2Bb#file')).toBe(
      '/assets/files/a?access=a%2Bb&download=1#file'
    );
  });

  it('preserves a remote server and replaces an existing download mode', () => {
    expect(attachmentDownloadUrl('https://remote.example/assets/files/a?download=0&access=t')).toBe(
      'https://remote.example/assets/files/a?download=1&access=t'
    );
  });

  it('does not invent a URL for an unavailable asset', () => {
    expect(attachmentDownloadUrl(null)).toBeNull();
  });
});
