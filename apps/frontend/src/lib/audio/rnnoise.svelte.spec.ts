import { afterEach, expect, it, vi } from 'vitest';
import { RnnoiseStage } from './rnnoise';

const contexts: AudioContext[] = [];
const stages: RnnoiseStage[] = [];
afterEach(async () => {
  stages.splice(0).forEach((stage) => stage.destroy());
  await Promise.all(contexts.splice(0).map((context) => context.close()));
  vi.restoreAllMocks();
});

function setup(sampleRate = 48000) {
  const context = new AudioContext({ sampleRate });
  contexts.push(context);
  const output = context.createAnalyser();
  const stage = new RnnoiseStage(context, output);
  stages.push(stage);
  return { context, stage, output };
}

it('does not fetch or load the optional worklet while disabled or at unsupported rates', async () => {
  const { context, stage } = setup(44100);
  const fetch = vi.spyOn(globalThis, 'fetch');
  const module = vi.spyOn(context.audioWorklet, 'addModule');
  await stage.setEnabled(false);
  expect(stage.unavailable).toBe(false);
  await stage.setEnabled(true);
  expect(stage.unavailable).toBe(true);
  expect(fetch).not.toHaveBeenCalled();
  expect(module).not.toHaveBeenCalled();
  await stage.setEnabled(false);
  expect(stage.unavailable).toBe(false);
});

it('loads the real WASM and worklet only from this origin and bypasses a runtime failure', async () => {
  const { context, stage, output } = setup();
  const fetch = vi.spyOn(globalThis, 'fetch');
  const module = vi.spyOn(context.audioWorklet, 'addModule');
  const connect = vi.spyOn(stage.input, 'connect');
  await stage.setEnabled(true);
  expect(stage.unavailable).toBe(false);
  expect(fetch).toHaveBeenCalledOnce();
  expect(new URL(String(fetch.mock.calls[0][0]), location.href).origin).toBe(location.origin);
  expect(String(fetch.mock.calls[0][0])).toContain('.wasm');
  expect(new URL(String(module.mock.calls[0][0]), location.href).origin).toBe(location.origin);
  const node = connect.mock.calls.find(([target]) => target instanceof AudioWorkletNode)?.[0];
  expect(node).toBeInstanceOf(AudioWorkletNode);
  if (!(node instanceof AudioWorkletNode)) throw new Error('Missing RNNoise worklet');
  expect(stage.input.channelCountMode).toBe('max');
  expect(node.channelCountMode).toBe('explicit');
  expect(node.channelCount).toBe(1);
  node.dispatchEvent(new Event('processorerror'));
  expect(stage.unavailable).toBe(true);
  expect(connect).toHaveBeenLastCalledWith(output);
  await stage.setEnabled(false);
  await stage.setEnabled(true);
  expect(stage.unavailable).toBe(false);
  expect(module).toHaveBeenCalledOnce();
});

it('keeps the direct path when an asset fails and permits a later retry', async () => {
  const { stage, output } = setup();
  vi.spyOn(globalThis, 'fetch').mockRejectedValueOnce(new Error('offline'));
  const connect = vi.spyOn(stage.input, 'connect');
  await stage.setEnabled(true);
  expect(stage.unavailable).toBe(true);
  expect(connect).toHaveBeenLastCalledWith(output);
  await stage.setEnabled(false);
  await stage.setEnabled(true);
  expect(stage.unavailable).toBe(false);
});

it.each(['disable', 'destroy'] as const)(
  'does not attach a late processor after %s during loading',
  async (action) => {
    const { stage } = setup();
    let release!: () => void;
    const original = globalThis.fetch;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (...args) => {
      await new Promise<void>((resolve) => {
        release = resolve;
      });
      return original(...args);
    });
    const connect = vi.spyOn(stage.input, 'connect');
    const pending = stage.setEnabled(true);
    await vi.waitFor(() => expect(release).toBeTypeOf('function'));
    if (action === 'destroy') stage.destroy();
    else await stage.setEnabled(false);
    release();
    await pending;
    expect(connect.mock.calls.some(([target]) => target instanceof AudioWorkletNode)).toBe(false);
    expect(stage.unavailable).toBe(false);
  }
);

it('reports a malformed WASM module and leaves audio bypassed', async () => {
  const { stage, output } = setup();
  vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(new Response(new Uint8Array([0, 1, 2])));
  const connect = vi.spyOn(stage.input, 'connect');
  await stage.setEnabled(true);
  expect(stage.unavailable).toBe(true);
  expect(connect).toHaveBeenLastCalledWith(output);
});

it('reduces steady background noise with the real processor and restores it when disabled', async () => {
  const { userEvent } = await import('vitest/browser');
  const { context, stage, output } = setup();
  const buffer = context.createBuffer(1, 48000, 48000);
  const samples = buffer.getChannelData(0);
  let seed = 123;
  for (let i = 0; i < samples.length; i++) {
    seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0;
    samples[i] = (seed / 2 ** 32 - 0.5) * 0.1;
  }
  const source = context.createBufferSource();
  source.buffer = buffer;
  source.loop = true;
  source.connect(stage.input);
  const mute = context.createGain();
  mute.gain.value = 0;
  output.connect(mute).connect(context.destination);
  const button = document.createElement('button');
  document.body.append(button);
  button.onclick = () => {
    void context.resume();
  };
  await userEvent.click(button);
  button.remove();
  source.start();
  const data = new Float32Array(output.fftSize);
  const rms = () => {
    output.getFloatTimeDomainData(data);
    return Math.sqrt(data.reduce((sum, sample) => sum + sample * sample, 0) / data.length);
  };
  try {
    await vi.waitFor(() => expect(rms()).toBeGreaterThan(0.02));
    await stage.setEnabled(true);
    expect(stage.unavailable).toBe(false);
    const started = context.currentTime;
    await vi.waitFor(() => expect(context.currentTime - started).toBeGreaterThan(1), {
      timeout: 3000
    });
    // RNNoise adapts over time; require a material reduction, not a fixed noise floor.
    expect(rms()).toBeLessThan(0.02);
    await stage.setEnabled(false);
    await vi.waitFor(() => expect(rms()).toBeGreaterThan(0.02));
  } finally {
    source.stop();
    source.disconnect();
    mute.disconnect();
  }
});

it('settles initialization on destroy even if addModule never resolves', async () => {
  const { context, stage } = setup();
  const module = vi
    .spyOn(context.audioWorklet, 'addModule')
    .mockImplementation(() => new Promise(() => {}));
  const pending = stage.setEnabled(true);
  await vi.waitFor(() => expect(module).toHaveBeenCalledOnce());
  stage.destroy();
  await pending;
  expect(stage.unavailable).toBe(false);
});
