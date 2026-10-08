import { describe, expect, it, vi } from 'vitest';
import { retainPlayback } from './playbackRetention';

function attach() {
  const wrapper = document.createElement('div');
  const video = document.createElement('video');
  wrapper.append(video);
  const onChange = vi.fn();
  const cleanup = retainPlayback(onChange)(wrapper) as () => void;
  // Media events do not bubble; the wrapper sees them in the capture phase.
  const fire = (type: string) => video.dispatchEvent(new Event(type));
  return { onChange, cleanup, fire };
}

describe('retainPlayback', () => {
  it.each(['pause', 'ended', 'emptied', 'error'])('releases playing media on %s', (type) => {
    const { onChange, fire } = attach();
    fire('play');
    fire('play');
    fire(type);
    expect(onChange.mock.calls).toEqual([[true], [false]]);
  });

  it.each(['pause', 'ended'])('keeps picture-in-picture active after %s', (type) => {
    const { onChange, fire } = attach();
    fire('play');
    fire('enterpictureinpicture');
    fire(type);
    expect(onChange.mock.calls).toEqual([[true]]);
    fire('leavepictureinpicture');
    expect(onChange.mock.calls).toEqual([[true], [false]]);
  });

  it('keeps media active when picture-in-picture ends during playback', () => {
    const { onChange, fire } = attach();
    fire('enterpictureinpicture');
    fire('play');
    fire('leavepictureinpicture');
    expect(onChange.mock.calls).toEqual([[true]]);
  });

  it('releases active media on removal and ignores later events', () => {
    const { onChange, cleanup, fire } = attach();
    fire('play');
    cleanup();
    fire('pause');
    fire('play');
    expect(onChange.mock.calls).toEqual([[true], [false]]);
  });

  it('reports nothing on removal of inactive media', () => {
    const { onChange, cleanup } = attach();
    cleanup();
    expect(onChange).not.toHaveBeenCalled();
  });
});
