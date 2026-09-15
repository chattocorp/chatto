import { expect, it } from 'vitest';
import { MicrophoneEffectsGraph } from './microphoneEffectsGraph';
import { microphoneEffectsForAmount } from './microphoneEffects';
import workletURL from './noiseGate.worklet?worker&url';

it('limits the complete AWESOME graph after EQ, compression and saturation', async () => {
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
    expect(Math.max(...samples.map(Math.abs))).toBeLessThanOrEqual(0.890001);
    expect(Math.max(...samples.map(Math.abs))).toBeGreaterThan(0.1);
  }
  graph.destroy();
  gate.disconnect();
  polish.disconnect();
  gate.port.close();
  polish.port.close();
});
