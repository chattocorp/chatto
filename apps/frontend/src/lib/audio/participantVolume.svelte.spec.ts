import { expect, it, vi } from 'vitest';
import { RemoteAudioTrack } from 'livekit-client';

it('uses a Web Audio gain above unity without unmuting the duplicate media element', async () => {
  const context = new AudioContext();
  const source = context.createMediaStreamDestination();
  const mediaTrack = source.stream.getAudioTracks()[0];
  const originalCreateGain = context.createGain.bind(context);
  const createGain = vi.spyOn(context, 'createGain').mockImplementation(() => {
    const gain = originalCreateGain();
    vi.spyOn(gain.gain, 'setTargetAtTime');
    return gain;
  });
  const track = new RemoteAudioTrack(mediaTrack, 'volume-test', {} as RTCRtpReceiver, context);
  const element = document.createElement('audio');
  // A silent source exercises the actual SDK graph without audible test output.
  try {
    track.attach(element);
    const gain = createGain.mock.results[0].value as GainNode;
    const setGain = gain.gain.setTargetAtTime;
    track.setVolume(2);
    expect(setGain).toHaveBeenLastCalledWith(2, 0, 0.1);
    expect(element.muted).toBe(true);
    expect(element.volume).toBeLessThanOrEqual(1);
    track.setVolume(0);
    expect(setGain).toHaveBeenLastCalledWith(0, 0, 0.1);
    track.setVolume(1.5);
    expect(setGain).toHaveBeenLastCalledWith(1.5, 0, 0.1);
    track.detach(element);
    track.attach(element);
    const replacement = createGain.mock.results.at(-1)?.value as GainNode;
    expect(replacement.gain.setTargetAtTime).toHaveBeenLastCalledWith(1.5, 0, 0.1);
    expect(element.muted).toBe(true);
  } finally {
    track.detach();
    mediaTrack.stop();
    source.disconnect();
    await context.close();
  }
});
