import { userEvent } from 'vitest/browser';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { microphoneEffectsForAmount } from '$lib/audio/microphoneEffects';
import { MicrophoneProcessor } from '$lib/audio/microphoneProcessor';
import { CallPreferencesState } from './callPreferences.svelte';
import { CallDeviceTest, type OutputAudioContext } from './callDeviceTest.svelte';

describe('CallDeviceTest', () => {
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

  it.each(['context', 'element'])('monitors audio through %s output and releases capture on stop', async (route) => {
    if (route === 'element') {
      const NativeContext = AudioContext;
      vi.stubGlobal('AudioContext', class extends NativeContext {
        constructor() {
          super();
          Object.defineProperty(this, 'setSinkId', { value: undefined });
        }
      });
    }
    const monitor = vi.spyOn(MicrophoneProcessor.prototype, 'connectMonitor');
    const source = new AudioContext();
    const oscillator = source.createOscillator();
    const destination = source.createMediaStreamDestination();
    oscillator.connect(destination);
    oscillator.start();
    const button = document.createElement('button');
    button.textContent = 'Activate test audio';
    document.body.append(button);
    button.onclick = () => {
      void source.resume();
    };
    await userEvent.click(button);
    button.remove();
    const stream = destination.stream;
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    const play = vi.spyOn(HTMLMediaElement.prototype, 'play');
    const test = new CallDeviceTest();
    const preferences = new CallPreferencesState('test-presets');
    preferences.setVoiceBoosting(false);
    const applied = vi.spyOn(MicrophoneProcessor.prototype, 'setEffects');
    try {
      await test.start(
        '',
        '',
        // Keep the gate active so this exercises live graph updates, not raw bypass.
        () => -50,
        () => preferences.effects
      );
      expect(applied).toHaveBeenCalledWith(
        expect.objectContaining({ strength: 0, compressor: false })
      );
      preferences.setVoiceBoosting(true);
      await vi.waitFor(() =>
        expect(applied).toHaveBeenCalledWith(
          expect.objectContaining({ strength: 1, compressor: true })
        )
      );
      expect(test.active).toBe(true);
      expect(navigator.mediaDevices.getUserMedia).toHaveBeenCalledWith(
        expect.objectContaining({ audio: expect.objectContaining({ autoGainControl: false }) })
      );
      await vi.waitFor(() => expect(test.level).toBeGreaterThan(0));
      if (route === 'context') {
        expect(play).not.toHaveBeenCalled();
        expect(monitor).toHaveBeenCalledOnce();
        const sink = monitor.mock.calls[0][0];
        expect(sink).toBe(sink.context.destination);
        const analyser = sink.context.createAnalyser();
        (monitor.mock.contexts[0] as MicrophoneProcessor).connectMonitor(analyser);
        const samples = new Float32Array(analyser.fftSize);
        await vi.waitFor(() => {
          analyser.getFloatTimeDomainData(samples);
          expect(Math.max(...samples.map(Math.abs))).toBeGreaterThan(0.1);
        });
        test.stop();
        await vi.waitFor(() => expect(sink.context.state).toBe('closed'));
      } else {
        expect(play).toHaveBeenCalledOnce();
        const audio = play.mock.contexts[0] as HTMLAudioElement;
        expect((audio.srcObject as MediaStream).getAudioTracks()[0].readyState).toBe('live');
        expect(audio.paused).toBe(false);
        test.stop();
        expect(audio.paused).toBe(true);
        expect(audio.srcObject).toBeNull();
      }
      expect(stream.getTracks().every((track) => track.readyState === 'ended')).toBe(true);
      expect(test.active).toBe(false);
    } finally {
      test.stop();
      oscillator.stop();
      await source.close();
    }
  });

  it('does not start playback when stopped during speaker selection', async () => {
    const context = new AudioContext();
    const stream = context.createMediaStreamDestination().stream;
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    let resolve!: () => void;
    const sink = vi.spyOn(AudioContext.prototype as OutputAudioContext, 'setSinkId').mockImplementation(
      () =>
        new Promise<void>((done) => {
          resolve = done;
        })
    );
    const play = vi.spyOn(HTMLMediaElement.prototype, 'play');
    const test = new CallDeviceTest();
    try {
      const pending = test.start('', 'speaker', () => -60, () => microphoneEffectsForAmount(50));
      await vi.waitFor(() => expect(sink).toHaveBeenCalledWith('speaker'));
      test.stop();
      resolve();
      await pending;
      expect(play).not.toHaveBeenCalled();
      expect(stream.getTracks().every((track) => track.readyState === 'ended')).toBe(true);
      expect(test.active).toBe(false);
    } finally {
      test.stop();
      await context.close();
    }
  });

  it('releases capture instead of falling back when the chosen output fails', async () => {
    const context = new AudioContext();
    const stream = context.createMediaStreamDestination().stream;
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    const sink = vi.spyOn(AudioContext.prototype as OutputAudioContext, 'setSinkId').mockRejectedValue(new Error('Output denied'));
    const test = new CallDeviceTest();
    try {
      await test.start('', 'missing-speaker', () => -60, () => microphoneEffectsForAmount(50));
      expect(sink).toHaveBeenCalledExactlyOnceWith('missing-speaker');
      expect(test.error).toBe(true);
      expect(test.active).toBe(false);
      expect(stream.getTracks().every((track) => track.readyState === 'ended')).toBe(true);
    } finally {
      test.stop();
      await context.close();
    }
  });

  it('does not request capture until explicitly started', () => {
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    const test = new CallDeviceTest();
    expect(test.active).toBe(false);
    expect(capture).not.toHaveBeenCalled();
    test.stop();
  });

  it('stops a stream that arrives after cancellation', async () => {
    let resolve!: (stream: MediaStream) => void;
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    const stop = vi.fn();
    const test = new CallDeviceTest();
    const pending = test.start('mic');
    test.stop();
    resolve({ getTracks: () => [{ stop }] } as unknown as MediaStream);
    await pending;
    expect(stop).toHaveBeenCalledOnce();
    expect(test.active).toBe(false);
    expect(test.pending).toBe(false);
  });

  it('handles denial without leaving active capture state', async () => {
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockRejectedValue(
      new DOMException('Denied', 'NotAllowedError')
    );
    const test = new CallDeviceTest();
    await test.start('mic', '', () => -60, () => microphoneEffectsForAmount(50));
    expect(test.error).toBe(true);
    expect(test.pending).toBe(false);
    expect(test.active).toBe(false);
  });

  it('does not let a rejected older request replace a newer test state', async () => {
    let reject!: (reason: unknown) => void;
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    capture.mockImplementationOnce(
      () =>
        new Promise((_, fail) => {
          reject = fail;
        })
    );
    capture.mockImplementationOnce(() => new Promise(() => {}));
    const test = new CallDeviceTest();
    const older = test.start('old');
    void test.start('new');
    reject(new Error('old failure'));
    await older;
    expect(test.pending).toBe(true);
    expect(test.error).toBe(false);
    test.stop();
  });
});

