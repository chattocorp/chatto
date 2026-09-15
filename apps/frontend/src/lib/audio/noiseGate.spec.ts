import { describe, expect, it } from 'vitest';
import { NoiseGate, normalizeGateThreshold, microphoneMeter } from './noiseGate';

function run(gate: NoiseGate, amplitude: number, ms: number) {
  const input = [new Float32Array(128).fill(amplitude)];
  const output = [new Float32Array(128)];
  for (let i = 0; i < Math.ceil((ms * 48) / 128); i++) gate.process(input, output);
  return output[0];
}

describe('Noise gate', () => {
  it('passes input exactly when off and blocks quiet input when enabled', () => {
    const gate = new NoiseGate(48000);
    expect(run(gate, 0.001, 10)[0]).toBeCloseTo(0.001);
    const enabled = new NoiseGate(48000);
    enabled.threshold = -30;
    expect(run(enabled, 0.001, 100).every((v) => v === 0)).toBe(true);
    expect(enabled.level).toBeCloseTo(0.001);
    expect(run(enabled, 0.1, 10)[127]).toBeCloseTo(0.1);
  });
  it('uses hysteresis, holds short pauses and releases smoothly', () => {
    const gate = new NoiseGate(48000);
    gate.threshold = -20;
    run(gate, 0.2, 10);
    expect(run(gate, 0.07, 500)[0]).toBeCloseTo(0.07);
    expect(run(gate, 0.01, 100)[0]).toBeCloseTo(0.01);
    const release = run(gate, 0.01, 90);
    expect(release[0]).toBeGreaterThan(0);
    expect(release[127]).toBeLessThan(release[0]);
    expect(run(gate, 0.01, 100)[127]).toBe(0);
    expect(run(gate, 0.07, 200)[127]).toBe(0);
    expect(run(gate, 0.2, 10)[127]).toBeCloseTo(0.2);
  });
  it('reopens immediately when disabled and keeps channels balanced', () => {
    const gate = new NoiseGate(48000);
    gate.threshold = -10;
    run(gate, 0.01, 10);
    gate.threshold = -60;
    const input = [new Float32Array(128).fill(0.01), new Float32Array(128).fill(0.02)];
    const output = [new Float32Array(128), new Float32Array(128)];
    gate.process(input, output);
    expect(output).toEqual(input);
  });
  it('normalizes invalid preferences to off and maps the meter to the threshold scale', () => {
    for (const bad of [undefined, null, '10', NaN, Infinity, -61, 1])
      expect(normalizeGateThreshold(bad)).toBe(-60);
    expect(normalizeGateThreshold(-30.4)).toBe(-30);
    expect(microphoneMeter(0)).toBe(0);
    expect(microphoneMeter(10 ** (-30 / 20))).toBeCloseTo(0.5);
    expect(microphoneMeter(1)).toBe(1);
  });
});

it('softens quiet word endings progressively without changing gate Off', () => {
  const gains = [0, 0.5, 1].map((softness) => {
    const gate = new NoiseGate(48000);
    gate.threshold = -20;
    gate.softness = softness;
    run(gate, 0.2, 10);
    return run(gate, 0.04, 600)[127];
  });
  expect(gains[0]).toBe(0);
  expect(gains[1]).toBe(0);
  expect(gains[2]).toBeGreaterThan(0);
  expect(gains[2]).toBeLessThan(0.04);
  const gate = new NoiseGate(48000);
  gate.softness = 1;
  expect(run(gate, 0.001, 10)[127]).toBeCloseTo(0.001);
  gate.threshold = -20;
  expect(run(gate, 0.001, 600)[127]).toBe(0);
});
