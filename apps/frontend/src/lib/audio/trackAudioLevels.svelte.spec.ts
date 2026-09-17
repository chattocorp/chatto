import { expect, it, vi } from 'vitest';
import { TrackAudioLevels } from './trackAudioLevels';
import { userEvent } from '@vitest/browser/context';

it('meters independent tracks and releases nodes without stopping capture', async () => {
  const context = new AudioContext();
  const start = document.createElement('button');
  start.textContent = 'Start audio meter test';
  start.onclick = () => {
    void context.resume();
  };
  document.body.append(start);
  await userEvent.click(start);
  const loud = context.createMediaStreamDestination();
  const quiet = context.createMediaStreamDestination();
  const oscillator = context.createOscillator();
  oscillator.connect(loud);
  oscillator.start();
  const loudTrack = loud.stream.getAudioTracks()[0];
  const quietTrack = quiet.stream.getAudioTracks()[0];
  const createSource = vi.spyOn(context, 'createMediaStreamSource');
  const meters = new TrackAudioLevels();
  try {
    meters.sync(
      context,
      new Map([
        ['loud', loudTrack],
        ['quiet', quietTrack]
      ])
    );
    await expect
      .poll(() => {
        meters.sample();
        return meters.get('loud');
      })
      .toBeGreaterThan(0.1);
    expect(meters.get('quiet')).toBe(0);
    expect(meters.get('absent')).toBe(0);
    meters.sync(
      context,
      new Map([
        ['loud', loudTrack],
        ['quiet', quietTrack]
      ])
    );
    expect(createSource).toHaveBeenCalledTimes(2);
    loudTrack.enabled = false;
    meters.sample();
    expect(meters.get('loud')).toBe(0);
    meters.sync(context, new Map([['loud', quietTrack]]));
    expect(meters.get('quiet')).toBe(0);
    expect(meters.get('loud')).toBe(0);
    expect(createSource).toHaveBeenCalledTimes(3);
    meters.clear();
    expect(quietTrack.readyState).toBe('live');
    expect(context.state).toBe('running');
  } finally {
    meters.clear();
    oscillator.stop();
    loudTrack.stop();
    quietTrack.stop();
    await context.close();
    start.remove();
    vi.restoreAllMocks();
  }
});