it('serializes speaker changes without replacing or stopping microphone capture', async () => {
  const context = new AudioContext();
  const stream = context.createMediaStreamDestination().stream;
  const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
  const sink = vi.spyOn(AudioContext.prototype as OutputAudioContext, 'setSinkId').mockResolvedValue(undefined);
  const test = new CallDeviceTest();
  try {
    await test.start('mic', 'first', () => -60, () => microphoneEffectsForAmount(50));
    let finish!: () => void;
    sink.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve; }));
    const second = test.setSpeaker('second');
    const third = test.setSpeaker('third');
    await vi.waitFor(() => expect(sink).toHaveBeenLastCalledWith('second'));
    expect(capture).toHaveBeenCalledOnce();
    expect(stream.getAudioTracks()[0].readyState).toBe('live');
    finish();
    expect(await second).toBe(true);
    expect(await third).toBe(true);
    expect(sink.mock.calls.map(([id]) => id)).toEqual(['first', 'second', 'third']);
    expect(test.active).toBe(true);
    expect(capture).toHaveBeenCalledOnce();
  } finally {
    test.stop();
    await context.close();
    vi.restoreAllMocks();
  }
});

it('does not apply queued output changes after the test is stopped', async () => {
  const context = new AudioContext();
  const stream = context.createMediaStreamDestination().stream;
  vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
  const sink = vi.spyOn(AudioContext.prototype as OutputAudioContext, 'setSinkId').mockResolvedValue(undefined);
  const test = new CallDeviceTest();
  try {
    await test.start('mic', '', () => -60, () => microphoneEffectsForAmount(50));
    let finish!: () => void;
    sink.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve; }));
    const pending = test.setSpeaker('second');
    const queued = test.setSpeaker('third');
    await vi.waitFor(() => expect(sink).toHaveBeenLastCalledWith('second'));
    test.stop();
    finish();
    expect(await pending).toBe(false);
    expect(await queued).toBe(false);
    expect(sink.mock.calls.map(([id]) => id)).toEqual(['', 'second']);
    expect(test.active).toBe(false);
    expect(stream.getAudioTracks()[0].readyState).toBe('ended');
  } finally {
    test.stop();
    await context.close();
    vi.restoreAllMocks();
  }
});

