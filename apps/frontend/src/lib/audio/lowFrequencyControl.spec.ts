import { expect, it } from 'vitest';
import { LowFrequencyControl } from './lowFrequencyControl';

const rms = (samples: Float32Array) =>
  Math.sqrt(samples.reduce((sum, x) => sum + x * x, 0) / samples.length);
function render(samples: Float32Array, amount: number, rate = 48000) {
  const processor = new LowFrequencyControl(rate, amount);
  const output = new Float32Array(samples.length);
  for (let i = 0; i < samples.length; i += 128)
    processor.process([samples.subarray(i, i + 128)], [output.subarray(i, i + 128)]);
  return output;
}
const tone = (frequency: number, amplitude: number, seconds = 1, rate = 48000) =>
  Float32Array.from(
    { length: seconds * rate },
    (_, i) => amplitude * Math.sin((2 * Math.PI * frequency * i) / rate)
  );

it.each([44100, 48000])('reduces brief low thumps progressively at %s Hz', (rate) => {
  const input = new Float32Array(rate);
  const burst = tone(60, 0.8, 0.06, rate);
  const offset = Math.round(rate * 0.3);
  input.set(burst, offset);
  const levels = [0, 0.5, 1].map((amount) =>
    rms(render(input, amount, rate).slice(offset, offset + burst.length))
  );
  expect(levels[1]).toBeLessThan(levels[0] * 0.95);
  expect(levels[2]).toBeLessThan(levels[1] * 0.95);
  expect(levels[2]).toBeGreaterThan(levels[0] * 0.7);
  const spokenOnset = tone(500, 0.3, 0.06, rate);
  expect(render(spokenOnset, 1, rate)).toEqual(spokenOnset);
});

it('controls sustained boom while preserving ordinary vocal warmth and quiet rumble', () => {
  const boomy = tone(100, 0.5, 2);
  const levels = [0, 0.5, 1].map((amount) => rms(render(boomy, amount).slice(48000)));
  expect(levels[2]).toBeLessThan(levels[1] * 0.95);
  expect(levels[1]).toBeLessThan(levels[0] * 0.95);
  expect(levels[2]).toBeGreaterThan(levels[0] * 0.84);
  const natural = Float32Array.from(
    { length: 48000 },
    (_, i) =>
      0.1 * Math.sin((2 * Math.PI * 150 * i) / 48000) +
      0.07 * Math.sin((2 * Math.PI * 300 * i) / 48000) +
      0.04 * Math.sin((2 * Math.PI * 600 * i) / 48000)
  );
  expect(rms(render(natural, 1))).toBeCloseTo(rms(natural), 3);
  const quiet = tone(60, 0.005);
  expect(render(quiet, 1)).toEqual(quiet);
});

it('uses linked gains, supports in-place processing, and returns to exact Normal', () => {
  const mono = tone(60, 0.8, 0.1);
  const stereo = [mono, Float32Array.from(mono, (x) => x * 0.5)];
  const separate = stereo.map((x) => new Float32Array(x.length));
  new LowFrequencyControl(48000, 1).process(stereo, separate);
  const inplace = stereo.map((x) => x.slice());
  new LowFrequencyControl(48000, 1).process(inplace, inplace);
  expect(inplace).toEqual(separate);
  for (let i = 0; i < mono.length; i++) expect(separate[0][i]).toBeCloseTo(separate[1][i] * 2, 6);
  const processor = new LowFrequencyControl(48000, 1);
  processor.process(stereo, separate);
  processor.amount = 0;
  const input = [new Float32Array(128).fill(0.1)];
  const output = [new Float32Array(128)];
  for (let i = 0; i < 150; i++) processor.process(input, output);
  expect(output).toEqual(input);
  processor.process([], output);
  expect(output[0].every((x) => x === 0)).toBe(true);
});

it('releases the bass reduction after returning to normal speech', () => {
  const input = new Float32Array(48000 * 5);
  input.set(tone(100, 0.5, 1));
  input.set(tone(500, 0.2, 4), 48000);
  const result = render(input, 1);
  expect(rms(result.slice(-48000))).toBeCloseTo(0.2 / Math.SQRT2, 3);
  expect(result.every(Number.isFinite)).toBe(true);
});

it('passes Normal and invalid initial amounts without changing samples', () => {
  const samples = Float32Array.of(0.5, -0.5, 0.001, 2, -2);
  for (const amount of [0, NaN, Infinity]) expect(render(samples, amount)).toEqual(samples);
});

it('fades out a held bass correction without a jump at Normal', () => {
  const processor = new LowFrequencyControl(48000, 1);
  const input = [new Float32Array(128).fill(0.5)];
  const output = [new Float32Array(128)];
  for (let i = 0; i < 750; i++) processor.process(input, output);
  let previous = output[0][127];
  expect(previous).toBeLessThan(0.45);
  processor.amount = 0;
  let largestStep = 0;
  for (let i = 0; i < 150; i++) {
    processor.process(input, output);
    for (const sample of output[0]) {
      largestStep = Math.max(largestStep, Math.abs(sample - previous));
      previous = sample;
    }
  }
  expect(largestStep).toBeLessThan(0.001);
  expect(previous).toBe(0.5);
});
