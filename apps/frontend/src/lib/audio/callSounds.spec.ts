import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { playCallSound } from './callSounds';

class FakeAudio {
  static instances: FakeAudio[] = [];
  onended: (() => void) | null = null;
  onerror: (() => void) | null = null;
  play = vi.fn().mockResolvedValue(undefined);
  pause = vi.fn();
  removeAttribute = vi.fn();
  load = vi.fn();
  constructor(readonly src: string) {
    FakeAudio.instances.push(this);
  }
}

describe('callSounds', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    FakeAudio.instances = [];
    vi.stubGlobal('Audio', FakeAudio);
    vi.stubGlobal(
      'AudioContext',
      vi.fn(() => {
        throw new Error('No custom audio graph');
      })
    );
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it.each(['join', 'leave'] as const)(
    'plays and releases the local %s cue without Web Audio',
    async (kind) => {
      const playing = playCallSound(kind);
      const audio = FakeAudio.instances[0];
      expect(audio.src).toContain(`call-${kind}.wav`);
      expect(audio.play).toHaveBeenCalledOnce();
      audio.onended?.();
      await playing;
      expect(audio.pause).toHaveBeenCalledOnce();
      expect(audio.removeAttribute).toHaveBeenCalledWith('src');
      expect(AudioContext).not.toHaveBeenCalled();
      expect(vi.getTimerCount()).toBe(0);
    }
  );

  it('bounds cleanup when playback does not finish', async () => {
    const playing = playCallSound('join');
    await vi.runAllTimersAsync();
    await playing;
    expect(FakeAudio.instances[0].pause).toHaveBeenCalledOnce();
  });

  it('ignores unavailable audio', async () => {
    vi.stubGlobal('Audio', undefined);
    await expect(playCallSound('join')).resolves.toBeUndefined();
  });
});
