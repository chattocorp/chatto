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
  expect(await level(20, 0.5, { lowCut: true })).toBeLessThan(0.05);
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
  expect(loud / quiet).toBeLessThan(140); // Uncompressed amplitude ratio is 160.
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
    [25, 15, 0.375, 0.375, 0.75, -4, 1.375],
    [50, 30, 0.75, 0.75, 1.5, -8, 1.75],
    [75, 45, 1.125, 1.125, 2.25, -12, 2.125],
    [100, 60, 1.5, 1.5, 3, -16, 2.5]
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

it('adds progressively clearer high frequencies without relying on a volume increase', async () => {
  const contrast = async (amount: number) => {
    const effects = microphoneEffectsForAmount(amount);
    return 20 * Math.log10(
      await level(8000, 0.02, effects) / await level(500, 0.02, effects)
    );
  };
  expect(await contrast(0)).toBeCloseTo(0, 2);
  const midpoint = await contrast(50);
  const awesome = await contrast(100);
  expect(midpoint).toBeGreaterThan(0.75);
  expect(awesome).toBeGreaterThan(1.5);
  expect(awesome).toBeGreaterThan(midpoint);
  expect(awesome).toBeLessThan(4);
});
