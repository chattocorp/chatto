import { afterEach, expect, it, vi } from 'vitest';
import { userEvent } from 'vitest/browser';
import { LocalAudioTrack, Track } from 'livekit-client';
import { MicrophoneProcessor } from './microphoneProcessor';
import { normalizeMicrophoneEffects } from './microphoneEffects';

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

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

it.each(['gate', 'polish'])(
  'gates a real LiveKit track and falls back after %s failure',
  async (failedStage) => {
    const { context, oscillator, track } = await input();
    const sources = vi.spyOn(context, 'createMediaStreamSource');
    const local = new LocalAudioTrack(track, undefined, true, context);
    const processor = new MicrophoneProcessor(-20);
    try {
      const connect = vi.spyOn(AudioNode.prototype, 'connect');
      await local.setProcessor(processor);
      const worklets = [
        ...new Set(
          connect.mock.calls
            .map(([node]) => node)
            .filter((node) => node instanceof AudioWorkletNode)
        )
      ];
      const worklet = worklets[failedStage === 'polish' ? 0 : 1];
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
      processor.setEffects({ ...normalizeMicrophoneEffects(), equalizer: true, mid: -6 });
      await vi.waitFor(() => expect(peak()).toBeLessThan(0.009));
      processor.setEffects(normalizeMicrophoneEffects());
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
  }
);

it.each([
  'missing API',
  'module failure',
  'native node failure',
  'polish node failure'
])('keeps ordinary audio available with %s', async (failure) => {
  const { context, oscillator, track } = await input();
  if (failure === 'missing API')
    Object.defineProperty(context, 'audioWorklet', { value: undefined });
  else if (failure === 'module failure')
    vi.spyOn(context.audioWorklet, 'addModule').mockRejectedValue(new Error('Unavailable'));
  else if (failure === 'polish node failure') {
    const NativeWorklet = AudioWorkletNode;
    vi.stubGlobal(
      'AudioWorkletNode',
      class extends NativeWorklet {
        constructor(context: BaseAudioContext, name: string, options?: AudioWorkletNodeOptions) {
          if (name === 'chatto-voice-polish') throw new Error('Unavailable');
          super(context, name, options);
        }
      }
    );
  } else
    vi.spyOn(context, 'createDynamicsCompressor').mockImplementation(() => {
      throw new Error('Unavailable');
    });
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
});

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

it.each([
  { frequency: 100, amplitude: 0.5 },
  { frequency: 8000, amplitude: 0.3 }
])(
  'applies automatic polish live and after restart at $frequency Hz',
  async ({ frequency, amplitude }) => {
    const { context, oscillator, gain, track } = await input();
    const processor = new MicrophoneProcessor();
    oscillator.frequency.value = frequency;
    gain.gain.value = amplitude;
    const analyser = context.createAnalyser();
    const samples = new Float32Array(analyser.fftSize);
    const peak = () => {
      analyser.getFloatTimeDomainData(samples);
      return Math.max(...samples.map(Math.abs));
    };
    try {
      await processor.init({ track, audioContext: context, kind: Track.Kind.Audio });
      const monitor = () =>
        context
          .createMediaStreamSource(new MediaStream([processor.processedTrack!]))
          .connect(analyser);
      monitor();
      await vi.waitFor(() => expect(peak()).toBeGreaterThan(amplitude * 0.83));
      const baseline = peak();
      processor.setEffects({ ...normalizeMicrophoneEffects(), polish: 1 });
      await vi.waitFor(() => expect(peak()).toBeLessThan(baseline * 0.99));
      await processor.restart({ track, audioContext: context, kind: Track.Kind.Audio });
      monitor();
      await vi.waitFor(() => {
        expect(peak()).toBeGreaterThan(0.05);
        expect(peak()).toBeLessThan(baseline * 0.99);
      });
      processor.setEffects(normalizeMicrophoneEffects());
      await vi.waitFor(() => expect(peak()).toBeGreaterThan(amplitude * 0.83));
    } finally {
      await processor.destroy();
      oscillator.stop();
      track.stop();
      await context.close();
    }
  }
);

it('updates gate softness without changing its saved threshold or Off behavior', async () => {
  const { context, oscillator, gain, track } = await input();
  gain.gain.value = 0.07;
  const processor = new MicrophoneProcessor(-20);
  const analyser = context.createAnalyser();
  const samples = new Float32Array(analyser.fftSize);
  const peak = () => {
    analyser.getFloatTimeDomainData(samples);
    return Math.max(...samples.map(Math.abs));
  };
  try {
    await processor.init({ track, audioContext: context, kind: Track.Kind.Audio });
    context.createMediaStreamSource(new MediaStream([processor.processedTrack!])).connect(analyser);
    await vi.waitFor(() => expect(processor.level).toBeGreaterThan(0.01));
    expect(peak()).toBe(0);
    processor.setEffects({ ...normalizeMicrophoneEffects(), polish: 1 });
    await vi.waitFor(() => {
      expect(peak()).toBeGreaterThan(0.01);
      expect(peak()).toBeLessThan(0.06);
    });
    processor.setThreshold(-60);
    await vi.waitFor(() => expect(peak()).toBeGreaterThan(0.065));
    processor.setThreshold(-20);
    processor.setEffects(normalizeMicrophoneEffects());
    // Drop below the closing threshold first; the existing hysteresis holds
    // an already-open gate through near-threshold speech.
    gain.gain.value = 0.001;
    await vi.waitFor(() => expect(peak()).toBe(0));
    gain.gain.value = 0.07;
    await vi.waitFor(() => expect(processor.level).toBeGreaterThan(0.03));
    expect(peak()).toBe(0);
  } finally {
    await processor.destroy();
    oscillator.stop();
    track.stop();
    await context.close();
  }
});
