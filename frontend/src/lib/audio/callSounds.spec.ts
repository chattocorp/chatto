import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

class FakeAudioParam {
  value = 0;

  setValueAtTime = vi.fn();
  exponentialRampToValueAtTime = vi.fn();
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

class FakeGainNode extends FakeAudioNode {
  gain = new FakeAudioParam();

  constructor() {
    super('gain');
  }
}

class FakeAudioContext {
  static instances: FakeAudioContext[] = [];

  currentTime = 0;
  state: AudioContextState = 'running';
  destination = new FakeAudioNode('destination');

  constructor() {
    FakeAudioContext.instances.push(this);
  }

  resume = vi.fn().mockResolvedValue(undefined);
  createOscillator = vi.fn(() => new FakeOscillatorNode());
  createGain = vi.fn(() => new FakeGainNode());
}

describe('callSounds', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.resetModules();
    vi.doUnmock('./notificationSounds');
    FakeAudioNode.instances = [];
    FakeAudioContext.instances = [];
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('schedules a rising join cue', async () => {
    vi.stubGlobal('AudioContext', FakeAudioContext);
    const { playCallSound } = await import('./callSounds');

    const playback = playCallSound('join');
    const oscillators = FakeAudioNode.instances.filter(
      (node): node is FakeOscillatorNode => node instanceof FakeOscillatorNode
    );

    expect(oscillators.map((osc) => osc.frequency.value)).toEqual([523.25, 659.25]);
    expect(oscillators.every((osc) => osc.start.mock.calls.length === 1)).toBe(true);
    expect(oscillators.every((osc) => osc.stop.mock.calls.length === 1)).toBe(true);

    await vi.runAllTimersAsync();
    await playback;
  });

  it('schedules a falling leave cue', async () => {
    vi.stubGlobal('AudioContext', FakeAudioContext);
    const { playCallSound } = await import('./callSounds');

    const playback = playCallSound('leave');
    const oscillators = FakeAudioNode.instances.filter(
      (node): node is FakeOscillatorNode => node instanceof FakeOscillatorNode
    );

    expect(oscillators.map((osc) => osc.frequency.value)).toEqual([659.25, 523.25]);

    await vi.runAllTimersAsync();
    await playback;
  });

  it('does not throw when Web Audio is unavailable', async () => {
    vi.stubGlobal('AudioContext', undefined);
    vi.stubGlobal('webkitAudioContext', undefined);
    const { playCallSound } = await import('./callSounds');

    await expect(playCallSound('join')).resolves.toBeUndefined();

    expect(FakeAudioContext.instances).toEqual([]);
  });

  it('does not depend on notification sound settings', async () => {
    vi.stubGlobal('AudioContext', FakeAudioContext);
    vi.doMock('./notificationSounds', () => {
      throw new Error('call sounds must not import notification sounds');
    });
    const { playCallSound } = await import('./callSounds');

    const playback = playCallSound('join');

    expect(FakeAudioContext.instances).toHaveLength(1);
    await vi.runAllTimersAsync();
    await playback;
  });
});