it('plays the original capture stream at Normal with the gate Off, without initializing DSP', async () => {
  const context = new AudioContext();
  const stream = context.createMediaStreamDestination().stream;
  const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
  const init = vi.spyOn(MicrophoneProcessor.prototype, 'init');
  const play = vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue();
  const sink = vi.spyOn(HTMLMediaElement.prototype, 'setSinkId').mockResolvedValue();
  const test = new CallDeviceTest();
  try {
    await test.start('raw-mic', 'headphones');
    expect(init).not.toHaveBeenCalled();
    expect(play).toHaveBeenCalledOnce();
    expect((play.mock.contexts[0] as HTMLAudioElement).srcObject).toBe(stream);
    expect(sink).toHaveBeenCalledExactlyOnceWith('headphones');
    expect(capture).toHaveBeenCalledWith({audio: {
      deviceId: {exact: 'raw-mic'}, channelCount: {ideal: 1},
      echoCancellation: false, noiseSuppression: false, autoGainControl: false
    }, video: false});
    await test.setSpeaker('');
    expect(sink).toHaveBeenLastCalledWith('');
    expect(capture).toHaveBeenCalledOnce();
    expect(test.active).toBe(true);
  } finally {
    test.stop();
    await context.close();
    vi.restoreAllMocks();
  }
});

it('crosses the raw/processed boundary and keeps the selected speaker while releasing old capture', async () => {
  const context = new AudioContext();
  const source = context.createMediaStreamDestination().stream;
  const streams: MediaStream[] = [];
  vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(async () => {
    const stream = source.clone();
    streams.push(stream);
    return stream;
  });
  const init = vi.spyOn(MicrophoneProcessor.prototype, 'init');
  const play = vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue();
  const elementSink = vi.spyOn(HTMLMediaElement.prototype, 'setSinkId').mockResolvedValue();
  const contextSink = vi.spyOn(AudioContext.prototype as OutputAudioContext, 'setSinkId').mockResolvedValue(undefined);
  const test = new CallDeviceTest();
  let amount = 0;
  try {
    await test.start('mic', 'first', () => -60, () => microphoneEffectsForAmount(amount));
    await test.setSpeaker('second');
    amount = 50;
    await vi.waitFor(() => {
      expect(init).toHaveBeenCalledOnce();
      expect(test.active).toBe(true);
    });
    expect(contextSink).toHaveBeenLastCalledWith('second');
    expect(streams[0].getAudioTracks()[0].readyState).toBe('ended');
    amount = 0;
    await vi.waitFor(() => expect(play).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(test.active).toBe(true));
    expect(elementSink).toHaveBeenLastCalledWith('second');
    expect(streams).toHaveLength(3);
    expect(streams[1].getAudioTracks()[0].readyState).toBe('ended');
    expect((play.mock.contexts[1] as HTMLAudioElement).srcObject).toBe(streams[2]);
    test.stop();
    expect(streams.every(s => s.getAudioTracks()[0].readyState === 'ended')).toBe(true);
  } finally {
    test.stop();
    await context.close();
    vi.restoreAllMocks();
  }
});


it('does not report an output switch as successful by changing only the raw meter context', async () => {
  const context = new AudioContext();
  const stream = context.createMediaStreamDestination().stream;
  vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
  const play = vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue();
  const sink = vi.spyOn(AudioContext.prototype as OutputAudioContext, 'setSinkId').mockResolvedValue();
  const test = new CallDeviceTest();
  try {
    await test.start('mic');
    Object.defineProperty(play.mock.contexts[0], 'setSinkId', { value: undefined });
    expect(await test.setSpeaker('headphones')).toBe(false);
    expect(sink).not.toHaveBeenCalled();
    expect(test.error).toBe(true);
    expect(stream.getAudioTracks()[0].readyState).toBe('ended');
  } finally {
    test.stop();
    await context.close();
    vi.restoreAllMocks();
  }
});
