import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { microphoneEffectsForAmount } from '$lib/audio/microphoneEffects';
import { MicrophoneProcessor } from '$lib/audio/microphoneProcessor';
import { CallDeviceTest } from './callDeviceTest.svelte';

class FakeRecorder {
  static instances: FakeRecorder[] = [];
  static output = new Blob(['sample'], { type: 'audio/webm' });
  static deferStop = false;
  state: RecordingState = 'inactive';
  ondataavailable: ((event: BlobEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onstop: ((event: Event) => void) | null = null;

  constructor(readonly stream: MediaStream) {
    FakeRecorder.instances.push(this);
  }

  start(): void {
    this.state = 'recording';
  }

  stop(): void {
    if (this.state !== 'recording') throw new Error('Not recording');
    this.state = 'inactive';
    if (!FakeRecorder.deferStop) queueMicrotask(() => this.flush());
  }

  flush(): void {
    this.ondataavailable?.({ data: FakeRecorder.output } as BlobEvent);
    this.onstop?.(new Event('stop'));
  }
}

const contexts: AudioContext[] = [];
const tests: CallDeviceTest[] = [];

function makeStream(): MediaStream {
  const context = new AudioContext();
  contexts.push(context);
  return context.createMediaStreamDestination().stream;
}

function makeTest(): CallDeviceTest {
  const test = new CallDeviceTest();
  tests.push(test);
  return test;
}

describe('CallDeviceTest', () => {
  beforeEach(() => {
    FakeRecorder.instances = [];
    FakeRecorder.output = new Blob(['sample'], { type: 'audio/webm' });
    FakeRecorder.deferStop = false;
    vi.stubGlobal('MediaRecorder', FakeRecorder);
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:microphone-test');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue();
    vi.spyOn(AudioContext.prototype, 'resume').mockResolvedValue();
  });

  afterEach(async () => {
    tests.splice(0).forEach((test) => test.cancel());
    await Promise.all(contexts.splice(0).map((context) => context.close()));
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('records raw capture without live playback and releases it before replay', async () => {
    const stream = makeStream();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    const play = vi.mocked(HTMLMediaElement.prototype.play);
    play.mockImplementation(async function (this: HTMLMediaElement) {
      expect(stream.getTracks().every((track) => track.readyState === 'ended')).toBe(true);
    });
    const processor = vi.spyOn(MicrophoneProcessor.prototype, 'init');
    const test = makeTest();

    await test.start('microphone');
    expect(test.active).toBe(true);
    expect(FakeRecorder.instances[0].stream).toBe(stream);
    expect(processor).not.toHaveBeenCalled();
    expect(play).not.toHaveBeenCalled();
    expect(navigator.mediaDevices.getUserMedia).toHaveBeenCalledWith({
      audio: {
        deviceId: { exact: 'microphone' },
        channelCount: { ideal: 1 },
        echoCancellation: false,
        noiseSuppression: false,
        autoGainControl: false
      },
      video: false
    });

    test.stop();
    expect(test.active).toBe(false);
    expect(stream.getTracks().every((track) => track.readyState === 'ended')).toBe(true);
    await vi.waitFor(() => expect(play).toHaveBeenCalledOnce());
    expect(test.hasRecording).toBe(true);
    expect(test.playing).toBe(true);
  });

  it('records the processed track without connecting it to live output', async () => {
    const stream = makeStream();
    const processed = makeStream();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    vi.spyOn(MicrophoneProcessor.prototype, 'init').mockImplementation(async function (
      this: MicrophoneProcessor
    ) {
      this.processedTrack = processed.getAudioTracks()[0];
    });
    const monitor = vi.spyOn(MicrophoneProcessor.prototype, 'connectMonitor');
    const play = vi.mocked(HTMLMediaElement.prototype.play);
    const test = makeTest();

    await test.start(
      '',
      '',
      () => -50,
      () => microphoneEffectsForAmount(50)
    );
    expect(FakeRecorder.instances[0].stream.getAudioTracks()[0]).toBe(
      processed.getAudioTracks()[0]
    );
    expect(monitor).not.toHaveBeenCalled();
    expect(play).not.toHaveBeenCalled();
    test.stop();
    await vi.waitFor(() => expect(play).toHaveBeenCalledOnce());
    expect(stream.getAudioTracks()[0].readyState).toBe('ended');
  });

  it('stops and replays after ten seconds', async () => {
    const stream = makeStream();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    let timeout: (() => void) | undefined;
    const schedule = vi.spyOn(window, 'setTimeout').mockImplementation((handler, delay) => {
      if (delay === 10_000) timeout = handler as () => void;
      return 1 as unknown as ReturnType<typeof setTimeout>;
    });
    const test = makeTest();
    await test.start('');
    expect(schedule).toHaveBeenCalledWith(expect.any(Function), 10_000);
    timeout!();
    await vi.waitFor(() => expect(test.playing).toBe(true));
    expect(stream.getAudioTracks()[0].readyState).toBe('ended');
  });

  it('keeps playback silent while the recorder finalizes', async () => {
    FakeRecorder.deferStop = true;
    const stream = makeStream();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    const test = makeTest();
    await test.start('');
    test.stop();
    expect(test.finalizing).toBe(true);
    expect(stream.getAudioTracks()[0].readyState).toBe('ended');
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();
    FakeRecorder.instances[0].flush();
    await vi.waitFor(() => expect(test.playing).toBe(true));
    expect(test.finalizing).toBe(false);
  });

  it('keeps the clip for a manual replay when automatic playback is blocked', async () => {
    const stream = makeStream();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    const play = vi.mocked(HTMLMediaElement.prototype.play);
    play.mockRejectedValueOnce(new DOMException('Blocked', 'NotAllowedError'));
    const test = makeTest();
    await test.start('');
    test.stop();
    await vi.waitFor(() => expect(test.playbackBlocked).toBe(true));
    expect(test.hasRecording).toBe(true);
    expect(test.playing).toBe(false);
    await test.playAgain();
    expect(test.playing).toBe(true);
    expect(test.playbackBlocked).toBe(false);
  });

  it('reports empty output without attempting playback', async () => {
    FakeRecorder.output = new Blob();
    const stream = makeStream();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    const test = makeTest();
    await test.start('');
    test.stop();
    await vi.waitFor(() => expect(test.error).toBe(true));
    expect(test.hasRecording).toBe(false);
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();
  });

  it('does not request capture when recording is unavailable', async () => {
    vi.stubGlobal('MediaRecorder', undefined);
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    const test = makeTest();
    await test.start('');
    expect(test.error).toBe(true);
    expect(capture).not.toHaveBeenCalled();
  });

  it('reports denied microphone access without retaining a recording', async () => {
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockRejectedValue(
      new DOMException('Denied', 'NotAllowedError')
    );
    const test = makeTest();
    await test.start('microphone');
    expect(test.error).toBe(true);
    expect(test.active).toBe(false);
    expect(test.pending).toBe(false);
    expect(test.hasRecording).toBe(false);
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();
  });

  it('switches the selected replay speaker without reopening capture', async () => {
    const stream = makeStream();
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    const sink = vi.spyOn(HTMLMediaElement.prototype, 'setSinkId').mockResolvedValue();
    const test = makeTest();
    await test.start('', 'first');
    expect(await test.setSpeaker('second')).toBe(true);
    expect(capture).toHaveBeenCalledOnce();
    expect(stream.getAudioTracks()[0].readyState).toBe('live');
    test.stop();
    await vi.waitFor(() => expect(test.hasRecording).toBe(true));
    expect(await test.setSpeaker('third')).toBe(true);
    expect(sink.mock.calls.map(([id]) => id)).toEqual(['first', 'second', 'third']);
    expect(capture).toHaveBeenCalledOnce();
  });

  it('routes replay through Web Audio when only context output selection is available', async () => {
    const descriptor = Object.getOwnPropertyDescriptor(HTMLMediaElement.prototype, 'setSinkId');
    Object.defineProperty(HTMLMediaElement.prototype, 'setSinkId', {
      configurable: true,
      value: undefined
    });
    const sink = vi
      .spyOn(
        AudioContext.prototype as AudioContext & { setSinkId: (id: string) => Promise<void> },
        'setSinkId'
      )
      .mockResolvedValue();
    const stream = makeStream();
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    const test = makeTest();
    try {
      await test.start('', 'first');
      expect(await test.setSpeaker('second')).toBe(true);
      test.stop();
      await vi.waitFor(() => expect(test.playing).toBe(true));
      expect(sink.mock.calls.map(([id]) => id)).toEqual(['first', 'second']);
      expect(capture).toHaveBeenCalledOnce();
      expect(stream.getAudioTracks()[0].readyState).toBe('ended');
    } finally {
      if (descriptor) Object.defineProperty(HTMLMediaElement.prototype, 'setSinkId', descriptor);
      else delete (HTMLMediaElement.prototype as { setSinkId?: unknown }).setSinkId;
    }
  });

  it('fails a selected output instead of using the default speaker', async () => {
    const capture = vi.spyOn(navigator.mediaDevices, 'getUserMedia');
    vi.spyOn(HTMLMediaElement.prototype, 'setSinkId').mockRejectedValue(
      new Error('Missing output')
    );
    const test = makeTest();
    await test.start('', 'missing');
    expect(test.error).toBe(true);
    expect(capture).not.toHaveBeenCalled();
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();
  });

  it('stops capture when an output switch fails during recording', async () => {
    const stream = makeStream();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    vi.spyOn(HTMLMediaElement.prototype, 'setSinkId').mockRejectedValue(
      new Error('Missing output')
    );
    const test = makeTest();
    await test.start('');
    expect(await test.setSpeaker('missing')).toBe(false);
    expect(test.error).toBe(true);
    expect(stream.getAudioTracks()[0].readyState).toBe('ended');
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();
  });

  it('discards the old clip when a new test starts', async () => {
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(async () => makeStream());
    const revoke = vi.mocked(URL.revokeObjectURL);
    const test = makeTest();
    await test.start('');
    test.stop();
    await vi.waitFor(() => expect(test.hasRecording).toBe(true));
    await test.start('');
    expect(revoke).toHaveBeenCalledWith('blob:microphone-test');
    expect(test.hasRecording).toBe(false);
    expect(test.active).toBe(true);
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledOnce();
  });

  it('discards capture and output on navigation without replay', async () => {
    const stream = makeStream();
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockResolvedValue(stream);
    const test = makeTest();
    await test.start('');
    test.cancel();
    await Promise.resolve();
    expect(stream.getAudioTracks()[0].readyState).toBe('ended');
    expect(test.hasRecording).toBe(false);
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();
  });

  it('stops a stream that arrives after cancellation', async () => {
    let resolve!: (stream: MediaStream) => void;
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    const test = makeTest();
    const pending = test.start('');
    test.cancel();
    const stream = makeStream();
    resolve(stream);
    await pending;
    expect(stream.getAudioTracks()[0].readyState).toBe('ended');
    expect(FakeRecorder.instances).toHaveLength(0);
  });

  it('cancels pending access when Stop is selected without starting replay', async () => {
    let resolve!: (stream: MediaStream) => void;
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        })
    );
    const test = makeTest();
    const pending = test.start('');
    expect(test.pending).toBe(true);
    test.stop();
    const stream = makeStream();
    resolve(stream);
    await pending;
    expect(stream.getAudioTracks()[0].readyState).toBe('ended');
    expect(test.hasRecording).toBe(false);
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();
  });

  it('restarts a recording when processing is enabled and discards the previous sample', async () => {
    const streams: MediaStream[] = [];
    vi.spyOn(navigator.mediaDevices, 'getUserMedia').mockImplementation(async () => {
      const stream = makeStream();
      streams.push(stream);
      return stream;
    });
    vi.spyOn(MicrophoneProcessor.prototype, 'init').mockImplementation(async function (
      this: MicrophoneProcessor,
      { track }
    ) {
      this.processedTrack = track;
    });
    let amount = 0;
    const test = makeTest();
    await test.start(
      '',
      '',
      () => -60,
      () => microphoneEffectsForAmount(amount)
    );
    amount = 50;
    await vi.waitFor(() => expect(FakeRecorder.instances).toHaveLength(2));
    expect(streams[0].getAudioTracks()[0].readyState).toBe('ended');
    expect(streams[1].getAudioTracks()[0].readyState).toBe('live');
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled();
  });
});
