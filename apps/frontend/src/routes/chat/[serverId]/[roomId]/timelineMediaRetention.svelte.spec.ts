import { expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import audioUrl from '../../../../../e2e/fixtures/test-audio.mp3?url';
import { retainPlayingTimelineMedia } from './timelineMediaRetention';

it('finds media already playing and removes all tracking when detached', async () => {
  const root = document.createElement('div');
  const row = document.createElement('div');
  row.dataset.timelineKey = 'playing-message';
  row.dataset.attachmentMedia = '';
  const media = document.createElement('audio');
  media.src = audioUrl;
  media.muted = true;
  media.loop = true;
  row.append(media);
  root.append(row);
  document.body.append(root);
  const onChange = vi.fn();
  let cleanup: (() => void) | void = undefined;

  try {
    await userEvent.click(document.body);
    await media.play();
    cleanup = retainPlayingTimelineMedia(onChange)(root);
    expect(onChange).toHaveBeenLastCalledWith(new Set(['playing-message']));
    cleanup?.();
    cleanup = undefined;
    expect(onChange).toHaveBeenLastCalledWith(new Set());
    onChange.mockClear();

    media.dispatchEvent(new Event('play'));
    media.dispatchEvent(new Event('pause'));
    media.remove();
    await Promise.resolve();
    expect(onChange).not.toHaveBeenCalled();
  } finally {
    cleanup?.();
    media.pause();
    root.remove();
  }
});
