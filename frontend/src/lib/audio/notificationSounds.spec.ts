import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest';

class FakeAudioParam {
  value = 0;

  setValueAtTime = vi.fn();
  linearRampToValueAtTime = vi.fn();
  exponentialRampToValueAtTime = vi.fn();
  cancelScheduledValues = vi.fn();
}

class FakeAudioNode {
  static instances: FakeAudioNode[] = [];

  connections: FakeAudioNode[] = [];
  disconnectCalls = 0;

  constructor(readonly kind: string) {
    FakeAudioNode.instances.push(this);
  }

  connect(destination: FakeAudioNode | FakeAudioParam) {
    if (destination instanceof FakeAudioNode) {
      this.connections.push(destination);
    }
    return destination;
  }

  disconnect() {
    this.disconnectCalls += 1;
    this.connections = [];
  }
}

class FakeOscillatorNode extends FakeAudioNode {
  frequency = new FakeAudioParam();
  type: OscillatorType = 'sine';

  constructor() {
    super('oscillator');
  }

  start = vi.fn();
  stop = vi.fn();
}

class FakeBufferSourceNode extends FakeAudioNode {
  buffer: FakeAudioBuffer | null = null;

  constructor() {
    super('buffer-source');
  }

  start = vi.fn();
  stop = vi.fn();
}

class FakeGainNode extends FakeAudioNode {
  gain = new FakeAudioParam();

  constructor() {
    super('gain');
  }
}

class FakeBiquadFilterNode extends FakeAudioNode {
  frequency = new FakeAudioParam();
  Q = new FakeAudioParam();
  type: BiquadFilterType = 'lowpass';

  constructor() {
    super('biquad-filter');
  }
}

class FakeDelayNode extends FakeAudioNode {
  delayTime = new FakeAudioParam();

  constructor() {
    super('delay');
  }
}

class FakeWaveShaperNode extends FakeAudioNode {
  curve: Float32Array<ArrayBuffer> | null = null;
  oversample: OverSampleType = 'none';

  constructor() {
    super('wave-shaper');
  }
}

class FakeConvolverNode extends FakeAudioNode {
  buffer: FakeAudioBuffer | null = null;

  constructor() {
    super('convolver');
  }
}

class FakeAudioBuffer {
  readonly numberOfChannels: number;
  readonly channels: Float32Array<ArrayBuffer>[];

  constructor(channels: number, length: number) {
    this.numberOfChannels = channels;
    this.channels = Array.from(
      { length: channels },
      () => new Float32Array(new ArrayBuffer(length * Float32Array.BYTES_PER_ELEMENT))
    );
  }

  getChannelData(channel: number) {
    return this.channels[channel];
  }
}

class FakeAudioContext {
  static instances: FakeAudioContext[] = [];

  currentTime = 0;
  sampleRate = 100;
  state: AudioContextState = 'running';
  destination = new FakeAudioNode('destination');

  constructor() {
    FakeAudioContext.instances.push(this);
  }

  resume = vi.fn().mockResolvedValue(undefined);
  createOscillator = vi.fn(() => new FakeOscillatorNode());
  createBufferSource = vi.fn(() => new FakeBufferSourceNode());
  createGain = vi.fn(() => new FakeGainNode());
  createBiquadFilter = vi.fn(() => new FakeBiquadFilterNode());
  createDelay = vi.fn(() => new FakeDelayNode());
  createWaveShaper = vi.fn(() => new FakeWaveShaperNode());
  createConvolver = vi.fn(() => new FakeConvolverNode());
  createBuffer = vi.fn((channels: number, length: number) => new FakeAudioBuffer(channels, length));
}

describe('notificationSounds', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.resetModules();
    FakeAudioNode.instances = [];
    FakeAudioContext.instances = [];
    vi.stubGlobal('AudioContext', FakeAudioContext);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('fades the previous sound graph before disconnecting it', async () => {
    expect.assertions(6);

    const { defaultNotificationSoundFilters, playNotificationSound } =
      await import('./notificationSounds');

    const firstPlayback = playNotificationSound('ding', {
      ...defaultNotificationSoundFilters,
      echo: 100,
      reverb: 100,
      crunch: 100
    });
    const firstPlaybackNodes = [...FakeAudioNode.instances];
    const destination = FakeAudioContext.instances[0].destination;
    const firstOutput = firstPlaybackNodes.find(
      (node): node is FakeGainNode =>
        node instanceof FakeGainNode && node.connections.includes(destination)
    );

    expect(firstPlaybackNodes.some((node) => node.kind === 'delay')).toBe(true);
    expect(firstPlaybackNodes.some((node) => node.kind === 'convolver')).toBe(true);
    expect(firstOutput).toBeDefined();

    const secondPlayback = playNotificationSound('pop', defaultNotificationSoundFilters);

    expect(firstOutput?.gain.linearRampToValueAtTime).toHaveBeenCalledWith(0, 0.04);
    expect(
      firstPlaybackNodes
        .filter((node) =>
          ['biquad-filter', 'delay', 'convolver', 'wave-shaper'].includes(node.kind)
        )
        .some((node) => node.disconnectCalls > 0)
    ).toBe(false);

    await vi.advanceTimersByTimeAsync(40);

    expect(
      firstPlaybackNodes
        .filter((node) =>
          ['biquad-filter', 'delay', 'convolver', 'wave-shaper'].includes(node.kind)
        )
        .every((node) => node.disconnectCalls > 0)
    ).toBe(true);

    await vi.runAllTimersAsync();
    await Promise.all([firstPlayback, secondPlayback]);
  });
});
