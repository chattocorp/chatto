import { expect, it, vi } from 'vitest';
import { RemoteAudioTrack } from 'livekit-client';
import { participantVolumeGain } from './participantVolume';

it('maps volume to a bounded perceptual curve with exact mute and unity', () => {
  expect(participantVolumeGain(0)).toBe(0);
  expect(participantVolumeGain(100)).toBe(1);
  expect(participantVolumeGain(50)).toBeCloseTo(10 ** (-10 / 20), 10);
  expect(participantVolumeGain(200)).toBeCloseTo(10 ** (10 / 20), 10);
  expect(participantVolumeGain(-10)).toBe(0);
  expect(participantVolumeGain(500)).toBe(participantVolumeGain(200));
  expect(participantVolumeGain(NaN)).toBe(1);
  expect(participantVolumeGain(Infinity)).toBe(1);
  expect(participantVolumeGain(200, false)).toBe(1);
  expect(participantVolumeGain(50, false)).toBe(participantVolumeGain(50));
  for (let percent = 5; percent <= 200; percent += 5) {
    expect(participantVolumeGain(percent)).toBeGreaterThan(participantVolumeGain(percent - 5));
  }
});

it('renders the stronger boost without an amplitude cap at two', async () => {
  const context = new OfflineAudioContext(1, 128, 48000);
  const signal = context.createConstantSource();
  signal.offset.value = 0.1;
  const gain = context.createGain();
  gain.gain.value = participantVolumeGain(200);
  signal.connect(gain).connect(context.destination);
  signal.start();
  const rendered = await context.startRendering();
  expect(rendered.getChannelData(0)[127]).toBeCloseTo(0.31622777, 6);
});

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
    const boost = participantVolumeGain(200);
    track.setVolume(boost);
    expect(setGain).toHaveBeenLastCalledWith(boost, 0, 0.1);
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
