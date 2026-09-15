import { afterEach, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { LocalAudioTrack, Track } from 'livekit-client';
import { MicrophoneProcessor } from './microphoneProcessor';
import { defaultMicrophoneEffects } from './microphoneEffects';

afterEach(() => vi.restoreAllMocks());

async function input() {
  const context = new AudioContext();
  const oscillator = context.createOscillator();
  const gain = context.createGain();
  gain.gain.value = 0.01;
  const destination = context.createMediaStreamDestination();
  oscillator.connect(gain).connect(destination);
  oscillator.start();
  const button = document.createElement('button');
  document.body.append(button);
  button.onclick = () => {
    void context.resume();
  };
  await userEvent.click(button);
  button.remove();
  return { context, oscillator, gain, track: destination.stream.getAudioTracks()[0] };
}

it('gates a real LiveKit track, updates thresholds, restarts, and releases only owned tracks', async () => {
  const { context, oscillator, track } = await input();
  const sources = vi.spyOn(context, 'createMediaStreamSource');
  const local = new LocalAudioTrack(track, undefined, true, context);
  const processor = new MicrophoneProcessor(-20);
  try {
    const connect = vi.spyOn(AudioNode.prototype, 'connect');
    await local.setProcessor(processor);
    const worklet: unknown = connect.mock.calls.find(
      ([node]) => node instanceof AudioWorkletNode
    )?.[0];
    if (!(worklet instanceof AudioWorkletNode)) throw new Error('Expected worklet connection');
    expect(sources).toHaveBeenCalledOnce();
    expect(processor.active).toBe(true);
    expect(local.mediaStreamTrack).not.toBe(track);
    const analyser = context.createAnalyser();
    context.createMediaStreamSource(new MediaStream([local.mediaStreamTrack])).connect(analyser);
    const samples = new Float32Array(analyser.fftSize);
    const peak = () => {
      analyser.getFloatTimeDomainData(samples);
      return Math.max(...samples.map(Math.abs));
    };
    await vi.waitFor(() => expect(processor.level).toBeGreaterThan(0));
    expect(peak()).toBe(0);
    processor.setThreshold(-60);
    await vi.waitFor(() => expect(peak()).toBeGreaterThan(0.001));
    processor.setEffects({ ...defaultMicrophoneEffects, equalizer: true, mid: -6 });
    await vi.waitFor(() => expect(peak()).toBeLessThan(0.009));
    processor.setEffects({ ...defaultMicrophoneEffects });
    await vi.waitFor(() => expect(peak()).toBeGreaterThan(0.0095));
    await local.mute();
    await vi.waitFor(() => expect(peak()).toBe(0));
    await local.unmute();
    await vi.waitFor(() => expect(peak()).toBeGreaterThan(0.001));
    expect(local.isMuted).toBe(false);
    processor.setThreshold(-20);
    await vi.waitFor(() => expect(peak()).toBe(0));
    worklet.dispatchEvent(
      new ErrorEvent('processorerror', { message: 'Synthetic worklet failure' })
    );
    expect(processor.unavailable).toBe(true);
    await vi.waitFor(() => expect(processor.level).toBeGreaterThan(0));
    await vi.waitFor(() => expect(peak()).toBeGreaterThan(0.001));
    const old = processor.processedTrack!;
    await processor.restart({ track, audioContext: context, kind: Track.Kind.Audio });
    expect(old.readyState).toBe('ended');
    expect(processor.processedTrack).not.toBe(old);
    const output = processor.processedTrack!;
    await processor.destroy();
    expect(output.readyState).toBe('ended');
    expect(track.readyState).toBe('live');
    expect(context.state).toBe('running');
  } finally {
    await processor.destroy();
    oscillator.stop();
    track.stop();
    await context.close();
  }
});

it.each(['missing API', 'module failure'])(
  'keeps ordinary audio available with %s',
  async (failure) => {
    const { context, oscillator, track } = await input();
    if (failure === 'missing API')
      Object.defineProperty(context, 'audioWorklet', { value: undefined });
    else vi.spyOn(context.audioWorklet, 'addModule').mockRejectedValue(new Error('Unavailable'));
    const processor = new MicrophoneProcessor(-20);
    try {
      await processor.init({ track, audioContext: context, kind: Track.Kind.Audio });
      expect(processor.unavailable).toBe(true);
      await vi.waitFor(() => expect(processor.level).toBeGreaterThan(0));
      expect(processor.processedTrack).toBe(track);
      await processor.destroy();
      expect(track.readyState).toBe('live');
    } finally {
      oscillator.stop();
      track.stop();
      await context.close();
    }
  }
);

it('does not attach audio after cancellation while loading', async () => {
  const { context, oscillator, track } = await input();
  let resolve!: () => void;
  vi.spyOn(context.audioWorklet, 'addModule').mockImplementation(
    () =>
      new Promise<void>((done) => {
        resolve = done;
      })
  );
  const processor = new MicrophoneProcessor(-20);
  try {
    const loading = processor.init({ track, audioContext: context, kind: Track.Kind.Audio });
    await processor.destroy();
    resolve();
    await loading;
    expect(processor.active).toBe(false);
    expect(processor.processedTrack).toBeUndefined();
    expect(track.readyState).toBe('live');
  } finally {
    oscillator.stop();
    track.stop();
    await context.close();
  }
});

it('rejects SDK initialization queued after permanent disposal', async () => {
  const { context, oscillator, track } = await input();
  const processor = new MicrophoneProcessor(-20);
  try {
    processor.dispose();
    await processor.init({ track, audioContext: context, kind: Track.Kind.Audio });
    expect(processor.active).toBe(false);
    expect(processor.processedTrack).toBeUndefined();
  } finally {
    oscillator.stop();
    track.stop();
    await context.close();
  }
});
