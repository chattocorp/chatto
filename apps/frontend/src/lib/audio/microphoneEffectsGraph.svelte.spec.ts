import { expect, it } from 'vitest';
import { MicrophoneEffectsGraph } from './microphoneEffectsGraph';
import {
  microphoneEffectsForAmount,
  normalizeMicrophoneEffects,
  type MicrophoneEffects
} from './microphoneEffects';

/** Render real native DSP offline so assertions do not depend on wall-clock audio. */
async function renderAudio(
  frequency: number,
  amplitude: number,
  effects: Partial<MicrophoneEffects>
) {
  const context = new OfflineAudioContext(1, 48000, 48000);
  const source = context.createOscillator();
  source.frequency.value = frequency;
  const gain = context.createGain();
  gain.gain.value = amplitude;
  const gate = context.createGain();
  const graph = new MicrophoneEffectsGraph(context, gate, context.destination);
  graph.update({ ...normalizeMicrophoneEffects(), ...effects }, true);
  source.connect(gain).connect(graph.input);
  source.start();
  const buffer = await context.startRendering();
  graph.destroy();
  return buffer.getChannelData(0);
}

async function level(frequency: number, amplitude: number, effects: Partial<MicrophoneEffects>) {
  const samples = (await renderAudio(frequency, amplitude, effects)).slice(24000);
  return Math.sqrt(samples.reduce((sum, value) => sum + value * value, 0) / samples.length);
}

it('preserves neutral audio with all effects disabled', async () => {
  expect(await level(1000, 0.5, {})).toBeCloseTo(0.5 / Math.SQRT2, 4);
});

it('removes low rumble while retaining speech frequencies', async () => {
  expect(await level(20, 0.5, { lowCut: true })).toBeLessThan(0.04);
  expect(await level(1000, 0.5, { lowCut: true })).toBeGreaterThan(0.34);
});

it.each([
  ['bass', 50],
  ['mid', 1200],
  ['treble', 10000]
] as const)(
  'cuts the %s band and bypasses saved EQ values when disabled',
  async (band, frequency) => {
    const flat = await level(frequency, 0.5, {});
    const cut = await level(frequency, 0.5, { equalizer: true, [band]: -6 });
    expect(cut).toBeLessThan(flat * 0.6);
    expect(await level(frequency, 0.5, { [band]: -6 })).toBeCloseTo(flat, 4);
  }
);

it('reserves headroom for boosted bands', async () => {
  expect(await level(1200, 1, { equalizer: true, bass: 6, mid: 6, treble: 6 })).toBeLessThan(0.71);
});

it('compresses loud input more than quiet input, with stronger reduction at higher amounts', async () => {
  const quiet = await level(1000, 0.005, { compressor: true });
  const loud = await level(1000, 0.8, { compressor: true });
  expect(quiet).toBeGreaterThan(0.005 / Math.SQRT2); // Native compressor includes makeup gain.
  expect(loud / quiet).toBeLessThan(80); // Uncompressed amplitude ratio is 160.
  expect(await level(1000, 0.8, { compressor: true, amount: 100 })).toBeLessThan(
    await level(1000, 0.8, { compressor: true, amount: 0 })
  );
  expect(await level(1000, 0.8, { compressor: false, amount: 100 })).toBeCloseTo(
    0.8 / Math.SQRT2,
    4
  );
});

it('interpolates fractional tone and compression settings across both slider halves', () => {
  const context = new OfflineAudioContext(1, 48000, 48000);
  const filters: BiquadFilterNode[] = [];
  const original = context.createBiquadFilter.bind(context);
  context.createBiquadFilter = () => {
    const node = original();
    filters.push(node);
    return node;
  };
  const compressors: DynamicsCompressorNode[] = [];
  const compressor = context.createDynamicsCompressor.bind(context);
  context.createDynamicsCompressor = () => {
    const node = compressor();
    compressors.push(node);
    return node;
  };
  const graph = new MicrophoneEffectsGraph(context, context.createGain(), context.destination);
  for (const [amount, cutoff, bass, mid, treble, threshold, ratio] of [
    [0, 0, 0, 0, 0, 0, 1],
    [25, 40, -0.5, 0.5, 0.5, -7.8, 1.95],
    [50, 80, -1, 1, 1, -15.6, 2.9],
    [75, 80, 1.5, -0.5, 2.5, -24, 5],
    [100, 80, 4, -2, 4, -32.4, 7.1]
  ]) {
    graph.update(microphoneEffectsForAmount(amount), true);
    expect(filters[0].frequency.value).toBeCloseTo(cutoff);
    expect(filters[1].gain.value).toBeCloseTo(bass);
    expect(filters[2].gain.value).toBeCloseTo(mid);
    expect(filters[3].gain.value).toBeCloseTo(treble);
    expect(compressors[0].threshold.value).toBeCloseTo(threshold);
    expect(compressors[0].ratio.value).toBeCloseTo(ratio);
  }
  graph.destroy();
});

it('keeps Normal audio neutral through the complete graph', async () => {
  expect(await level(1000, 0.5, microphoneEffectsForAmount(0))).toBeCloseTo(0.5 / Math.SQRT2, 4);
});

it('makes AWESOME fuller and more compressed than Pretty cool', async () => {
  const cool = microphoneEffectsForAmount(50);
  const awesome = microphoneEffectsForAmount(100);
  const tone = async (settings: MicrophoneEffects) =>
    (await level(100, 0.05, { ...settings, compressor: false })) /
    (await level(1200, 0.05, { ...settings, compressor: false }));
  expect(await tone(awesome)).toBeGreaterThan((await tone(cool)) * 1.5);
  const dynamics = async (settings: MicrophoneEffects) =>
    (await level(1000, 0.8, settings)) / (await level(1000, 0.02, settings));
  expect(await dynamics(awesome)).toBeLessThan(await dynamics(cool));
});

it('adds harmonics with saturation and keeps full-scale output bounded', async () => {
  const dry = await renderAudio(1000, 0.4, { saturation: 0 });
  const saturated = await renderAudio(1000, 0.4, { saturation: 0.5 });
  const harmonic = (samples: Float32Array, frequency: number) => {
    let real = 0;
    let imaginary = 0;
    for (let i = 0; i < samples.length; i++) {
      const phase = (2 * Math.PI * frequency * i) / 48000;
      real += samples[i] * Math.cos(phase);
      imaginary += samples[i] * Math.sin(phase);
    }
    return (2 * Math.hypot(real, imaginary)) / samples.length;
  };
  expect(harmonic(dry, 3000)).toBeLessThan(0.00001);
  expect(harmonic(saturated, 3000)).toBeGreaterThan(0.005);
  const fullScale = await renderAudio(1000, 1, microphoneEffectsForAmount(100));
  expect(Math.max(...fullScale.map(Math.abs))).toBeLessThanOrEqual(1);
});

it('introduces saturation only over the last fifth of Your Voice', () => {
  expect(microphoneEffectsForAmount(0).saturation).toBe(0);
  expect(microphoneEffectsForAmount(50).saturation).toBe(0);
  expect(microphoneEffectsForAmount(80).saturation).toBe(0);
  expect(microphoneEffectsForAmount(90).saturation).toBe(0.25);
  expect(microphoneEffectsForAmount(100).saturation).toBe(0.5);
});
