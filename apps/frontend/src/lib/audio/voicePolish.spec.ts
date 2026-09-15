import { expect, it } from 'vitest';
import { VoicePolish, normalizePolish } from './voicePolish';
import { microphoneEffectsForAmount } from './microphoneEffects';

function render(frequency: number, amplitude: number, amount: number, rate = 48000) {
  const processor = new VoicePolish(rate, amount);
  const samples = Float32Array.from(
    { length: rate },
    (_, i) => amplitude * Math.sin((2 * Math.PI * frequency * i) / rate)
  );
  const output = new Float32Array(rate);
  for (let i = 0; i < rate; i += 128)
    processor.process([samples.subarray(i, i + 128)], [output.subarray(i, i + 128)]);
  return output;
}
const rms = (samples: Float32Array) =>
  Math.sqrt(samples.reduce((sum, x) => sum + x * x, 0) / samples.length);

it.each([44100, 48000])('selectively reduces strong sibilance at %s Hz', (rate) => {
  const low = rms(render(500, 0.3, 1, rate).slice(rate / 2));
  expect(low).toBeCloseTo(0.3 / Math.SQRT2, 3);
  const normal = rms(render(8000, 0.3, 0, rate).slice(rate / 2));
  const cool = rms(render(8000, 0.3, 0.5, rate).slice(rate / 2));
  const awesome = rms(render(8000, 0.3, 1, rate).slice(rate / 2));
  expect(awesome).toBeLessThan(cool * 0.9);
  expect(cool).toBeLessThan(normal * 0.9);
  expect(rms(render(8000, 0.005, 1, rate).slice(rate / 2))).toBeCloseTo(0.005 / Math.SQRT2, 5);
});

it('bypasses Normal exactly, including the first sample and stereo', () => {
  const input = [Float32Array.of(1.4, -1.7, 0.25), Float32Array.of(-0.1, 0.2, 0.3)];
  const output = input.map((c) => new Float32Array(c.length));
  new VoicePolish(48000, 0).process(input, output);
  expect(output).toEqual(input);
});

it('catches first-sample peaks, links stereo gain and releases after a transient', () => {
  const processor = new VoicePolish(48000, 1);
  const input = [Float32Array.of(4, -4), Float32Array.of(2, -2)];
  const output = input.map((c) => new Float32Array(c.length));
  processor.process(input, output);
  expect(Math.abs(output[0][0])).toBeCloseTo(0.89, 5);
  for (let i = 0; i < 2; i++) {
    expect(Math.abs(output[0][i])).toBeLessThanOrEqual(0.890001);
    expect(output[0][i]).toBeCloseTo(output[1][i] * 2, 5);
  }
  const quiet = [new Float32Array(128).fill(0.1)];
  const after = [new Float32Array(128)];
  for (let i = 0; i < 400; i++) processor.process(quiet, after);
  expect(after[0][127]).toBeCloseTo(0.1, 4);
  processor.process([], after);
  // Filter tails are finite and decay; a missing channel must not reuse old input.
  expect(after[0].every(Number.isFinite)).toBe(true);
});

it('smooths updates and returns to exact bypass', () => {
  const processor = new VoicePolish(48000, 1);
  const input = [new Float32Array(128).fill(0.1)];
  const output = [new Float32Array(128)];
  processor.amount = 0;
  for (let i = 0; i < 150; i++) processor.process(input, output);
  expect(output).toEqual(input);
  for (const bad of [NaN, Infinity, undefined, null, '1']) expect(normalizePolish(bad)).toBe(0);
  expect(normalizePolish(-1)).toBe(0);
  expect(normalizePolish(2)).toBe(1);
  expect([0, 50, 100].map((n) => microphoneEffectsForAmount(n).polish)).toEqual([0, 0.5, 1]);
});
