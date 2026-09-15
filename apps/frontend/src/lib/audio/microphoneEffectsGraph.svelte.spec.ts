import { expect, it } from 'vitest';
import { MicrophoneEffectsGraph } from './microphoneEffectsGraph';
import { normalizeMicrophoneEffects, type MicrophoneEffects } from './microphoneEffects';

/** Render real native DSP offline so assertions do not depend on wall-clock audio. */
async function level(frequency: number, amplitude: number, effects: Partial<MicrophoneEffects>) {
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
  const samples = buffer.getChannelData(0).slice(24000);
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
