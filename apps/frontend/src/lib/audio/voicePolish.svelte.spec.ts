import { expect, it } from 'vitest';
import { MicrophoneEffectsGraph } from './microphoneEffectsGraph';
import { microphoneEffectsForAmount } from './microphoneEffects';
import workletURL from './noiseGate.worklet?worker&url';

it('limits the complete AWESOME graph after EQ and compression', async () => {
  const context = new OfflineAudioContext(2, 48000, 48000);
  await context.audioWorklet.addModule(workletURL);
  const input = context.createBuffer(2, 48000, 48000);
  for (let c = 0; c < 2; c++) {
    const samples = input.getChannelData(c);
    for (let i = 0; i < samples.length; i++) {
      // Broadband transients and a loud voiced tone exercise startup and steady state.
      samples[i] =
        (c ? 0.5 : 1) * (i % 2400 === 0 ? 4 : 1.5 * Math.sin((2 * Math.PI * 500 * i) / 48000));
    }
  }
  const source = context.createBufferSource();
  source.buffer = input;
  const gate = new AudioWorkletNode(context, 'chatto-microphone-gate', {
    processorOptions: { threshold: -60, polish: 1 }
  });
  const polish = new AudioWorkletNode(context, 'chatto-voice-polish', {
    processorOptions: { polish: 1 }
  });
  polish.connect(context.destination);
  const graph = new MicrophoneEffectsGraph(context, gate, polish);
  graph.update(microphoneEffectsForAmount(100), true);
  source.connect(graph.input);
  source.start();
  const rendered = await context.startRendering();
  for (let c = 0; c < 2; c++) {
    const samples = rendered.getChannelData(c);
    expect(samples.every(Number.isFinite)).toBe(true);
    expect(Math.max(...samples.map(Math.abs))).toBeLessThanOrEqual(0.990001);
    expect(Math.max(...samples.map(Math.abs))).toBeGreaterThan(0.1);
  }
  graph.destroy();
  gate.disconnect();
  polish.disconnect();
  gate.port.close();
  polish.port.close();
});

// A voiced harmonic fixture is repeatable, but is not a substitute for listening
// to different real microphones. Bound level/tone changes instead of rewarding
// stronger processing merely because it produces a measurable difference.
it.each(
  [0, 50, 100].flatMap((amount) => [100, 180, 260].map((fundamental) => ({ amount, fundamental })))
)(
  'bounds loudness and distortion at $amount% ($fundamental Hz)',
  async ({ amount, fundamental }) => {
    const rate = 48000;
    const context = new OfflineAudioContext(1, rate, rate);
    await context.audioWorklet.addModule(workletURL);
    const buffer = context.createBuffer(1, rate, rate);
    const input = buffer.getChannelData(0);
    for (let i = 0; i < rate; i++) {
      const phase = (2 * Math.PI * fundamental * i) / rate;
      input[i] = 0.15 * Math.sin(phase) + 0.07 * Math.sin(2 * phase) + 0.035 * Math.sin(3 * phase);
    }
    const gate = new AudioWorkletNode(context, 'chatto-microphone-gate', {
      processorOptions: { threshold: -60, polish: amount / 100 }
    });
    const polish = new AudioWorkletNode(context, 'chatto-voice-polish', {
      processorOptions: { polish: amount / 100 }
    });
    const graph = new MicrophoneEffectsGraph(context, gate, polish);
    polish.connect(context.destination);
    graph.update(microphoneEffectsForAmount(amount), true);
    const source = context.createBufferSource();
    source.buffer = buffer;
    source.connect(graph.input);
    source.start();
    try {
      const output = (await context.startRendering()).getChannelData(0);
      const magnitude = (samples: Float32Array, frequency: number) => {
        let re = 0,
          im = 0;
        for (let i = rate / 2; i < rate; i++) {
          const phase = (2 * Math.PI * frequency * i) / rate;
          re += samples[i] * Math.cos(phase);
          im += samples[i] * Math.sin(phase);
        }
        return Math.hypot(re, im);
      };
      const gain = magnitude(output, fundamental) / magnitude(input, fundamental);
      const gainDb = 20 * Math.log10(gain);
      if (amount === 0) expect(gainDb).toBeCloseTo(0, 3);
      else {
        expect(gainDb).toBeGreaterThan(amount === 100 ? 4 : 0.5);
        expect(gainDb).toBeLessThan(amount === 100 ? 12 : 7);
      }
      for (const harmonic of [2, 3]) {
        const relative =
          magnitude(output, fundamental * harmonic) /
          magnitude(input, fundamental * harmonic) /
          gain;
        expect(Math.abs(20 * Math.log10(relative))).toBeLessThan(6);
      }
      // The input has no fifth harmonic: avoid adding audible waveshaping distortion.
      expect(magnitude(output, fundamental * 5) / magnitude(output, fundamental)).toBeLessThan(
        0.01
      );
    } finally {
      graph.destroy();
      gate.disconnect();
      polish.disconnect();
      gate.port.close();
      polish.port.close();
    }
  }
);
